package api

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/skip2/go-qrcode"

	"mctrl/internal/auth"
	"mctrl/internal/config"
	"mctrl/internal/project"
	"mctrl/internal/runner"
	"mctrl/internal/terminal"
	"mctrl/internal/tmux"
	"mctrl/internal/version"
	"mctrl/internal/work"
)

type pairingQRRequest struct {
	Token string `json:"token"`
}

func requestIsLoopback(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

func (s *Server) handlePairQR(w http.ResponseWriter, r *http.Request) {
	if !requestIsLoopback(r) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "API route not found", nil)
		return
	}
	if !auth.ValidateBrowserOrigin(r, s.cfg.AllowedOrigins, s.cfg.TransportProfile == config.TransportTLSTerminated) {
		writeError(w, http.StatusForbidden, "FORBIDDEN_ORIGIN", "Origin is not allowed", nil)
		return
	}
	var request pairingQRRequest
	if err := readJSONBody(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	pairing, err := auth.VerifyPairing(s.stateDir, request.Token)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "PAIR_TOKEN_EXPIRED", "Pair token is invalid or expired", nil)
		return
	}
	pairURL := config.AdvertisedPairURL(s.cfg) + "#token=" + url.QueryEscape(strings.TrimSpace(request.Token))
	png, err := qrcode.Encode(pairURL, qrcode.Medium, 512)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not render pairing QR code", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data_url":   "data:image/png;base64," + base64.StdEncoding.EncodeToString(png),
		"pair_url":   config.AdvertisedPairURL(s.cfg),
		"expires_at": pairing.ExpiresAt,
	})
}

const deviceLinkTTL = 2 * time.Minute

type pairRequest struct {
	PairToken      string `json:"pair_token"`
	Token          string `json:"token"`
	DeviceName     string `json:"device_name"`
	InstallationID string `json:"installation_id"`
}

func (s *Server) handlePair(w http.ResponseWriter, r *http.Request) {
	if !auth.ValidateBrowserOrigin(r, s.cfg.AllowedOrigins, s.cfg.TransportProfile == config.TransportTLSTerminated) {
		writeError(w, http.StatusForbidden, "FORBIDDEN_ORIGIN", "Origin is not allowed", nil)
		return
	}
	var request pairRequest
	if err := readJSONBody(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	pairToken := request.PairToken
	if pairToken == "" {
		pairToken = request.Token
	}
	result, err := s.devices.PairForInstallation(s.stateDir, pairToken, request.DeviceName, request.InstallationID)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrPairingClosed):
			writeError(w, http.StatusForbidden, "PAIRING_CLOSED", "Pairing is closed", nil)
		case errors.Is(err, auth.ErrPairTokenInvalid):
			writeError(w, http.StatusUnauthorized, "PAIR_TOKEN_EXPIRED", "Pair token is invalid or expired", nil)
		case errors.Is(err, auth.ErrInvalidInstallationID):
			writeError(w, http.StatusBadRequest, "INVALID_INSTALLATION_ID", "Device installation identity is invalid", nil)
		default:
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not pair device", nil)
		}
		return
	}
	secure := s.cfg.TransportProfile == "tls_terminated" || r.TLS != nil
	auth.SetSessionCookieForProfile(w, result.SessionToken, secure, s.profileName)
	auth.SetCSRFCookieForProfile(w, result.CSRFToken, secure, s.profileName)
	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"device": map[string]interface{}{
			"id":   result.Device.ID,
			"name": result.Device.Name,
		},
		"csrf_token":   result.CSRFToken,
		"access_token": result.AccessToken,
		"reused":       result.Reused,
	})
}

type deviceLinkRequest struct {
	Token          string `json:"token"`
	InstallationID string `json:"installation_id"`
}

func (s *Server) handleDeviceLink(w http.ResponseWriter, r *http.Request) {
	if !auth.ValidateBrowserOrigin(r, s.cfg.AllowedOrigins, s.cfg.TransportProfile == config.TransportTLSTerminated) {
		writeError(w, http.StatusForbidden, "FORBIDDEN_ORIGIN", "Origin is not allowed", nil)
		return
	}
	var request deviceLinkRequest
	if err := readJSONBody(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	result, err := s.devices.LinkBrowser(request.Token, request.InstallationID)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrDeviceLinkInvalid):
			writeError(w, http.StatusUnauthorized, "DEVICE_LINK_EXPIRED", "Browser link is invalid or expired", nil)
		case errors.Is(err, auth.ErrInvalidInstallationID):
			writeError(w, http.StatusBadRequest, "INVALID_INSTALLATION_ID", "Device installation identity is invalid", nil)
		case errors.Is(err, auth.ErrInstallationIDConflict):
			writeError(w, http.StatusConflict, "INSTALLATION_ALREADY_BOUND", "This browser installation is already bound to another device", nil)
		default:
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "The browser link is no longer authorized", nil)
		}
		return
	}
	secure := s.cfg.TransportProfile == "tls_terminated" || r.TLS != nil
	auth.SetSessionCookieForProfile(w, result.SessionToken, secure, s.profileName)
	auth.SetCSRFCookieForProfile(w, result.CSRFToken, secure, s.profileName)
	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"device": map[string]interface{}{
			"id":   result.Device.ID,
			"name": result.Device.Name,
		},
		"csrf_token": result.CSRFToken,
	})
}

