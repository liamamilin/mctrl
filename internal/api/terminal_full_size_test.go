package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"mctrl/internal/auth"
	"mctrl/internal/config"
	"mctrl/internal/terminal"
	"mctrl/internal/tmux"
)

type pairedPhone struct {
	server     *Server
	httpServer *httptest.Server
	session    string
	csrf       string
}

func pairPhone(t *testing.T, adapter *tmux.Adapter) *pairedPhone {
	t.Helper()
	root := t.TempDir()
	cfg := config.Default()
	cfg.RemoteAvailability = config.AvailabilityWorkOnly
	server, err := newServer(cfg, root, adapter)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { server.Close() })
	pairing, err := auth.CreatePairing(root, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	paired, err := auth.NewRegistry(root).Pair(root, pairing.Token, "Settings Phone")
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server)
	t.Cleanup(httpServer.Close)
	return &pairedPhone{server: server, httpServer: httpServer, session: paired.SessionToken, csrf: paired.CSRFToken}
}

func (p *pairedPhone) do(t *testing.T, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	request := httptest.NewRequest(method, p.httpServer.URL+path, reader)
	request.Header.Set("Cookie", auth.SessionCookieName+"="+p.session)
	request.Header.Set("Origin", p.httpServer.URL)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(auth.CSRFHeaderName, p.csrf)
	recorder := httptest.NewRecorder()
	p.server.Handler().ServeHTTP(recorder, request)
	return recorder
}

func hostFullSize(t *testing.T, phone *pairedPhone) config.TerminalSize {
	t.Helper()
	recorder := phone.do(t, http.MethodGet, "/api/v1/host", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("host response = %d %s", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		TerminalFullSize config.TerminalSize `json:"terminal_full_size"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	return payload.TerminalFullSize
}

func TestHostAdvertisesTheConfiguredFullSize(t *testing.T) {
	adapter, _ := isolateTestTmux(t)
	phone := pairPhone(t, adapter)
	got := hostFullSize(t, phone)
	if got != config.DefaultFullTerminalSize() {
		t.Fatalf("host advertised %dx%d, want the configured default %dx%d",
			got.Cols, got.Rows,
			config.DefaultFullTerminalCols, config.DefaultFullTerminalRows)
	}
}

func TestTerminalSizeUpdatePersistsAndTakesEffectWithoutRestart(t *testing.T) {
	adapter, _ := isolateTestTmux(t)
	phone := pairPhone(t, adapter)

	recorder := phone.do(t, http.MethodPut, "/api/v1/settings/terminal-size", `{"cols":300,"rows":80}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("update response = %d %s", recorder.Code, recorder.Body.String())
	}
	if got := hostFullSize(t, phone); got != (config.TerminalSize{Cols: 300, Rows: 80}) {
		t.Fatalf("host advertised %dx%d after the update, want 300x80", got.Cols, got.Rows)
	}
	if got := phone.server.TerminalFullSize(); got != (config.TerminalSize{Cols: 300, Rows: 80}) {
		t.Fatalf("server full size = %dx%d, want 300x80", got.Cols, got.Rows)
	}
	// The change must survive a restart, so it has to be on disk.
	persisted, err := config.Load(filepath.Join(phone.server.stateDir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if persisted.TerminalFullSize != (config.TerminalSize{Cols: 300, Rows: 80}) {
		t.Fatalf("config.json holds %dx%d, want 300x80", persisted.TerminalFullSize.Cols, persisted.TerminalFullSize.Rows)
	}
}

func TestTerminalSizeUpdateRejectsOutOfRangeValues(t *testing.T) {
	adapter, _ := isolateTestTmux(t)
	phone := pairPhone(t, adapter)
	for _, test := range []struct {
		name string
		body string
	}{
		{name: "too many columns", body: `{"cols":5000,"rows":60}`},
		{name: "too few columns", body: `{"cols":2,"rows":60}`},
		{name: "too many rows", body: `{"cols":240,"rows":9000}`},
		{name: "too few rows", body: `{"cols":240,"rows":1}`},
		{name: "missing values", body: `{}`},
		{name: "not json", body: `nope`},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := phone.do(t, http.MethodPut, "/api/v1/settings/terminal-size", test.body)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("update response = %d %s, want 400", recorder.Code, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), "INVALID_") {
				t.Fatalf("update response did not use a stable code: %s", recorder.Body.String())
			}
		})
	}
	// A rejected write must leave the effective value untouched.
	if got := phone.server.TerminalFullSize(); got != config.DefaultFullTerminalSize() {
		t.Fatalf("a rejected update changed the full size to %dx%d", got.Cols, got.Rows)
	}
}

