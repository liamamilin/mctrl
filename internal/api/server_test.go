package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mctrl/internal/auth"
	"mctrl/internal/config"
	"mctrl/internal/work"
)

func TestIdempotencyFingerprintRejectsPayloadReuse(t *testing.T) {
	request := createWorkRequest{RequestID: "request", ProjectID: "project", RunnerID: "shell", Prompt: "echo one"}
	existing := work.Work{RequestFingerprint: requestFingerprint(request), ProjectID: request.ProjectID, RunnerID: request.RunnerID}
	if err := checkIdempotentPayload(existing, request); err != nil {
		t.Fatalf("matching payload was rejected: %v", err)
	}
	changed := request
	changed.Prompt = "echo two"
	if err := checkIdempotentPayload(existing, changed); err == nil {
		t.Fatal("same request_id accepted a different prompt")
	}
	legacy := work.Work{ProjectID: request.ProjectID, RunnerID: request.RunnerID}
	if err := checkIdempotentPayload(legacy, request); err != nil {
		t.Fatalf("compatible legacy Work was rejected: %v", err)
	}
}

func TestLateSessionEvidenceCannotRegressExitedWork(t *testing.T) {
	cfg := config.Default()
	cfg.RemoteAvailability = config.AvailabilityWorkOnly
	server, err := NewServer(cfg, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	finished := time.Now().UTC()
	exitCode := 5
	item := work.Work{
		ID: "work_late_session", AttemptID: "attempt_late", RequestID: "request_late",
		State: work.StateExited, CreatedAt: finished, FinishedAt: &finished,
		ExitCode: &exitCode, LaunchStage: work.StageChildExited, PromptDelivery: work.PromptConfirmed,
	}
	if err := server.works.Save(item); err != nil {
		t.Fatal(err)
	}
	updated, err := server.recordSessionEvidence(item, "$999", 123)
	if err != nil {
		t.Fatal(err)
	}
	if updated.State != work.StateExited || updated.LaunchStage != work.StageChildExited || updated.ExitCode == nil || *updated.ExitCode != 5 || updated.RunnerPID != 0 {
		t.Fatalf("late session evidence regressed Work: %+v", updated)
	}
	if updated.SessionID != "$999" {
		t.Fatalf("late session id was not attached: %+v", updated)
	}
}

func TestAcceptedWorkCanResumeLaunchOnIdempotentRetry(t *testing.T) {
	socket := isolateTestTmux(t)
	t.Setenv("MCTRL_RUNNER_BINARY", "/usr/bin/true")
	root := t.TempDir()
	cfg := config.Default()
	cfg.RemoteAvailability = config.AvailabilityWorkOnly
	server, err := NewServer(cfg, root)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	projectInfo, err := server.projects.Add("Resume", t.TempDir(), "shell")
	if err != nil {
		t.Fatal(err)
	}
	pairing, err := auth.CreatePairing(root, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	result, err := auth.NewRegistry(root).Pair(root, pairing.Token, "Resume Phone")
	if err != nil {
		t.Fatal(err)
	}
	item := work.Work{ID: "work_resume", RequestID: "request-resume", DeviceID: result.Device.ID, ProjectID: projectInfo.ID, ProjectPath: projectInfo.Path, RunnerID: "shell", SessionName: "mctrl-test-resume", State: work.StateAccepted, CreatedAt: time.Now().Add(-time.Minute), KeepAwake: true, LaunchStage: work.StageAccepted, PromptDelivery: work.PromptPending}
	if err := server.works.Save(item); err != nil {
		t.Fatal(err)
	}
	body := `{"request_id":"request-resume","project_id":"` + projectInfo.ID + `","runner_id":"shell","prompt":"echo resumed"}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/work", strings.NewReader(body))
	request.Header.Set("Cookie", auth.SessionCookieName+"="+result.SessionToken)
	request.Header.Set("Origin", "http://example.com")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(auth.CSRFHeaderName, result.CSRFToken)
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("resume status = %d %s", recorder.Code, recorder.Body.String())
	}
	got, err := server.works.Get(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.SessionID == "" || got.State != work.StateStarting {
		t.Fatalf("accepted Work was not resumed: %+v", got)
	}
	_ = testTmuxCommand(socket, "kill-session", "-t", item.SessionName).Run()
}

func TestConfirmableShellCommandUsesExactExecutables(t *testing.T) {
	t.Setenv("SHELL", "/opt/homebrew/bin/fish")
	for _, command := range []string{"sh", "/bin/bash", "-zsh", "/opt/homebrew/bin/fish", "nu"} {
		if !confirmableShellCommand(command) {
			t.Fatalf("shell command was not recognized: %q", command)
		}
	}
	for _, command := range []string{"", "vim", "my-custom-shell-helper", "bash-wrapper"} {
		if confirmableShellCommand(command) {
			t.Fatalf("non-shell command was accepted: %q", command)
		}
	}
}

func TestLaunchDoesNotRespawnUnrelatedSessionNameCollision(t *testing.T) {
	socket := isolateTestTmux(t)
	t.Setenv("MCTRL_RUNNER_BINARY", "/usr/bin/true")
	root := t.TempDir()
	cfg := config.Default()
	cfg.RemoteAvailability = config.AvailabilityWorkOnly
	server, err := NewServer(cfg, root)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	projectInfo, err := server.projects.Add("Collision", t.TempDir(), "shell")
	if err != nil {
		t.Fatal(err)
	}
	pairing, err := auth.CreatePairing(root, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	paired, err := auth.NewRegistry(root).Pair(root, pairing.Token, "Collision Phone")
	if err != nil {
		t.Fatal(err)
	}
	sessionName := "mctrl-test-collision-" + strings.ReplaceAll(time.Now().UTC().Format("150405.000000"), ".", "")
	if output, err := testTmuxCommand(socket, "new-session", "-d", "-s", sessionName, "sleep 5").CombinedOutput(); err != nil {
		t.Fatalf("create collision session: %v %s", err, output)
	}
	defer testTmuxCommand(socket, "kill-session", "-t", sessionName).Run()
	item := work.Work{
		ID: "work_collision", RequestID: "request-collision", DeviceID: paired.Device.ID,
		ProjectID: projectInfo.ID, ProjectPath: projectInfo.Path, RunnerID: "shell",
		SessionName: sessionName, State: work.StateAccepted, CreatedAt: time.Now(),
		KeepAwake: true, LaunchStage: work.StageAccepted, PromptDelivery: work.PromptPending,
	}
	if err := server.works.Save(item); err != nil {
		t.Fatal(err)
	}
	body := `{"request_id":"request-collision","project_id":"` + projectInfo.ID + `","runner_id":"shell","prompt":"echo collision"}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/work", strings.NewReader(body))
	request.Header.Set("Cookie", auth.SessionCookieName+"="+paired.SessionToken)
	request.Header.Set("Origin", "http://example.com")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(auth.CSRFHeaderName, paired.CSRFToken)
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), "TMUX_SESSION_COLLISION") {
		t.Fatalf("collision response = %d %s", recorder.Code, recorder.Body.String())
	}
	if _, err := server.works.ReadLaunchSpec(item.ID); err == nil {
		t.Fatal("collision wrote launch evidence")
	}
	session, err := server.tmux.InspectSession(t.Context(), sessionName)
	if err != nil {
		t.Fatal(err)
	}
	if session.ActivePane == nil || strings.Contains(session.ActivePane.StartCommand, item.ID+".launch.json") {
		t.Fatalf("unrelated Session was modified: %+v", session.ActivePane)
	}
}