func (s *Server) handleDeviceLinkCreate(w http.ResponseWriter, r *http.Request) {
	ticket, err := s.devices.CreateDeviceLink(principalFrom(r), deviceLinkTTL)
	if err != nil {
		if errors.Is(err, auth.ErrUnauthorized) {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Paired device authentication is required", nil)
		} else {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not create browser link", nil)
		}
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"token":      ticket.Token,
		"expires_at": ticket.ExpiresAt,
	})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	principal := principalFrom(r)
	csrf := ""
	if cookie, cookieErr := r.Cookie(auth.CSRFCookieNameForProfile(s.profileName)); cookieErr == nil && s.devices.CheckCSRF(principal, cookie.Value) {
		csrf = cookie.Value
	} else {
		var err error
		csrf, err = s.devices.CSRF(principal)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Session is no longer valid", nil)
			return
		}
	}
	auth.SetCSRFCookieForProfile(w, csrf, r.TLS != nil || s.cfg.TransportProfile == "tls_terminated", s.profileName)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"device": map[string]interface{}{
			"id":   principal.Device.ID,
			"name": principal.Device.Name,
		},
		"csrf_token": csrf,
	})
}

func (s *Server) handleHost(w http.ResponseWriter, _ *http.Request) {
	info := s.HostInfo()
	full := terminal.DefaultFullWinsize()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"name":         info.Name,
		"hostname":     info.Hostname,
		"status":       info.Status,
		"remote_ready": info.RemoteReady,
		"transport":    s.cfg.TransportProfile,
		"public_url":   s.cfg.PublicURL,
		"version":      version.Version,
		"profile":      s.profileName,
		// The phone's FULL overview asks tmux for this size instead of
		// trusting the pane's current size, which the phone itself shrinks when
		// it is the only attached client.
		"terminal_full_size": map[string]interface{}{
			"cols": full.Cols,
			"rows": full.Rows,
		},
	})
}

func (s *Server) handleProjects(w http.ResponseWriter, _ *http.Request) {
	items, err := s.projects.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not load projects", nil)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

type projectRequest struct {
	Name          string `json:"name"`
	Path          string `json:"path"`
	DefaultRunner string `json:"default_runner"`
}

func (s *Server) handleProjectCreate(w http.ResponseWriter, r *http.Request) {
	var request projectRequest
	if err := readJSONBody(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	if strings.TrimSpace(request.Path) == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "path is required", nil)
		return
	}
	item, err := s.projects.Add(request.Name, request.Path, request.DefaultRunner)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "project path") {
			writeError(w, http.StatusConflict, "PROJECT_PATH_MISSING", err.Error(), nil)
		} else {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		}
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) activeWorkForProject(projectID string) ([]work.Work, error) {
	items, err := s.works.List()
	if err != nil {
		return nil, err
	}
	active := make([]work.Work, 0)
	for _, item := range items {
		if item.ProjectID == projectID && !item.Terminal() {
			active = append(active, item)
		}
	}
	return active, nil
}

func (s *Server) handleProjectDelete(w http.ResponseWriter, _ *http.Request, id string) {
	if _, err := s.projects.Get(id); err != nil {
		if errors.Is(err, project.ErrNotFound) {
			writeError(w, http.StatusNotFound, "PROJECT_NOT_FOUND", "Project was not found", nil)
		} else {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not load Project", nil)
		}
		return
	}
	active, err := s.activeWorkForProject(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not inspect active Work", nil)
		return
	}
	if len(active) > 0 {
		writeError(w, http.StatusConflict, "PROJECT_IN_USE", "Project has active Managed Work; close or finish the Work before unregistering it", map[string]interface{}{
			"active_work_count": len(active),
		})
		return
	}
	if err := s.projects.Remove(id); err != nil {
		if errors.Is(err, project.ErrNotFound) {
			writeError(w, http.StatusNotFound, "PROJECT_NOT_FOUND", "Project was not found", nil)
		} else {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not unregister Project", nil)
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "unregistered"})
}

func (s *Server) handleRunners(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.runners.List())
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := s.tmux.ListSessions(r.Context())
	if err != nil {
		s.writeTmuxError(w, err)
		return
	}
	result := make([]tmux.Session, 0, len(sessions))
	for _, session := range sessions {
		inspected, inspectErr := s.tmux.InspectSession(r.Context(), session.ID)
		if inspectErr != nil {
			// A session can disappear between list and inspect. Return the
			// list-level fact rather than turning a race into a false outage.
			result = append(result, session)
			continue
		}
		s.attachManagedWork(&inspected)
		result = append(result, inspected)
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request, id string) {
	session, err := s.tmux.InspectSession(r.Context(), id)
	if err != nil {
		s.writeTmuxError(w, err)
		return
	}
	s.attachManagedWork(&session)
	writeJSON(w, http.StatusOK, session)
}

