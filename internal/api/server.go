package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"mctrl/internal/auth"
	"mctrl/internal/config"
	"mctrl/internal/host"
	"mctrl/internal/power"
	"mctrl/internal/project"
	"mctrl/internal/runner"
	"mctrl/internal/terminal"
	"mctrl/internal/tmux"
	"mctrl/internal/work"
)

type Server struct {
	cfg         config.Config
	stateDir    string
	projects    *project.Store
	devices     *auth.Registry
	works       *work.Store
	prompts     *promptStore
	tmux        *tmux.Adapter
	runners     *runner.Registry
	terminal    *terminal.Bridge
	staticFS    fs.FS
	hostPower   *power.Assertion
	profileName string

	mu               sync.RWMutex
	started          time.Time
	stopping         bool
	lastControlErr   string
	lastReconcileErr string
}

func NewServer(cfg config.Config, stateDir string) (*Server, error) {
	return newServer(cfg, stateDir, tmux.New())
}

func newServer(cfg config.Config, stateDir string, adapter *tmux.Adapter) (*Server, error) {
	if adapter == nil {
		adapter = tmux.New()
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if cfg.PublicURL != "" {
		// TLS terminators do not always preserve the original Host header.
		// The configured public origin is therefore always an explicit browser
		// origin in addition to same-origin matching.
		public, _ := url.Parse(strings.TrimRight(cfg.PublicURL, "/"))
		origin := public.Scheme + "://" + public.Host
		found := false
		for _, allowed := range cfg.AllowedOrigins {
			if strings.EqualFold(strings.TrimRight(allowed, "/"), origin) {
				found = true
				break
			}
		}
		if !found {
			cfg.AllowedOrigins = append(cfg.AllowedOrigins, origin)
		}
	}
	if stateDir == "" {
		var err error
		stateDir, err = config.StateDir()
		if err != nil {
			return nil, err
		}
	}
	profile, err := config.ActiveProfile()
	if err != nil {
		return nil, err
	}
	if err := config.ValidateRuntimeIdentity(stateDir, profile.Name); err != nil {
		return nil, err
	}
	for _, subdir := range []string{"", "work", "logs", filepath.Join("logs", "runner")} {
		if err := os.MkdirAll(filepath.Join(stateDir, subdir), 0700); err != nil {
			return nil, err
		}
	}
	projects := project.NewStore(stateDir)
	works := work.NewStore(stateDir)
	if err := works.Ensure(); err != nil {
		return nil, err
	}
	if err := projects.Validate(); err != nil {
		return nil, fmt.Errorf("validate project registry: %w", err)
	}
	if err := auth.NewRegistry(stateDir).Validate(); err != nil {
		return nil, fmt.Errorf("validate device registry: %w", err)
	}
	server := &Server{
		cfg:         cfg,
		stateDir:    stateDir,
		profileName: profile.Name,
		projects:    projects,
		devices:     auth.NewRegistry(stateDir),
		works:       works,
		prompts:     newPromptStore(stateDir),
		tmux:        adapter,
		runners:     runner.NewRegistry(),
		staticFS:    embeddedStatic(),
		started:     time.Now().UTC(),
	}
	server.terminal = terminal.NewBridge(adapter, server.sessionHasManagedWork, server.devices.IsActive)
	if reconcileErr := server.prompts.reconcile(); reconcileErr != nil {
		server.RecordReconcileError(reconcileErr)
	}
	if cfg.RemoteAvailability != config.AvailabilityWorkOnly {
		assertion, err := power.StartHost(string(cfg.RemoteAvailability), os.Getpid())
		if err != nil {
			// The control plane remains useful without the optional host
			// assertion; expose the failure through RemoteReady instead of
			// silently claiming power protection.
			server.mu.Lock()
			server.hostPower = nil
			server.lastControlErr = "host power assertion unavailable"
			server.mu.Unlock()
		} else {
			server.hostPower = assertion
		}
	}
	return server, nil
}

func (s *Server) Config() config.Config { return s.cfg }
func (s *Server) StateDir() string      { return s.stateDir }

// RecordControlError stores a degraded control-plane fact for health/status
// reporting without exposing raw filesystem or process details over the API.
func (s *Server) RecordControlError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err == nil {
		s.lastControlErr = ""
		return
	}
	s.lastControlErr = err.Error()
}

func (s *Server) RecordReconcileError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err == nil {
		s.lastReconcileErr = ""
		return
	}
	s.lastReconcileErr = err.Error()
}

func (s *Server) controlError() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.lastReconcileErr != "" {
		return "reconciliation degraded"
	}
	return s.lastControlErr
}

