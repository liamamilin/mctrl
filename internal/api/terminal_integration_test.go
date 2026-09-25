package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"mctrl/internal/auth"
	"mctrl/internal/config"
)

func TestTerminalWebSocketAttachesToRealTmux(t *testing.T) {
	adapter, socket := isolateTestTmux(t)
	root := t.TempDir()
	cfg := config.Default()
	cfg.RemoteAvailability = config.AvailabilityWorkOnly
	server, err := newServer(cfg, root, adapter)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	name := "mctrl-ws-test-" + strings.ReplaceAll(time.Now().UTC().Format("150405.000000"), ".", "")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	id, err := adapter.CreateManagedSession(ctx, name, t.TempDir(), []string{"/bin/sh", "-c", "printf ready; sleep 5"})
	if err != nil {
		t.Fatalf("tmux managed session: %v", err)
	}
	defer testTmuxCommand(socket, "kill-session", "-t", name).Run()

	pairing, err := auth.CreatePairing(root, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	pairResult, err := auth.NewRegistry(root).Pair(root, pairing.Token, "WS Test")
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server)
	defer httpServer.Close()
	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/api/v1/sessions/" + id + "/terminal"
	dialer := websocket.Dialer{HandshakeTimeout: 3 * time.Second}
	header := http.Header{}
	header.Set("Origin", httpServer.URL)
	header.Set("Cookie", auth.SessionCookieName+"="+pairResult.SessionToken)
	conn, response, err := dialer.DialContext(ctx, wsURL, header)
	if err != nil {
		if response != nil {
			t.Fatalf("websocket dial: %v (status %d)", err, response.StatusCode)
		}
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var output strings.Builder
	for i := 0; i < 8; i++ {
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			t.Fatal(err)
		}
		if messageType != websocket.BinaryMessage {
			continue
		}
		output.Write(data)
		if strings.Contains(output.String(), "ready") {
			return
		}
	}
	t.Fatalf("terminal output did not contain ready marker: %q", output.String())
}

func TestTerminalWebSocketReportsSessionGone(t *testing.T) {
	testTerminalWebSocketReportsSessionGone(t, false)
}

func TestTerminalWebSocketReportsSessionGoneWhileServerStaysAlive(t *testing.T) {
	testTerminalWebSocketReportsSessionGone(t, true)
}

func testTerminalWebSocketReportsSessionGone(t *testing.T, keepServerAlive bool) {
	adapter, socket := isolateTestTmux(t)
	root := t.TempDir()
	cfg := config.Default()
	cfg.RemoteAvailability = config.AvailabilityWorkOnly
	server, err := newServer(cfg, root, adapter)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	name := "mctrl-ws-gone-" + strings.ReplaceAll(time.Now().UTC().Format("150405.000000"), ".", "")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := adapter.CreateManagedSession(ctx, name, t.TempDir(), []string{"/bin/sh", "-c", "printf ready; sleep 10"}); err != nil {
		t.Fatal(err)
	}
	if keepServerAlive {
		keepAliveName := "mctrl-ws-keepalive-" + strings.ReplaceAll(time.Now().UTC().Format("150405.000000"), ".", "")
		if _, err := adapter.CreateManagedSession(ctx, keepAliveName, t.TempDir(), []string{"/bin/sh", "-c", "sleep 30"}); err != nil {
			t.Fatal(err)
		}
		defer testTmuxCommand(socket, "kill-session", "-t", keepAliveName).Run()
	}
	pairing, err := auth.CreatePairing(root, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	paired, err := auth.NewRegistry(root).Pair(root, pairing.Token, "Gone Test Phone")
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server)
	defer httpServer.Close()
	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/api/v1/sessions/" + name + "/terminal"
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
	if output, err := testTmuxCommand(socket, "kill-session", "-t", name).CombinedOutput(); err != nil {
		t.Fatalf("kill Session: %v %s", err, output)
	}
	_ = conn.SetReadDeadline(time.Now().Add(4 * time.Second))
	lastControl := ""
	for {
		messageType, data, readErr := conn.ReadMessage()
		if readErr != nil {
			t.Fatalf("Session disappeared without SESSION_GONE control frame: %v (last control: %q)", readErr, lastControl)
		}
		if messageType == websocket.TextMessage {
			lastControl = string(data)
			if strings.Contains(lastControl, "SESSION_GONE") {
				return
			}
		}
	}
}

func TestTerminalWebSocketClosesAfterCLIRevocation(t *testing.T) {
	adapter, socket := isolateTestTmux(t)
	root := t.TempDir()
	cfg := config.Default()
	cfg.RemoteAvailability = config.AvailabilityWorkOnly
	server, err := newServer(cfg, root, adapter)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	name := "mctrl-ws-revoke-" + strings.ReplaceAll(time.Now().UTC().Format("150405.000000"), ".", "")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := adapter.CreateManagedSession(ctx, name, t.TempDir(), []string{"/bin/sh", "-c", "printf ready; sleep 10"}); err != nil {
		t.Fatal(err)
	}
	defer testTmuxCommand(socket, "kill-session", "-t", name).Run()
	pairing, err := auth.CreatePairing(root, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	paired, err := auth.NewRegistry(root).Pair(root, pairing.Token, "Revoked Phone")
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server)
	defer httpServer.Close()
	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/api/v1/sessions/" + name + "/terminal"
	header := http.Header{}
	header.Set("Origin", httpServer.URL)
	header.Set("Cookie", auth.SessionCookieName+"="+paired.SessionToken)
	conn, _, err := (&websocket.Dialer{HandshakeTimeout: 3 * time.Second}).DialContext(ctx, wsURL, header)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	ready := false
	for !ready {
		messageType, data, readErr := conn.ReadMessage()
		if readErr != nil {
			t.Fatal(readErr)
		}
		if messageType == websocket.BinaryMessage && strings.Contains(string(data), "ready") {
			ready = true
		}
	}
	if err := auth.NewRegistry(root).Revoke(paired.Device.ID); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(4 * time.Second))
	for {
		messageType, data, readErr := conn.ReadMessage()
		if readErr != nil {
			// The registry monitor closes revoked attachments even when the
			// browser does not receive the final control frame.
			return
		}
		if messageType == websocket.TextMessage && strings.Contains(string(data), "DEVICE_REVOKED") {
			return
		}
	}
}