type closeSessionRequest struct {
	Force bool `json:"force"`
}

func (s *Server) handleSessionClose(w http.ResponseWriter, r *http.Request, id string) {
	var request closeSessionRequest
	if err := readJSONBody(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	session, err := s.tmux.InspectSession(r.Context(), id)
	if errors.Is(err, tmux.ErrSessionNotFound) || errors.Is(err, tmux.ErrNoServer) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "gone", "session_id": id})
		return
	}
	if err != nil {
		s.writeTmuxError(w, err)
		return
	}
	associated, associatedOK := s.workForSession(session.ID, session.Name)
	if associatedOK && !associated.Terminal() && !request.Force {
		writeError(w, http.StatusConflict, "WORK_RUNNING", "This Session has active Managed Work; confirm termination before closing it", map[string]interface{}{
			"work": associated.DTO(),
		})
		return
	}
	if err := s.tmux.CloseSession(r.Context(), session.ID); err != nil {
		if errors.Is(err, tmux.ErrSessionNotFound) || errors.Is(err, tmux.ErrNoServer) {
			writeJSON(w, http.StatusOK, map[string]string{"status": "gone", "session_id": session.ID})
			return
		}
		s.writeTmuxError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":      "closed",
		"session_id":  session.ID,
		"was_managed": associatedOK,
	})
}

func (s *Server) workForSession(sessionID, sessionName string) (work.Work, bool) {
	if strings.TrimSpace(sessionID) == "" && strings.TrimSpace(sessionName) == "" {
		return work.Work{}, false
	}
	items, err := s.works.List()
	if err != nil {
		return work.Work{}, false
	}
	var terminal *work.Work
	for index := range items {
		item := items[index]
		if (sessionID != "" && item.SessionID != sessionID) &&
			(sessionName == "" || item.SessionName != sessionName) {
			continue
		}
		if !item.Terminal() {
			return item, true
		}
		if terminal == nil {
			candidate := item
			terminal = &candidate
		}
	}
	if terminal != nil {
		return *terminal, true
	}
	return work.Work{}, false
}

func (s *Server) attachManagedWork(session *tmux.Session) {
	if item, ok := s.workForSession(session.ID, session.Name); ok {
		session.ManagedWork = item.DTO()
	}
}

func (s *Server) handlePreview(w http.ResponseWriter, r *http.Request, id string) {
	lines := parseBoundedInt(r.URL.Query().Get("lines"), 100, 1, 500)
	session, err := s.tmux.InspectSession(r.Context(), id)
	if err != nil {
		s.writeTmuxError(w, err)
		return
	}
	if session.ActivePane == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"lines": []string{}, "captured_at": time.Now().UTC()})
		return
	}
	values, err := s.tmux.CapturePane(r.Context(), session.ActivePane.ID, lines)
	if err != nil {
		s.writeTmuxError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"lines": values, "captured_at": time.Now().UTC()})
}

func (s *Server) handleTerminal(w http.ResponseWriter, r *http.Request, id string) {
	if !auth.ValidateBrowserOrigin(r, s.cfg.AllowedOrigins, s.cfg.TransportProfile == config.TransportTLSTerminated) {
		writeError(w, http.StatusForbidden, "FORBIDDEN_ORIGIN", "Origin is not allowed", nil)
		return
	}
	if _, err := s.tmux.InspectSession(r.Context(), id); err != nil {
		s.writeTmuxError(w, err)
		return
	}
	s.terminal.ServeHTTP(w, r, id, principalFrom(r).Device.ID)
}

type createWorkRequest struct {
	RequestID string `json:"request_id"`
	ProjectID string `json:"project_id"`
	RunnerID  string `json:"runner_id"`
	Prompt    string `json:"prompt"`
}

func promptFingerprint(sessionID, text string) string {
	sum := sha256.Sum256([]byte(sessionID + "\x00" + text))
	return fmt.Sprintf("%x", sum[:])
}

func requestFingerprint(request createWorkRequest) string {
	sum := sha256.Sum256([]byte(request.ProjectID + "\x00" + request.RunnerID + "\x00" + request.Prompt))
	return fmt.Sprintf("%x", sum[:])
}

func checkIdempotentPayload(existing work.Work, request createWorkRequest) error {
	expected := requestFingerprint(request)
	if existing.RequestFingerprint != "" {
		if existing.RequestFingerprint != expected {
			return errors.New("request_id was already used with a different launch payload")
		}
		return nil
	}
	// Compatibility for records created before request fingerprints existed.
	if existing.ProjectID != request.ProjectID || existing.RunnerID != request.RunnerID {
		return errors.New("request_id was already used with a different launch payload")
	}
	return nil
}

