package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"mctrl/internal/auth"
	"mctrl/internal/config"
	"mctrl/internal/terminal"
	"mctrl/internal/tmux"
	"mctrl/internal/work"
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

func TestTerminalWebSocketRepairsManagedWorkTransport(t *testing.T) {
	adapter, socket := isolateTestTmux(t)
	root := t.TempDir()
	cfg := config.Default()
	cfg.RemoteAvailability = config.AvailabilityWorkOnly
	server, err := newServer(cfg, root, adapter)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	name := "mctrl-ws-raw-" + strings.ReplaceAll(time.Now().UTC().Format("150405.000000"), ".", "")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// The pane echoes the bytes it receives with control characters made
	// visible, so the test observes exactly what the Session's program gets from
	// the phone: a received Return reads back as ^M, a rewritten line feed does
	// not.
	id, err := adapter.CreateManagedSession(ctx, name, t.TempDir(), []string{"/bin/sh", "-c", "printf MCTRL-READY; exec /bin/cat -v"})
	if err != nil {
		t.Fatalf("tmux managed session: %v", err)
	}
	defer testTmuxCommand(socket, "kill-session", "-t", name).Run()
	item := work.Work{
		ID: "work_raw_transport", ProjectID: "project_raw", ProjectPath: t.TempDir(), RunnerID: "shell",
		RequestID: "request_raw", DeviceID: "device_raw", SessionName: name, SessionID: id,
		State: work.StateRunning, CreatedAt: time.Now().UTC(), LaunchStage: work.StageChildStarted, PromptDelivery: work.PromptNone,
	}
	if err := server.works.Save(item); err != nil {
		t.Fatal(err)
	}

	pairing, err := auth.CreatePairing(root, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	pairResult, err := auth.NewRegistry(root).Pair(root, pairing.Token, "Raw Transport Phone")
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server)
	defer httpServer.Close()
	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/api/v1/sessions/" + id + "/terminal"
	header := http.Header{}
	header.Set("Origin", httpServer.URL)
	header.Set("Cookie", auth.SessionCookieName+"="+pairResult.SessionToken)
	conn, _, err := (&websocket.Dialer{HandshakeTimeout: 3 * time.Second}).DialContext(ctx, wsURL, header)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	stream := &terminalStream{conn: conn}
	stream.readUntilContains(t, "MCTRL-READY", 8*time.Second, "the Session program never started")
	if !stream.sawNotice("INPUT_MODE_REPAIRED") {
		t.Fatal("the repair was not reported to the client")
	}
	transport, err := adapter.PaneTransport(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if raw, rawErr := terminal.TransportIsRawPath(transport.TTY); rawErr != nil {
		t.Fatal(rawErr)
	} else if !raw {
		t.Fatalf("pane transport %s is still line-buffered after the repair notice", transport.TTY)
	}

	// A single keystroke must arrive without any Return. A cooked line
	// discipline holds it in the kernel until the user presses Return, which is
	// exactly the reported symptom.
	stream.sync(t)
	stream.roundTrip(t, "z", "z", 4*time.Second, "one keystroke without Return")

	// Return must arrive as Return, exactly once. A cooked line discipline
	// rewrites it into a line feed, which programs bind to "newline" instead of
	// "submit", and echoes the line a second time over the program's output.
	stream.roundTrip(t, "q\r", "q^M", 4*time.Second, "Return")
}

// sync waits until the attachment stops emitting attach-time noise, so the byte
// exact assertions that follow only observe the Session program.
func (s *terminalStream) sync(t *testing.T) {
	t.Helper()
	for attempt := 0; attempt < 6; attempt++ {
		if err := s.conn.WriteMessage(websocket.BinaryMessage, []byte("s\r")); err != nil {
			t.Fatal(err)
		}
		s.readUntilSuffix(t, "s^M", 4*time.Second, "the Session program stopped echoing")
		if s.take() == "s^M" {
			return
		}
	}
	t.Fatal("the terminal stream never settled after attaching")
}

// terminalStream collects the terminal attachment's binary frames and the text
// control frames mctrl sends alongside them. A read deadline must never expire
// between assertions: a gorilla WebSocket cannot be read again after one does,
// so every read is bounded by a positive signal instead of a quiet period.
type terminalStream struct {
	conn    *websocket.Conn
	pending strings.Builder
	notices strings.Builder
}

func (s *terminalStream) readUntilContains(t *testing.T, want string, timeout time.Duration, failure string) {
	t.Helper()
	s.readUntil(t, want, false, timeout, failure)
}

func (s *terminalStream) readUntilSuffix(t *testing.T, want string, timeout time.Duration, failure string) {
	t.Helper()
	s.readUntil(t, want, true, timeout, failure)
}

func (s *terminalStream) readUntil(t *testing.T, want string, requireSuffix bool, timeout time.Duration, failure string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		pending := s.pending.String()
		if (requireSuffix && strings.HasSuffix(pending, want)) || (!requireSuffix && strings.Contains(pending, want)) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s (stream so far: %q)", failure, pending+s.all())
		}
		_ = s.conn.SetReadDeadline(deadline)
		messageType, data, err := s.conn.ReadMessage()
		if err != nil {
			t.Fatalf("%s (stream so far: %q): %v", failure, pending+s.all(), err)
		}
		if messageType == websocket.TextMessage {
			s.notices.Write(data)
			continue
		}
		s.pending.Write(data)
	}
}

