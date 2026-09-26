package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"mctrl/internal/auth"
	"mctrl/internal/config"
	"mctrl/internal/terminal"
)

func TestHostAdvertisesTheCanonicalFullSize(t *testing.T) {
	adapter, _ := isolateTestTmux(t)
	root := t.TempDir()
	cfg := config.Default()
	cfg.RemoteAvailability = config.AvailabilityWorkOnly
	server, err := newServer(cfg, root, adapter)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	pairing, err := auth.CreatePairing(root, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	paired, err := auth.NewRegistry(root).Pair(root, pairing.Token, "Full Size Phone")
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server)
	defer httpServer.Close()
	request := httptest.NewRequest(http.MethodGet, httpServer.URL+"/api/v1/host", nil)
	request.Header.Set("Cookie", auth.SessionCookieName+"="+paired.SessionToken)
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("host response = %d %s", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		TerminalFullSize *struct {
			Cols uint16 `json:"cols"`
			Rows uint16 `json:"rows"`
		} `json:"terminal_full_size"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.TerminalFullSize == nil {
		t.Fatal("host response did not advertise terminal_full_size")
	}
	full := terminal.DefaultFullWinsize()
	if payload.TerminalFullSize.Cols != full.Cols || payload.TerminalFullSize.Rows != full.Rows {
		t.Fatalf("host advertised %dx%d, want %dx%d", payload.TerminalFullSize.Cols, payload.TerminalFullSize.Rows, full.Cols, full.Rows)
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