type launchFailure struct {
	status  int
	code    string
	message string
}

func (s *Server) launchWork(item work.Work, projectInfo project.Project, request createWorkRequest) (work.Work, *launchFailure) {
	runnerBinary, err := mctrlRunnerBinary()
	if err != nil {
		updated := s.markLaunchFailure(item, "WORK_LAUNCH_FAILED", err.Error())
		return updated, &launchFailure{http.StatusInternalServerError, "WORK_LAUNCH_FAILED", err.Error()}
	}
	runnerCommand := []string{
		runnerBinary,
		"--work-file", filepath.Join(s.stateDir, "work", item.ID+".launch.json"),
		"--attempt-id", item.AttemptID,
	}

	launchContext, cancelLaunch := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelLaunch()
	sessionID, exists, err := s.findSession(launchContext, item.SessionName)
	if err != nil {
		updated := s.markLaunchFailure(item, "TMUX_UNAVAILABLE", err.Error())
		return updated, &launchFailure{http.StatusServiceUnavailable, "TMUX_UNAVAILABLE", err.Error()}
	}

	runnerAlreadyPresent := false
	runnerPanePID := 0
	var activePane *tmux.Pane
	if exists {
		session, inspectErr := s.tmux.InspectSession(launchContext, sessionID)
		if inspectErr != nil {
			updated := s.preservePartialSession(item, sessionID, inspectErr.Error())
			return updated, &launchFailure{http.StatusServiceUnavailable, "TMUX_UNAVAILABLE", inspectErr.Error()}
		}
		if session.ActivePane == nil {
			updated := s.preservePartialSession(item, sessionID, "session has no active pane")
			return updated, &launchFailure{http.StatusServiceUnavailable, "TMUX_UNAVAILABLE", "Managed Session has no active pane"}
		}
		activePane = session.ActivePane
		command := strings.ToLower(session.ActivePane.StartCommand)
		if command == "" {
			command = strings.ToLower(session.ActivePane.Command)
		}
		runnerBelongsToWork := runnerCommandMatchesAttempt(command, item.ID, item.AttemptID)
		sessionBelongsToWork := item.SessionID == sessionID
		if item.SessionID == "" && item.LaunchStage >= work.StageSessionCreated {
			sessionBelongsToWork = true
		}
		if !runnerBelongsToWork && !sessionBelongsToWork {
			message := fmt.Sprintf("tmux Session name %q is already owned by another process", item.SessionName)
			updated := s.markLaunchFailure(item, "TMUX_SESSION_COLLISION", message)
			return updated, &launchFailure{http.StatusConflict, "TMUX_SESSION_COLLISION", message}
		}
		runnerAlreadyPresent = !session.ActivePane.Dead && runnerBelongsToWork
		if runnerAlreadyPresent && session.ActivePane.PID > 0 {
			runnerPanePID = session.ActivePane.PID
		}
	}

	if !runnerAlreadyPresent {
		spec := work.LaunchSpec{
			WorkID:      item.ID,
			AttemptID:   item.AttemptID,
			ProjectID:   projectInfo.ID,
			ProjectPath: projectInfo.Path,
			RunnerID:    request.RunnerID,
			Prompt:      request.Prompt,
			StorePrompt: s.cfg.StorePrompts,
			KeepAwake:   item.KeepAwake,
			StateDir:    s.stateDir,
		}
		if err := s.works.WriteLaunchSpec(spec); err != nil {
			updated := s.markLaunchFailure(item, "WORK_LAUNCH_FAILED", err.Error())
			return updated, &launchFailure{http.StatusInternalServerError, "WORK_LAUNCH_FAILED", "Could not persist launch evidence"}
		}
		if exists {
			if respawnErr := s.tmux.RespawnPane(launchContext, activePane.ID, runnerCommand); respawnErr != nil {
				updated := s.preservePartialSession(item, sessionID, respawnErr.Error())
				return updated, &launchFailure{http.StatusServiceUnavailable, "TMUX_UNAVAILABLE", respawnErr.Error()}
			}
		} else {
			sessionID, err = s.tmux.CreateManagedSession(launchContext, item.SessionName, projectInfo.Path, runnerCommand)
			if err != nil {
				if sessionID != "" {
					updated := s.preservePartialSession(item, sessionID, err.Error())
					return updated, &launchFailure{http.StatusServiceUnavailable, "TMUX_UNAVAILABLE", err.Error()}
				}
				if !s.cfg.StorePrompts {
					_ = s.works.DeleteLaunchSpec(item.ID)
				}
				updated := s.markLaunchFailure(item, "TMUX_UNAVAILABLE", err.Error())
				return updated, &launchFailure{http.StatusServiceUnavailable, "TMUX_UNAVAILABLE", err.Error()}
			}
		}
	}
	updated, err := s.recordSessionEvidence(item, sessionID, runnerPanePID)
	if errors.Is(err, work.ErrAttemptMismatch) {
		return updated, &launchFailure{http.StatusConflict, "WORK_ATTEMPT_CHANGED", "Work launch was superseded by a newer attempt"}
	}
	if err != nil {
		return item, &launchFailure{http.StatusInternalServerError, "INTERNAL_ERROR", "Work started but session evidence could not be recorded"}
	}
	return updated, nil
}