// sessionHasManagedWork reports whether a tmux Session identifier or display
// name still hosts non-terminal Managed Work. Only such a Session's pane is
// mctrl's to reconfigure; every other terminal belongs to the user.
func (s *Server) sessionHasManagedWork(sessionID string) bool {
	if strings.TrimSpace(sessionID) == "" {
		return false
	}
	items, err := s.works.List()
	if err != nil {
		s.RecordReconcileError(err)
		return false
	}
	for _, item := range items {
		if item.Terminal() {
			continue
		}
		if item.SessionID == sessionID || item.SessionName == sessionID {
			return true
		}
	}
	return false
}

func (s *Server) Close() {
	s.mu.Lock()
	if s.stopping {
		s.mu.Unlock()
		return
	}
	s.stopping = true
	assertion := s.hostPower
	s.mu.Unlock()
	if assertion != nil {
		assertion.Release()
	}
}

func (s *Server) HostInfo() host.Info {
	ready := false
	s.mu.RLock()
	assertion := s.hostPower
	s.mu.RUnlock()
	switch s.cfg.RemoteAvailability {
	case config.AvailabilityOnAC:
		ready = assertion.Active() && power.OnAC()
	case config.AvailabilityAlways:
		ready = assertion.Active()
	case config.AvailabilityWorkOnly:
		if items, err := s.works.List(); err == nil {
			for _, item := range items {
				if item.KeepAwake && (item.State == work.StateStarting || item.State == work.StateRunning) && power.WorkAssertionActive(item.RunnerPID) {
					ready = true
					break
				}
			}
		}
	}
	return host.Current(s.cfg, ready)
}

func (s *Server) Handler() http.Handler { return s }

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	setSecurityHeaders(w)
	if r.URL.Path == "/healthz" {
		s.handleHealth(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/") {
		w.Header().Set("Cache-Control", "no-store")
		s.handleAPI(w, r)
		return
	}
	s.serveStatic(w, r)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	status := "ok"
	payload := map[string]interface{}{"time": time.Now().UTC(), "profile": s.profileName}
	if s.controlError() != "" {
		status = "degraded"
		payload["degraded"] = true
	}
	payload["status"] = status
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) handleAPI(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1")
	path = strings.Trim(path, "/")
	if path == "" {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "API route not found", nil)
		return
	}
	parts := strings.Split(path, "/")
	// Pairing is deliberately the only unauthenticated API operation. The
	// token is short-lived, one-time, and protected by Origin validation.
	if len(parts) == 1 && parts[0] == "pair" {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "INVALID_REQUEST", "pair requires POST", nil)
			return
		}
		s.handlePair(w, r)
		return
	}
	if len(parts) == 2 && parts[0] == "pair" && parts[1] == "qr" {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "INVALID_REQUEST", "pair QR requires POST", nil)
			return
		}
		s.handlePairQR(w, r)
		return
	}
	if len(parts) == 2 && parts[0] == "devices" && parts[1] == "link" {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "INVALID_REQUEST", "browser link requires POST", nil)
			return
		}
		s.handleDeviceLink(w, r)
		return
	}

	principal, err := s.authenticate(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Paired device authentication is required", nil)
		return
	}
	if !s.validateMutationOrigin(r, principal) {
		writeError(w, http.StatusForbidden, "FORBIDDEN_ORIGIN", "Origin is not allowed", nil)
		return
	}
	if isMutation(r.Method) && principal.ViaCookie && !s.devices.CheckCSRF(principal, r.Header.Get(auth.CSRFHeaderName)) {
		writeError(w, http.StatusForbidden, "CSRF_FAILED", "CSRF token is missing or invalid", nil)
		return
	}
	r = r.WithContext(withPrincipal(r.Context(), principal))

	switch {
	case len(parts) == 1 && parts[0] == "me" && r.Method == http.MethodGet:
		s.handleMe(w, r)
	case len(parts) == 1 && parts[0] == "host" && r.Method == http.MethodGet:
		s.handleHost(w, r)
	case len(parts) == 1 && parts[0] == "projects" && r.Method == http.MethodGet:
		s.handleProjects(w, r)
	case len(parts) == 1 && parts[0] == "projects" && r.Method == http.MethodPost:
		s.handleProjectCreate(w, r)
	case len(parts) == 2 && parts[0] == "projects" && r.Method == http.MethodDelete:
		s.handleProjectDelete(w, r, mustUnescape(parts[1]))
	case len(parts) == 1 && parts[0] == "runners" && r.Method == http.MethodGet:
		s.handleRunners(w, r)
	case len(parts) == 1 && parts[0] == "sessions" && r.Method == http.MethodGet:
		s.handleSessions(w, r)
	case len(parts) == 2 && parts[0] == "sessions" && r.Method == http.MethodGet:
		s.handleSession(w, r, mustUnescape(parts[1]))
	case len(parts) == 3 && parts[0] == "sessions" && parts[2] == "preview" && r.Method == http.MethodGet:
		s.handlePreview(w, r, mustUnescape(parts[1]))
	case len(parts) == 3 && parts[0] == "sessions" && parts[2] == "close" && r.Method == http.MethodPost:
		s.handleSessionClose(w, r, mustUnescape(parts[1]))
	case len(parts) == 3 && parts[0] == "sessions" && parts[2] == "terminal" && r.Method == http.MethodGet:
		s.handleTerminal(w, r, mustUnescape(parts[1]))
	case len(parts) == 3 && parts[0] == "sessions" && parts[2] == "prompt" && r.Method == http.MethodPost:
		s.handlePrompt(w, r, mustUnescape(parts[1]))
	case len(parts) == 1 && parts[0] == "work" && r.Method == http.MethodGet:
		s.handleWorkList(w, r)
	case len(parts) == 1 && parts[0] == "work" && r.Method == http.MethodPost:
		s.handleWorkCreate(w, r)
	case len(parts) == 2 && parts[0] == "work" && r.Method == http.MethodGet:
		s.handleWorkGet(w, r, mustUnescape(parts[1]))
	case len(parts) == 2 && parts[0] == "devices" && parts[1] == "links" && r.Method == http.MethodPost:
		s.handleDeviceLinkCreate(w, r)
	case len(parts) == 2 && parts[0] == "devices" && parts[1] == "history" && r.Method == http.MethodGet:
		s.handleDeviceHistory(w, r)
	case len(parts) == 2 && parts[0] == "devices" && parts[1] == "installation" && r.Method == http.MethodPost:
		s.handleDeviceInstallation(w, r)
	case len(parts) == 1 && parts[0] == "devices" && r.Method == http.MethodGet:
		s.handleDevices(w, r)
	case len(parts) == 2 && parts[0] == "devices" && r.Method == http.MethodDelete:
		s.handleDeviceRevoke(w, r, mustUnescape(parts[1]))
	default:
		writeError(w, http.StatusNotFound, "NOT_FOUND", "API route not found", nil)
	}
}