// roundTrip sends input and requires the Session program to receive exactly the
// same bytes back, with no line-discipline echo and no rewritten control bytes.
func (s *terminalStream) roundTrip(t *testing.T, input, want string, timeout time.Duration, failure string) {
	t.Helper()
	if err := s.conn.WriteMessage(websocket.BinaryMessage, []byte(input)); err != nil {
		t.Fatal(err)
	}
	s.readUntilSuffix(t, want, timeout, failure+" never reached the Session program")
	if got := s.take(); got != want {
		t.Fatalf("%s: Session program received %q, want %q", failure, got, want)
	}
}

func (s *terminalStream) take() string {
	value := s.pending.String()
	s.pending.Reset()
	return value
}

func (s *terminalStream) discard() {
	s.pending.Reset()
}

func (s *terminalStream) all() string {
	return s.notices.String()
}

func (s *terminalStream) sawNotice(code string) bool {
	return strings.Contains(s.notices.String(), code)
}

func TestTerminalWebSocketLeavesExternalTransportUntouched(t *testing.T) {
	adapter, socket := isolateTestTmux(t)
	root := t.TempDir()
	cfg := config.Default()
	cfg.RemoteAvailability = config.AvailabilityWorkOnly
	server, err := newServer(cfg, root, adapter)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	name := "mctrl-ws-external-" + strings.ReplaceAll(time.Now().UTC().Format("150405.000000"), ".", "")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	id, err := adapter.CreateManagedSession(ctx, name, t.TempDir(), []string{"/bin/sh", "-c", "printf ready; sleep 10"})
	if err != nil {
		t.Fatal(err)
	}
	defer testTmuxCommand(socket, "kill-session", "-t", name).Run()

	pairing, err := auth.CreatePairing(root, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	pairResult, err := auth.NewRegistry(root).Pair(root, pairing.Token, "External Transport Phone")
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server)
	defer httpServer.Close()
	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/api/v1/sessions/" + id + "/terminal"
	header := http.Header{}
	header.Set("Origin", httpServer.URL)
	header.Set("Cookie", auth.SessionCookieName+"="+pairResult.SessionToken)
	conn, _, err := (&websocket.Dialer{HandshakeTimeout: 3 * time.Second}).DialContext(ctx, wsURL, header)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	sawNotice := false
	for {
		messageType, data, readErr := conn.ReadMessage()
		if readErr != nil {
			t.Fatal(readErr)
		}
		if messageType == websocket.TextMessage {
			if strings.Contains(string(data), "INPUT_MODE_REPAIRED") {
				sawNotice = true
			}
			t.Fatalf("an external Session must not receive control notices: %s", data)
		}
		if strings.Contains(string(data), "ready") {
			break
		}
	}
	transport, err := adapter.PaneTransport(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if raw, rawErr := terminal.TransportIsRawPath(transport.TTY); rawErr != nil {
		t.Fatal(rawErr)
	} else if raw {
		t.Fatalf("mctrl reconfigured a terminal it does not own: %s", transport.TTY)
	}
	if sawNotice {
		t.Fatal("an external Session reported a repair it must not receive")
	}
}

func TestCloseSessionRequiresForceForActiveManagedWork(t *testing.T) {
	adapter, socket := isolateTestTmux(t)
	root := t.TempDir()
	cfg := config.Default()
	cfg.RemoteAvailability = config.AvailabilityWorkOnly
	server, err := newServer(cfg, root, adapter)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	name := "mctrl-close-test-" + strings.ReplaceAll(time.Now().UTC().Format("150405.000000"), ".", "")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	id, err := adapter.CreateManagedSession(ctx, name, t.TempDir(), []string{"/bin/sh", "-c", "sleep 10"})
	if err != nil {
		t.Fatal(err)
	}
	defer testTmuxCommand(socket, "kill-session", "-t", name).Run()
	item := work.Work{
		ID: "work_close_test", ProjectID: "project_close", ProjectPath: t.TempDir(), RunnerID: "shell",
		RequestID: "request_close", DeviceID: "device_close", SessionName: name, SessionID: id,
		State: work.StateRunning, CreatedAt: time.Now().UTC(), LaunchStage: work.StageChildStarted, PromptDelivery: work.PromptConfirmed,
	}
	if err := server.works.Save(item); err != nil {
		t.Fatal(err)
	}

	pairing, err := auth.CreatePairing(root, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	pairResult, err := auth.NewRegistry(root).Pair(root, pairing.Token, "Close Test Phone")
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server)
	defer httpServer.Close()
	closeURL := httpServer.URL + "/api/v1/sessions/" + id + "/close"
	request := httptest.NewRequest(http.MethodPost, closeURL, strings.NewReader(`{"force":false}`))
	request.Header.Set("Cookie", auth.SessionCookieName+"="+pairResult.SessionToken)
	request.Header.Set("Origin", httpServer.URL)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(auth.CSRFHeaderName, pairResult.CSRFToken)
	blocked := httptest.NewRecorder()
	server.Handler().ServeHTTP(blocked, request)
	if blocked.Code != http.StatusConflict || !strings.Contains(blocked.Body.String(), "WORK_RUNNING") {
		t.Fatalf("active Work close response = %d %s", blocked.Code, blocked.Body.String())
	}

	forcedRequest := httptest.NewRequest(http.MethodPost, closeURL, strings.NewReader(`{"force":true}`))
	forcedRequest.Header.Set("Cookie", auth.SessionCookieName+"="+pairResult.SessionToken)
	forcedRequest.Header.Set("Origin", httpServer.URL)
	forcedRequest.Header.Set("Content-Type", "application/json")
	forcedRequest.Header.Set(auth.CSRFHeaderName, pairResult.CSRFToken)
	closed := httptest.NewRecorder()
	server.Handler().ServeHTTP(closed, forcedRequest)
	if closed.Code != http.StatusOK || !strings.Contains(closed.Body.String(), `"status":"closed"`) {
		t.Fatalf("forced close response = %d %s", closed.Code, closed.Body.String())
	}
	if exists, existsErr := adapter.SessionExists(context.Background(), id); exists || (existsErr != nil && !errors.Is(existsErr, tmux.ErrNoServer)) {
		t.Fatalf("closed Session still exists: exists=%v err=%v", exists, existsErr)
	}
}

func TestCloseExternalSessionIsAllowedWithoutForce(t *testing.T) {
	adapter, socket := isolateTestTmux(t)
	root := t.TempDir()
	cfg := config.Default()
	cfg.RemoteAvailability = config.AvailabilityWorkOnly
	server, err := newServer(cfg, root, adapter)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	name := "mctrl-external-close-" + strings.ReplaceAll(time.Now().UTC().Format("150405.000000"), ".", "")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	id, err := adapter.CreateManagedSession(ctx, name, t.TempDir(), []string{"/bin/sh", "-c", "sleep 10"})
	if err != nil {
		t.Fatal(err)
	}
	defer testTmuxCommand(socket, "kill-session", "-t", name).Run()

	pairing, err := auth.CreatePairing(root, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	pairResult, err := auth.NewRegistry(root).Pair(root, pairing.Token, "External Close Phone")
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server)
	defer httpServer.Close()
	closeURL := httpServer.URL + "/api/v1/sessions/" + id + "/close"
	request := httptest.NewRequest(http.MethodPost, closeURL, strings.NewReader(`{"force":false}`))
	request.Header.Set("Cookie", auth.SessionCookieName+"="+pairResult.SessionToken)
	request.Header.Set("Origin", httpServer.URL)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(auth.CSRFHeaderName, pairResult.CSRFToken)
	closed := httptest.NewRecorder()
	server.Handler().ServeHTTP(closed, request)
	if closed.Code != http.StatusOK || !strings.Contains(closed.Body.String(), `"status":"closed"`) {
		t.Fatalf("external close response = %d %s", closed.Code, closed.Body.String())
	}
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