func (s *Server) recordSessionEvidence(item work.Work, sessionID string, runnerPanePID int) (work.Work, error) {
	updated, err := s.works.UpdateIf(item.ID, item.AttemptID, nil, func(current *work.Work) error {
		current.SessionID = sessionID
		if !current.Terminal() && current.RunnerPID == 0 && runnerPanePID > 0 {
			current.RunnerPID = runnerPanePID
		}
		if current.State == work.StateAccepted {
			current.State = work.StateStarting
		}
		current.LaunchStage = work.AdvanceLaunchStage(current.LaunchStage, work.StageSessionCreated)
		if !current.Terminal() {
			current.RecoveryStatus = ""
			current.TerminationReason = ""
			current.ErrorCode = ""
			current.ErrorMessage = ""
		}
		return nil
	})
	if errors.Is(err, work.ErrAttemptMismatch) {
		latest, getErr := s.works.Get(item.ID)
		if getErr == nil {
			return latest, err
		}
	}
	return updated, err
}

func (s *Server) preservePartialSession(item work.Work, sessionID, message string) work.Work {
	updated, err := s.works.UpdateIf(item.ID, item.AttemptID, []work.State{work.StateAccepted, work.StateStarting}, func(current *work.Work) error {
		current.SessionID = sessionID
		if current.State == work.StateAccepted {
			current.State = work.StateStarting
		}
		current.LaunchStage = work.AdvanceLaunchStage(current.LaunchStage, work.StageSessionCreated)
		current.RecoveryStatus = "session_setup_incomplete"
		current.TerminationReason = message
		return nil
	})
	if err == nil {
		return updated
	}
	current, getErr := s.works.Get(item.ID)
	if getErr == nil {
		return current
	}
	return item
}

func (s *Server) findSession(ctx context.Context, name string) (string, bool, error) {
	if strings.TrimSpace(name) == "" {
		return "", false, nil
	}
	sessions, err := s.tmux.ListSessions(ctx)
	if err != nil {
		return "", false, err
	}
	for _, session := range sessions {
		if session.Name == name {
			return session.ID, true, nil
		}
	}
	return "", false, nil
}

func (s *Server) launchWithLock(item work.Work, projectInfo project.Project, request createWorkRequest) (work.Work, *launchFailure, error) {
	var result work.Work
	var failure *launchFailure
	err := s.works.WithLaunchLock(item.ID, func() error {
		current, err := s.works.Get(item.ID)
		if err != nil {
			return err
		}
		// Never create a second child for a Work whose runner was observed.
		// ACCEPTED/early STARTING records are the only records eligible for
		// launch resumption.
		if !(current.State == work.StateAccepted && current.RunnerPID == 0 && current.ChildPID == 0) && !(current.State == work.StateStarting && current.RunnerPID == 0 && current.ChildPID == 0) {
			result = current
			return nil
		}
		if current.ProjectID != request.ProjectID || current.RunnerID != request.RunnerID {
			result = current
			return nil
		}
		if current.AttemptID == "" {
			updated, updateErr := s.works.UpdateIf(current.ID, "", []work.State{work.StateAccepted, work.StateStarting}, func(latest *work.Work) error {
				latest.AttemptID = work.NewAttemptID()
				if latest.RequestFingerprint == "" {
					latest.RequestFingerprint = requestFingerprint(request)
				}
				return nil
			})
			if updateErr != nil {
				return updateErr
			}
			current = updated
		}
		if strings.TrimSpace(current.ProjectPath) != "" {
			projectInfo.Path = current.ProjectPath
		}
		result, failure = s.launchWork(current, projectInfo, request)
		return nil
	})
	return result, failure, err
}