func TestTerminalSizeUpdateRequiresPairedDeviceAndCSRF(t *testing.T) {
	adapter, _ := isolateTestTmux(t)
	phone := pairPhone(t, adapter)

	unauthenticated := httptest.NewRequest(http.MethodPut, phone.httpServer.URL+"/api/v1/settings/terminal-size", strings.NewReader(`{"cols":300,"rows":80}`))
	unauthenticated.Header.Set("Origin", phone.httpServer.URL)
	unauthenticated.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	phone.server.Handler().ServeHTTP(recorder, unauthenticated)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated update = %d %s, want 401", recorder.Code, recorder.Body.String())
	}

	withoutCSRF := httptest.NewRequest(http.MethodPut, phone.httpServer.URL+"/api/v1/settings/terminal-size", strings.NewReader(`{"cols":300,"rows":80}`))
	withoutCSRF.Header.Set("Cookie", auth.SessionCookieName+"="+phone.session)
	withoutCSRF.Header.Set("Origin", phone.httpServer.URL)
	withoutCSRF.Header.Set("Content-Type", "application/json")
	recorder = httptest.NewRecorder()
	phone.server.Handler().ServeHTTP(recorder, withoutCSRF)
	if recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), "CSRF_FAILED") {
		t.Fatalf("update without CSRF = %d %s, want 403 CSRF_FAILED", recorder.Code, recorder.Body.String())
	}
}

func TestTerminalFullSizeSurvivesAnOlderConfigFile(t *testing.T) {
	// A config written before terminal_full_size existed must still load, or the
	// daemon would refuse to start after an upgrade.
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	legacy := `{
  "schema_version": 1,
  "device_name": "Test Mac",
  "listen_address": "127.0.0.1",
  "port": 7681,
  "transport_profile": "trusted_lan_http",
  "remote_availability": "on_ac",
  "start_after_login": true,
  "store_prompts": false
}
`
	if err := os.WriteFile(path, []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("legacy config failed to load: %v", err)
	}
	if loaded.TerminalFullSize != config.DefaultFullTerminalSize() {
		t.Fatalf("legacy config full size = %dx%d, want the default %dx%d",
			loaded.TerminalFullSize.Cols, loaded.TerminalFullSize.Rows,
			config.DefaultFullTerminalCols, config.DefaultFullTerminalRows)
	}
}

func TestTerminalWebSocketClampsAbsurdClientResizes(t *testing.T) {
	adapter, socket := isolateTestTmux(t)
	root := t.TempDir()
	cfg := config.Default()
	cfg.RemoteAvailability = config.AvailabilityWorkOnly
	server, err := newServer(cfg, root, adapter)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	name := "mctrl-ws-clamp-" + strings.ReplaceAll(time.Now().UTC().Format("150405.000000"), ".", "")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	id, err := adapter.CreateManagedSession(ctx, name, t.TempDir(), []string{"/bin/sh", "-c", "printf ready; sleep 20"})
	if err != nil {
		t.Fatal(err)
	}
	defer testTmuxCommand(socket, "kill-session", "-t", name).Run()

	pairing, err := auth.CreatePairing(root, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	paired, err := auth.NewRegistry(root).Pair(root, pairing.Token, "Clamp Phone")
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server)
	defer httpServer.Close()
	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/api/v1/sessions/" + id + "/terminal"
	header := http.Header{}
	header.Set("Origin", httpServer.URL)
	header.Set("Cookie", auth.SessionCookieName+"="+paired.SessionToken)
	conn, _, err := (&websocket.Dialer{HandshakeTimeout: 3 * time.Second}).DialContext(ctx, wsURL, header)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		messageType, data, readErr := conn.ReadMessage()
		if readErr != nil {
			t.Fatal(readErr)
		}
		if messageType == websocket.BinaryMessage && strings.Contains(string(data), "ready") {
			break
		}
	}

	// tmux honours whatever size the client asks for, so an unbounded resize
	// from a phone would grow the shared window without limit. The attachment
	// must clamp it instead.
	if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"resize","cols":4000,"rows":4000}`)); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		inspected, inspectErr := adapter.InspectSession(ctx, id)
		if inspectErr == nil && inspected.ActivePane != nil && inspected.ActivePane.Width > 0 {
			if inspected.ActivePane.Width > terminal.MaxTerminalCols || inspected.ActivePane.Height > terminal.MaxTerminalRows {
				t.Fatalf("pane grew to %dx%d, beyond the supported %dx%d",
					inspected.ActivePane.Width, inspected.ActivePane.Height,
					terminal.MaxTerminalCols, terminal.MaxTerminalRows)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("pane never reported a size")
		}
		time.Sleep(100 * time.Millisecond)
	}
}
