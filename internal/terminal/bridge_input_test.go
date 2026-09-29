package terminal

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"mctrl/internal/config"
	"mctrl/internal/tmux"
)

// isolateBridgeTmux gives the test a private tmux server, so it never touches
// the developer's own sessions.
func isolateBridgeTmux(t *testing.T) *tmux.Adapter {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is not installed")
	}
	// tmux rejects a socket path that exceeds its own limit, so keep it short
	// rather than under the test's long temporary directory.
	dir, err := os.MkdirTemp("", "mctrl-t")
	if err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(dir, "s")
	t.Cleanup(func() {
		_ = exec.Command("tmux", "-S", socket, "kill-server").Run()
		_ = os.RemoveAll(dir)
	})
	return tmux.NewWithSocket(socket)
}

// dialBridge attaches a client to the bridge and returns the live connection.
func dialBridge(t *testing.T, bridge *Bridge, sessionID string) *websocket.Conn {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bridge.ServeHTTP(w, r, sessionID, "device-test")
	}))
	t.Cleanup(server.Close)
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/terminal"
	conn, _, err := (&websocket.Dialer{HandshakeTimeout: 3 * time.Second}).Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial bridge: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func readWhenNonEmpty(t *testing.T, path string, within time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil && len(data) > 0 {
			return string(data)
		}
		time.Sleep(25 * time.Millisecond)
	}
	data, _ := os.ReadFile(path)
	return string(data)
}

// The phone's input path must deliver the client's own keystrokes and nothing
// else. Every other byte the pane receives is the phone typing into the
// Session without the user asking, and it is written into the pane's own
// history where it stays after the client disconnects.
//
// This is the input-path counterpart of the replay-width test: a real pane, a
// real bridge, and a real WebSocket, asserting on the bytes the pane received.
func TestBridgeDeliversOnlyClientBytesToThePane(t *testing.T) {
	adapter := isolateBridgeTmux(t)
	dir := t.TempDir()
	logPath := filepath.Join(dir, "stdin.bin")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	script := fmt.Sprintf("stty raw -echo 2>/dev/null; cat >> %q", logPath)
	id, err := adapter.CreateManagedSession(ctx, "mctrl-input-"+strings.ReplaceAll(time.Now().UTC().Format("150405.000000"), ".", ""), dir, []string{"/bin/sh", "-c", script})
	if err != nil {
		t.Fatalf("create recording session: %v", err)
	}
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); {
		if _, statErr := os.Stat(logPath); statErr == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	bridge := NewBridge(adapter, func(string) bool { return true }, config.DefaultFullTerminalSize)
	conn := dialBridge(t, bridge, id)

	// The client's first message, exactly as the web client sends it: a text
	// frame carrying a resize. It is protocol, and it must never reach the pane.
	if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"resize","cols":46,"rows":33}`)); err != nil {
		t.Fatal(err)
	}
	// A control frame that is not a resize. It has no pane meaning at all.
	if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"prompt","text":"hello"}`)); err != nil {
		t.Fatal(err)
	}
	// The keystrokes the keybar actually sends, including escape sequences and
	// the page keys, because those are the bytes most likely to be mangled into
	// something a program prints back as text.
	keystrokes := "\x1b[5~\x1b[6~\x1b[A\rhello\x1b[997;1n"
	if err := conn.WriteMessage(websocket.BinaryMessage, []byte(keystrokes)); err != nil {
		t.Fatal(err)
	}

	got := readWhenNonEmpty(t, logPath, 8*time.Second)
	if got != keystrokes {
		t.Fatalf("pane received %q, want exactly the client keystrokes %q", got, keystrokes)
	}
	if strings.Contains(got, "resize") || strings.Contains(got, "cols") {
		t.Errorf("a control frame leaked into the pane as input: %q", got)
	}
}

// The escape sequences the keybar sends must survive the bridge byte for byte.
// A single dropped or reordered byte turns a page key into text a program
// renders, which is indistinguishable from the user typing nonsense.
func TestBridgePreservesEscapeSequenceBytes(t *testing.T) {
	adapter := isolateBridgeTmux(t)
	dir := t.TempDir()
	logPath := filepath.Join(dir, "stdin.bin")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	script := fmt.Sprintf("stty raw -echo 2>/dev/null; cat >> %q", logPath)
	id, err := adapter.CreateManagedSession(ctx, "mctrl-escape-"+strings.ReplaceAll(time.Now().UTC().Format("150405.000000"), ".", ""), dir, []string{"/bin/sh", "-c", script})
	if err != nil {
		t.Fatalf("create recording session: %v", err)
	}
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); {
		if _, statErr := os.Stat(logPath); statErr == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	bridge := NewBridge(adapter, func(string) bool { return true }, config.DefaultFullTerminalSize)
	conn := dialBridge(t, bridge, id)
	_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"resize","cols":46,"rows":33}`))

	// Written in separate frames, because that is how a real key press arrives.
	// A sequence split across two frames must not be reordered or dropped.
	parts := []string{"\x1b[", "997", ";1", "n", "\x1b[5~", "\x1b[6~"}
	for _, part := range parts {
		if err := conn.WriteMessage(websocket.BinaryMessage, []byte(part)); err != nil {
			t.Fatal(err)
		}
	}

	got := readWhenNonEmpty(t, logPath, 8*time.Second)
	want := strings.Join(parts, "")
	if got != want {
		t.Fatalf("split escape sequence arrived as %q, want %q", got, want)
	}
}