func (s *Server) authenticate(r *http.Request) (auth.Principal, error) {
	credential, viaCookie := auth.RequestCredentialWithSourceForProfile(r, s.profileName)
	if credential == "" {
		return auth.Principal{}, auth.ErrUnauthorized
	}
	principal, err := s.devices.Authenticate(credential)
	if err != nil {
		return auth.Principal{}, err
	}
	principal.ViaCookie = viaCookie
	return principal, nil
}

func (s *Server) validateMutationOrigin(r *http.Request, principal auth.Principal) bool {
	if !isMutation(r.Method) {
		return true
	}
	if principal.ViaCookie {
		return auth.ValidateBrowserOrigin(r, s.cfg.AllowedOrigins, s.cfg.TransportProfile == config.TransportTLSTerminated)
	}
	return auth.ValidateOriginForProfile(r, s.cfg.AllowedOrigins, s.cfg.TransportProfile == config.TransportTLSTerminated)
}

func isMutation(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func withPrincipal(ctx context.Context, principal auth.Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, principal)
}

func principalFrom(r *http.Request) auth.Principal {
	value, _ := r.Context().Value(principalKey{}).(auth.Principal)
	return value
}

type principalKey struct{}

func mustUnescape(value string) string {
	decoded, err := url.PathUnescape(value)
	if err != nil {
		return value
	}
	return decoded
}

func parseBoundedInt(value string, fallback, minValue, maxValue int) int {
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < minValue || parsed > maxValue {
		return fallback
	}
	return parsed
}

func readJSONBody(r *http.Request, destination interface{}) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 2<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("invalid JSON body: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("invalid JSON body: multiple values")
		}
		return fmt.Errorf("invalid JSON body: %w", err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code, message string, details interface{}) {
	payload := map[string]interface{}{"code": code, "message": message}
	if details != nil {
		payload["details"] = details
	}
	writeJSON(w, status, map[string]interface{}{"error": payload})
}

func setSecurityHeaders(w http.ResponseWriter) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
	w.Header().Set("Content-Security-Policy", strings.Join([]string{
		"default-src 'self'",
		"base-uri 'none'",
		"object-src 'none'",
		"frame-ancestors 'none'",
		"form-action 'self'",
		"script-src 'self'",
		"style-src 'self' 'unsafe-inline'",
		"img-src 'self' data:",
		"font-src 'self' data:",
		"connect-src 'self' ws: wss:",
		"worker-src 'self'",
		"manifest-src 'self'",
	}, "; "))
}