func (s *Server) handleWorkCreate(w http.ResponseWriter, r *http.Request) {
	principal := principalFrom(r)
	var request createWorkRequest
	if err := readJSONBody(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	request.RequestID = strings.TrimSpace(request.RequestID)
	request.ProjectID = strings.TrimSpace(request.ProjectID)
	request.RunnerID = strings.TrimSpace(request.RunnerID)
	if request.RequestID == "" || len(request.RequestID) > 160 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "request_id is required and must be at most 160 characters", nil)
		return
	}
	if request.ProjectID == "" || request.RunnerID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "project_id and runner_id are required", nil)
		return
	}
	if len(request.Prompt) > 200000 || strings.ContainsRune(request.Prompt, '\x00') {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "prompt contains unsupported characters or is too long", nil)
		return
	}
	if existing, err := s.works.FindByRequest(principal.Device.ID, request.RequestID); err == nil {
		if payloadErr := checkIdempotentPayload(existing, request); payloadErr != nil {
			writeError(w, http.StatusConflict, "IDEMPOTENCY_KEY_REUSED", payloadErr.Error(), nil)
			return
		}
		if (existing.State == work.StateAccepted && existing.RunnerPID == 0 && existing.ChildPID == 0) || (existing.State == work.StateStarting && existing.RunnerPID == 0 && existing.ChildPID == 0) {
			projectInfo, projectErr := s.projects.Get(existing.ProjectID)
			if projectErr != nil {
				writeJSON(w, http.StatusOK, existing.DTO())
				return
			}
			updated, failure, launchErr := s.launchWithLock(existing, projectInfo, request)
			if launchErr != nil {
				writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not resume Work launch", nil)
				return
			}
			if failure != nil {
				writeError(w, failure.status, failure.code, failure.message, map[string]interface{}{"work": updated.DTO()})
				return
			}
			writeJSON(w, http.StatusOK, updated.DTO())
			return
		}
		writeJSON(w, http.StatusOK, existing.DTO())
		return
	} else if !errors.Is(err, work.ErrNotFound) {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not check launch idempotency", nil)
		return
	}

	projectInfo, err := s.projects.Get(request.ProjectID)
	if err != nil {
		if errors.Is(err, project.ErrNotFound) {
			writeError(w, http.StatusNotFound, "PROJECT_NOT_FOUND", "Project was not found", nil)
		} else {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not load project", nil)
		}
		return
	}
	if info, statErr := os.Stat(projectInfo.Path); statErr != nil || !info.IsDir() {
		writeError(w, http.StatusConflict, "PROJECT_PATH_MISSING", "Project path is unavailable", map[string]string{"path": projectInfo.Path})
		return
	}
	runnerAdapter, err := s.runners.Get(request.RunnerID)
	if err != nil {
		writeError(w, http.StatusNotFound, "RUNNER_NOT_FOUND", "Runner was not found", nil)
		return
	}
	detection := runnerAdapter.Detect()
	if !detection.Available {
		writeError(w, http.StatusConflict, "RUNNER_NOT_FOUND", fmt.Sprintf("%s executable was not found", runnerAdapter.Name()), nil)
		return
	}
	if _, err := s.runners.BuildLaunch(runner.WorkSpec{ProjectPath: projectInfo.Path, RunnerID: request.RunnerID, Prompt: request.Prompt}); err != nil {
		writeError(w, http.StatusConflict, "RUNNER_NOT_FOUND", err.Error(), nil)
		return
	}

	workID := work.NewID()
	now := time.Now().UTC()
	item := work.Work{
		ID:                 workID,
		RequestID:          request.RequestID,
		RequestFingerprint: requestFingerprint(request),
		AttemptID:          work.NewAttemptID(),
		DeviceID:           principal.Device.ID,
		ProjectID:          projectInfo.ID,
		ProjectPath:        projectInfo.Path,
		RunnerID:           request.RunnerID,
		SessionName:        managedSessionName(projectInfo.ID),
		State:              work.StateAccepted,
		CreatedAt:          now,
		KeepAwake:          true,
		LaunchStage:        work.StageAccepted,
		PromptDelivery:     work.PromptPending,
	}
	if s.cfg.StorePrompts {
		item.Prompt = request.Prompt
	}
	saved, existed, err := s.works.SaveIdempotent(item)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not persist Work", nil)
		return
	}
	if existed {
		if payloadErr := checkIdempotentPayload(saved, request); payloadErr != nil {
			writeError(w, http.StatusConflict, "IDEMPOTENCY_KEY_REUSED", payloadErr.Error(), nil)
			return
		}
		updated, failure, launchErr := s.launchWithLock(saved, projectInfo, request)
		if launchErr != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not resume Work launch", nil)
			return
		}
		if failure != nil {
			writeError(w, failure.status, failure.code, failure.message, map[string]interface{}{"work": updated.DTO()})
			return
		}
		writeJSON(w, http.StatusOK, updated.DTO())
		return
	}
	updated, failure, launchErr := s.launchWithLock(item, projectInfo, request)
	if launchErr != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not persist Work launch evidence", nil)
		return
	}
	if failure != nil {
		writeError(w, failure.status, failure.code, failure.message, map[string]interface{}{"work": updated.DTO()})
		return
	}
	writeJSON(w, http.StatusCreated, updated.DTO())
}