func TestIdempotentRetryDoesNotRestartOrRewriteLiveRunnerEvidence(t *testing.T) {
	socket := isolateTestTmux(t)
	fakeRunner := filepath.Join(t.TempDir(), "mctrl-runner")
	if err := os.WriteFile(fakeRunner, []byte("#!/bin/sh\ntrap 'exit 0' INT TERM\nsleep 30 & wait $!\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MCTRL_RUNNER_BINARY", fakeRunner)
	root := t.TempDir()
	cfg := config.Default()
	cfg.RemoteAvailability = config.AvailabilityWorkOnly
	server, err := NewServer(cfg, root)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	projectInfo, err := server.projects.Add("Live Resume", t.TempDir(), "shell")
	if err != nil {
		t.Fatal(err)
	}
	pairing, err := auth.CreatePairing(root, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	paired, err := auth.NewRegistry(root).Pair(root, pairing.Token, "Live Runner Phone")
	if err != nil {
		t.Fatal(err)
	}
	item := work.Work{
		ID: "work_live_runner", RequestID: "request-live-runner", DeviceID: paired.Device.ID,
		ProjectID: projectInfo.ID, ProjectPath: projectInfo.Path, RunnerID: "shell",
		SessionName: "mctrl-test-live-runner", State: work.StateAccepted, CreatedAt: time.Now(),
		KeepAwake: true, LaunchStage: work.StageAccepted, PromptDelivery: work.PromptPending,
	}
	if err := server.works.Save(item); err != nil {
		t.Fatal(err)
	}
	defer testTmuxCommand(socket, "kill-session", "-t", item.SessionName).Run()
	body := `{"request_id":"request-live-runner","project_id":"` + projectInfo.ID + `","runner_id":"shell","prompt":"echo live"}`
	post := func() *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/work", strings.NewReader(body))
		request.Header.Set("Cookie", auth.SessionCookieName+"="+paired.SessionToken)
		request.Header.Set("Origin", "http://example.com")
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set(auth.CSRFHeaderName, paired.CSRFToken)
		recorder := httptest.NewRecorder()
		server.Handler().ServeHTTP(recorder, request)
		return recorder
	}
	if first := post(); first.Code != http.StatusOK {
		t.Fatalf("initial live-runner launch status = %d %s", first.Code, first.Body.String())
	}

	var startCommand string
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
		session, inspectErr := server.tmux.InspectSession(t.Context(), item.SessionName)
		if inspectErr == nil && session.ActivePane != nil {
			startCommand = session.ActivePane.StartCommand
			if strings.Contains(startCommand, item.ID+".launch.json") {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !strings.Contains(startCommand, item.ID+".launch.json") {
		t.Fatalf("live runner start command not observed: %q", startCommand)
	}
	sentinel, err := server.works.ReadLaunchSpec(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	sentinel.Prompt = "do-not-overwrite"
	if err := server.works.WriteLaunchSpec(sentinel); err != nil {
		t.Fatal(err)
	}
	if retry := post(); retry.Code != http.StatusOK {
		t.Fatalf("live-runner retry status = %d %s", retry.Code, retry.Body.String())
	}
	after, err := server.works.ReadLaunchSpec(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Prompt != "do-not-overwrite" {
		t.Fatalf("retry rewrote live runner evidence: %q", after.Prompt)
	}
}

func TestStructuredPromptIsIdempotentAndRejectsPayloadReuse(t *testing.T) {
	socket := isolateTestTmux(t)
	root := t.TempDir()
	cfg := config.Default()
	cfg.RemoteAvailability = config.AvailabilityWorkOnly
	server, err := NewServer(cfg, root)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	name := "mctrl-prompt-test-" + strings.ReplaceAll(time.Now().UTC().Format("150405.000000"), ".", "")
	if output, err := testTmuxCommand(socket, "new-session", "-d", "-s", name, "-c", t.TempDir(), "/bin/sh").CombinedOutput(); err != nil {
		t.Fatalf("create shell session: %v %s", err, output)
	}
	defer testTmuxCommand(socket, "kill-session", "-t", name).Run()
	pairing, err := auth.CreatePairing(root, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	paired, err := auth.NewRegistry(root).Pair(root, pairing.Token, "Prompt Phone")
	if err != nil {
		t.Fatal(err)
	}
	post := func(requestID, text string) *httptest.ResponseRecorder {
		body, marshalErr := json.Marshal(map[string]string{"request_id": requestID, "text": text})
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		request := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/"+name+"/prompt", strings.NewReader(string(body)))
		request.Header.Set("Cookie", auth.SessionCookieName+"="+paired.SessionToken)
		request.Header.Set("Origin", "http://example.com")
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set(auth.CSRFHeaderName, paired.CSRFToken)
		recorder := httptest.NewRecorder()
		server.Handler().ServeHTTP(recorder, request)
		return recorder
	}
	marker := "prompt-marker-7219"
	if first := post("prompt-request-1", "echo "+marker); first.Code != http.StatusOK || !strings.Contains(first.Body.String(), "CONFIRMED") {
		t.Fatalf("prompt response = %d %s", first.Code, first.Body.String())
	}
	deadline := time.Now().Add(3 * time.Second)
	beforeRetry := ""
	for time.Now().Before(deadline) {
		lines, captureErr := server.tmux.CapturePane(t.Context(), name, 100)
		if captureErr == nil {
			beforeRetry = strings.Join(lines, "\n")
			if strings.Contains(beforeRetry, marker) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !strings.Contains(beforeRetry, marker) {
		t.Fatal("prompt marker was not observed")
	}
	if retry := post("prompt-request-1", "echo "+marker); retry.Code != http.StatusOK {
		t.Fatalf("idempotent Prompt retry = %d %s", retry.Code, retry.Body.String())
	}
	time.Sleep(100 * time.Millisecond)
	afterLines, captureErr := server.tmux.CapturePane(t.Context(), name, 100)
	if captureErr != nil {
		t.Fatal(captureErr)
	}
	if after := strings.Join(afterLines, "\n"); after != beforeRetry {
		t.Fatalf("idempotent Prompt retry changed the Session:\nbefore=%q\nafter=%q", beforeRetry, after)
	}
	if reused := post("prompt-request-1", "echo different"); reused.Code != http.StatusConflict || !strings.Contains(reused.Body.String(), "IDEMPOTENCY_KEY_REUSED") {
		t.Fatalf("reused Prompt id response = %d %s", reused.Code, reused.Body.String())
	}
}

func TestTLSTerminatorPublicOriginIsAcceptedWhenHostIsRewritten(t *testing.T) {
	cfg := config.Default()
	cfg.TransportProfile = config.TransportTLSTerminated
	cfg.PublicURL = "https://mctrl.example.test"
	cfg.RemoteAvailability = config.AvailabilityWorkOnly
	server, err := NewServer(cfg, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	pairing, err := auth.CreatePairing(server.StateDir(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	body := `{"token":"` + pairing.Token + `","device_name":"TLS Phone"}`
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:7681/api/v1/pair", strings.NewReader(body))
	request.Header.Set("Origin", cfg.PublicURL)
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("pair status = %d %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Header().Get("Set-Cookie"), "Secure") {
		t.Fatal("TLS profile did not set a Secure session cookie")
	}
}

func TestDeviceListHidesRevokedHistoryAndBindsInstallation(t *testing.T) {
	cfg := config.Default()
	cfg.RemoteAvailability = config.AvailabilityWorkOnly
	server, err := NewServer(cfg, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	registry := auth.NewRegistry(server.StateDir())
	firstPairing, err := auth.CreatePairing(server.StateDir(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	first, err := registry.Pair(server.StateDir(), firstPairing.Token, "Active Browser")
	if err != nil {
		t.Fatal(err)
	}
	secondPairing, err := auth.CreatePairing(server.StateDir(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	second, err := registry.Pair(server.StateDir(), secondPairing.Token, "Revoked Browser")
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Revoke(second.Device.ID); err != nil {
		t.Fatal(err)
	}

	cookie := auth.SessionCookieName + "=" + first.SessionToken
	listRequest := httptest.NewRequest(http.MethodGet, "http://example.com/api/v1/devices", nil)
	listRequest.Header.Set("Cookie", cookie)
	listRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(listRecorder, listRequest)
	if listRecorder.Code != http.StatusOK {
		t.Fatalf("device list status = %d %s", listRecorder.Code, listRecorder.Body.String())
	}
	var listed []map[string]interface{}
	if err := json.Unmarshal(listRecorder.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0]["id"] != first.Device.ID {
		t.Fatalf("active device list = %#v", listed)
	}

	historyRequest := httptest.NewRequest(http.MethodGet, "http://example.com/api/v1/devices/history", nil)
	historyRequest.Header.Set("Cookie", cookie)
	historyRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(historyRecorder, historyRequest)
	if historyRecorder.Code != http.StatusOK {
		t.Fatalf("device history status = %d %s", historyRecorder.Code, historyRecorder.Body.String())
	}
	var history []map[string]interface{}
	if err := json.Unmarshal(historyRecorder.Body.Bytes(), &history); err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0]["id"] != second.Device.ID {
		t.Fatalf("device history = %#v", history)
	}

	bindBody := `{"installation_id":"api-installation-CCCCCCCC"}`
	bindRequest := httptest.NewRequest(http.MethodPost, "http://example.com/api/v1/devices/installation", strings.NewReader(bindBody))
	bindRequest.Header.Set("Cookie", cookie)
	bindRequest.Header.Set("Origin", "http://example.com")
	bindRequest.Header.Set("Content-Type", "application/json")
	bindRequest.Header.Set(auth.CSRFHeaderName, first.CSRFToken)
	bindRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(bindRecorder, bindRequest)
	if bindRecorder.Code != http.StatusOK {
		t.Fatalf("bind installation status = %d %s", bindRecorder.Code, bindRecorder.Body.String())
	}
	devices, err := registry.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 2 || devices[0].InstallationHash == "" {
		t.Fatalf("bound registry = %#v", devices)
	}
}

func TestPairConsoleQRIsLoopbackOriginAndTokenProtected(t *testing.T) {
	cfg := config.Default()
	cfg.RemoteAvailability = config.AvailabilityWorkOnly
	server, err := NewServer(cfg, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	pairing, err := auth.CreatePairing(server.StateDir(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	body := `{"token":"` + pairing.Token + `"}`
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:7681/api/v1/pair/qr", strings.NewReader(body))
	request.RemoteAddr = "127.0.0.1:54321"
	request.Header.Set("Origin", "http://127.0.0.1:7681")
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("pair QR status = %d %s", recorder.Code, recorder.Body.String())
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	dataURL, _ := payload["data_url"].(string)
	if !strings.HasPrefix(dataURL, "data:image/png;base64,") {
		t.Fatalf("pair QR payload = %#v", payload)
	}
	if pairURL, _ := payload["pair_url"].(string); pairURL == "" {
		t.Fatalf("pair QR payload omitted manual pair URL: %#v", payload)
	}

	remoteRequest := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:7681/api/v1/pair/qr", strings.NewReader(body))
	remoteRequest.RemoteAddr = "192.168.1.20:54321"
	remoteRequest.Header.Set("Origin", "http://127.0.0.1:7681")
	remoteRequest.Header.Set("Content-Type", "application/json")
	remoteRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(remoteRecorder, remoteRequest)
	if remoteRecorder.Code != http.StatusNotFound {
		t.Fatalf("remote pair QR status = %d %s", remoteRecorder.Code, remoteRecorder.Body.String())
	}
}

func TestBrowserLinkUsesOneDeviceRecord(t *testing.T) {
	cfg := config.Default()
	cfg.RemoteAvailability = config.AvailabilityWorkOnly
	server, err := NewServer(cfg, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	pairing, err := auth.CreatePairing(server.StateDir(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	paired, err := auth.NewRegistry(server.StateDir()).PairForInstallation(server.StateDir(), pairing.Token, "My iPhone", "pwa-installation-AAAAAAAA")
	if err != nil {
		t.Fatal(err)
	}
	cookie := auth.SessionCookieName + "=" + paired.SessionToken
	createRequest := httptest.NewRequest(http.MethodPost, "http://example.com/api/v1/devices/links", nil)
	createRequest.Header.Set("Cookie", cookie)
	createRequest.Header.Set("Origin", "http://example.com")
	createRequest.Header.Set(auth.CSRFHeaderName, paired.CSRFToken)
	createRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(createRecorder, createRequest)
	if createRecorder.Code != http.StatusCreated {
		t.Fatalf("create link status = %d %s", createRecorder.Code, createRecorder.Body.String())
	}
	var ticket struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(createRecorder.Body.Bytes(), &ticket); err != nil {
		t.Fatal(err)
	}

	linkBody := `{"token":"` + ticket.Token + `","installation_id":"safari-installation-BBBBBBBB"}`
	linkRequest := httptest.NewRequest(http.MethodPost, "http://example.com/api/v1/devices/link", strings.NewReader(linkBody))
	linkRequest.Header.Set("Origin", "http://example.com")
	linkRequest.Header.Set("Content-Type", "application/json")
	linkRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(linkRecorder, linkRequest)
	if linkRecorder.Code != http.StatusCreated {
		t.Fatalf("link browser status = %d %s", linkRecorder.Code, linkRecorder.Body.String())
	}
	var linkedCookie *http.Cookie
	for _, candidate := range linkRecorder.Result().Cookies() {
		if candidate.Name == auth.SessionCookieName {
			linkedCookie = candidate
		}
	}
	if linkedCookie == nil {
		t.Fatal("link response did not set a session cookie")
	}
	devices, err := auth.NewRegistry(server.StateDir()).ListActive()
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 || devices[0].ID != paired.Device.ID {
		t.Fatalf("linked active devices = %#v", devices)
	}
}

func TestAPIRequiresAuthAndEnforcesCSRF(t *testing.T) {
	cfg := config.Default()
	cfg.RemoteAvailability = config.AvailabilityWorkOnly
	server, err := NewServer(cfg, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	handler := server.Handler()

	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/v1/host", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.Code)
	}

	root := server.StateDir()
	pairing, err := auth.CreatePairing(root, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	result, err := auth.NewRegistry(root).Pair(root, pairing.Token, "Test Browser")
	if err != nil {
		t.Fatal(err)
	}
	cookie := auth.SessionCookieName + "=" + result.SessionToken
	csrf := result.CSRFToken
	projectBody := `{"name":"Test","path":"` + t.TempDir() + `","default_runner":"shell"}`

	withoutCSRF := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/projects", strings.NewReader(projectBody))
	request.Header.Set("Cookie", cookie)
	request.Header.Set("Origin", "http://example.com")
	request.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(withoutCSRF, request)
	if withoutCSRF.Code != http.StatusForbidden || !strings.Contains(withoutCSRF.Body.String(), "CSRF_FAILED") {
		t.Fatalf("missing CSRF response = %d %s", withoutCSRF.Code, withoutCSRF.Body.String())
	}

	withCSRF := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/v1/projects", strings.NewReader(projectBody))
	request.Header.Set("Cookie", cookie)
	request.Header.Set("Origin", "http://example.com")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(auth.CSRFHeaderName, csrf)
	handler.ServeHTTP(withCSRF, request)
	if withCSRF.Code != http.StatusCreated {
		t.Fatalf("project create status = %d %s", withCSRF.Code, withCSRF.Body.String())
	}
}