func (s *Server) markLaunchFailure(item work.Work, code, message string) work.Work {
	updated, err := s.works.UpdateIf(item.ID, item.AttemptID, []work.State{work.StateAccepted, work.StateStarting}, func(current *work.Work) error {
		if current.RunnerPID != 0 || current.ChildPID != 0 {
			return work.ErrStateConflict
		}
		current.State = work.StateLaunchFailed
		current.ErrorCode = code
		current.ErrorMessage = message
		current.FinishedAt = timePtr(time.Now().UTC())
		return nil
	})
	if err == nil {
		return updated
	}
	current, getErr := s.works.Get(item.ID)
	if getErr == nil {
		return current
	}
	return item
}

type promptRequest struct {
	RequestID string `json:"request_id"`
	Text      string `json:"text"`
}

func (s *Server) handlePrompt(w http.ResponseWriter, r *http.Request, sessionID string) {
	principal := principalFrom(r)
	var request promptRequest
	if err := readJSONBody(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	request.RequestID = strings.TrimSpace(request.RequestID)
	request.Text = strings.TrimRight(request.Text, "\r\n")
	if request.RequestID == "" || len(request.RequestID) > 160 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "request_id is required", nil)
		return
	}
	if strings.TrimSpace(request.Text) == "" || len(request.Text) > 200000 || strings.ContainsRune(request.Text, '\x00') {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "text is required, must be at most 200000 characters, and cannot contain NUL", nil)
		return
	}
	session, sessionErr := s.tmux.InspectSession(r.Context(), sessionID)
	if sessionErr != nil {
		s.writeTmuxError(w, sessionErr)
		return
	}
	associated, associatedOK := s.workForSession(session.ID, session.Name)
	if associatedOK && associated.RunnerID != "shell" {
		writeError(w, http.StatusConflict, "PROMPT_DELIVERY_FAILED", "Structured Prompt is supported only for Shell Managed Work; use the terminal for this Runner", map[string]string{"runner_id": associated.RunnerID})
		return
	}
	if !associatedOK && session.ActivePane != nil {
		if !confirmableShellCommand(session.ActivePane.Command) {
			writeError(w, http.StatusConflict, "PROMPT_DELIVERY_FAILED", "Structured Prompt cannot be confirmed for this Session; use the terminal instead", nil)
			return
		}
	}
	fingerprint := promptFingerprint(sessionID, request.Text)
	record, existed, err := s.prompts.claim(promptRecord{
		RequestID:   request.RequestID,
		DeviceID:    principal.Device.ID,
		SessionID:   sessionID,
		Fingerprint: fingerprint,
		State:       "DELIVERY_STARTED",
		CreatedAt:   time.Now().UTC(),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not persist Prompt request", nil)
		return
	}
	if !existed {
		record = promptRecord{RequestID: request.RequestID, DeviceID: principal.Device.ID, SessionID: sessionID, Fingerprint: fingerprint, State: "DELIVERY_STARTED", CreatedAt: time.Now().UTC()}
	}
	if existed {
		if record.Fingerprint != "" && record.Fingerprint != fingerprint {
			writeError(w, http.StatusConflict, "IDEMPOTENCY_KEY_REUSED", "request_id was already used for a different Prompt", nil)
			return
		}
		if record.Fingerprint == "" && record.SessionID != sessionID {
			writeError(w, http.StatusConflict, "IDEMPOTENCY_KEY_REUSED", "request_id was already used for a different Session", nil)
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"request_id": record.RequestID, "delivery": record.State})
		return
	}
	if err := s.tmux.SendPrompt(r.Context(), sessionID, request.Text); err != nil {
		record.State = "DELIVERY_UNKNOWN"
		record.Error = err.Error()
		record.FinishedAt = time.Now().UTC()
		_ = s.prompts.save(record)
		writeError(w, http.StatusBadGateway, "PROMPT_DELIVERY_FAILED", "Prompt delivery could not be confirmed; inspect the Session before retrying", map[string]interface{}{"request_id": request.RequestID, "delivery": record.State})
		return
	}
	record.State = "CONFIRMED"
	record.FinishedAt = time.Now().UTC()
	if err := s.prompts.save(record); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Prompt was sent but its result could not be recorded", map[string]interface{}{"request_id": request.RequestID})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"request_id": record.RequestID, "delivery": record.State})
}

func (s *Server) handleWorkList(w http.ResponseWriter, _ *http.Request) {
	items, err := s.works.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not load Work records", nil)
		return
	}
	result := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		result = append(result, item.DTO())
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleWorkGet(w http.ResponseWriter, _ *http.Request, id string) {
	item, err := s.works.Get(id)
	if err != nil {
		if errors.Is(err, work.ErrNotFound) {
			writeError(w, http.StatusNotFound, "WORK_NOT_FOUND", "Work was not found", nil)
		} else {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid Work id", nil)
		}
		return
	}
	writeJSON(w, http.StatusOK, item.DTO())
}

type deviceInstallationRequest struct {
	InstallationID string `json:"installation_id"`
}

func (s *Server) handleDeviceInstallation(w http.ResponseWriter, r *http.Request) {
	var request deviceInstallationRequest
	if err := readJSONBody(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	if err := s.devices.BindInstallation(principalFrom(r), request.InstallationID); err != nil {
		switch {
		case errors.Is(err, auth.ErrInvalidInstallationID):
			writeError(w, http.StatusBadRequest, "INVALID_INSTALLATION_ID", "Device installation identity is invalid", nil)
		case errors.Is(err, auth.ErrInstallationIDConflict):
			writeError(w, http.StatusConflict, "INSTALLATION_ALREADY_BOUND", "This browser installation is already bound to another device", nil)
		default:
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Paired device authentication is required", nil)
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "bound"})
}

func pairedDeviceDTO(device auth.Device) map[string]interface{} {
	entry := map[string]interface{}{
		"id":         device.ID,
		"name":       device.Name,
		"created_at": device.CreatedAt,
		"last_seen":  device.LastSeen,
	}
	if device.RevokedAt != nil {
		entry["revoked_at"] = device.RevokedAt
	}
	return entry
}

func (s *Server) handleDevices(w http.ResponseWriter, _ *http.Request) {
	devices, err := s.devices.ListActive()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not load devices", nil)
		return
	}
	result := make([]map[string]interface{}, 0, len(devices))
	for _, device := range devices {
		result = append(result, pairedDeviceDTO(device))
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleDeviceHistory(w http.ResponseWriter, _ *http.Request) {
	devices, err := s.devices.ListRevoked()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not load device history", nil)
		return
	}
	result := make([]map[string]interface{}, 0, len(devices))
	for _, device := range devices {
		result = append(result, pairedDeviceDTO(device))
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleDeviceRevoke(w http.ResponseWriter, _ *http.Request, id string) {
	if err := s.devices.Revoke(id); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "INVALID_REQUEST", "Device was not found", nil)
		} else {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not revoke device", nil)
		}
		return
	}
	s.terminal.CloseDevice(id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

func (s *Server) writeTmuxError(w http.ResponseWriter, err error) {
	if errors.Is(err, tmux.ErrSessionNotFound) {
		writeError(w, http.StatusNotFound, "SESSION_NOT_FOUND", "Session was not found", nil)
		return
	}
	if errors.Is(err, tmux.ErrUnavailable) || errors.Is(err, tmux.ErrNoServer) {
		writeError(w, http.StatusServiceUnavailable, "TMUX_UNAVAILABLE", "tmux is unavailable", nil)
		return
	}
	writeError(w, http.StatusBadGateway, "TERMINAL_ATTACH_FAILED", err.Error(), nil)
}

func confirmableShellCommand(command string) bool {
	base := strings.ToLower(filepath.Base(strings.TrimSpace(command)))
	base = strings.TrimPrefix(base, "-")
	if base == "" {
		return false
	}
	if configured := strings.TrimSpace(os.Getenv("SHELL")); configured != "" && base == strings.ToLower(filepath.Base(configured)) {
		return true
	}
	switch base {
	case "ash", "bash", "csh", "dash", "fish", "ksh", "nu", "sh", "tcsh", "zsh":
		return true
	default:
		return false
	}
}

func runnerCommandMatchesWork(command, workID string) bool {
	return runnerCommandMatchesAttempt(command, workID, "")
}

func runnerCommandMatchesAttempt(command, workID, attemptID string) bool {
	command = strings.ToLower(command)
	if !strings.Contains(command, strings.ToLower(workID+".launch.json")) {
		return false
	}
	return attemptID == "" || strings.Contains(command, strings.ToLower(attemptID))
}

func managedSessionName(projectID string) string {
	value := strings.ToLower(projectID)
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		} else if b.Len() > 0 && !strings.HasSuffix(b.String(), "-") {
			b.WriteByte('-')
		}
	}
	base := strings.Trim(b.String(), "-")
	if base == "" {
		base = "project"
	}
	if len(base) > 36 {
		base = base[:36]
	}
	profile, _ := config.ActiveProfile()
	prefix := "mctrl"
	if profile.Name != "" && profile.Name != "v1" {
		prefix += "-" + profile.Name
	}
	return prefix + "-" + base + "-" + strings.ToLower(work.NewRequestID()[:8])
}

func mctrlRunnerBinary() (string, error) {
	if value := strings.TrimSpace(os.Getenv("MCTRL_RUNNER_BINARY")); value != "" {
		if info, err := os.Stat(value); err == nil && !info.IsDir() && info.Mode()&0111 != 0 {
			return filepath.Abs(value)
		}
		return "", fmt.Errorf("MCTRL_RUNNER_BINARY does not point to a file: %s", value)
	}
	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate mctrl executable: %w", err)
	}
	candidate := filepath.Join(filepath.Dir(executable), "mctrl-runner")
	if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() && info.Mode()&0111 != 0 {
		return candidate, nil
	}
	return "", fmt.Errorf("mctrl-runner binary not found next to mctrl; build both binaries")
}

func timePtr(value time.Time) *time.Time { return &value }
