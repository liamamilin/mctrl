package terminal

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
	"golang.org/x/term"

	"mctrl/internal/tmux"
)

type Bridge struct {
	tmux *tmux.Adapter

	mu           sync.Mutex
	nextID       uint64
	connections  map[uint64]connection
	deviceActive func(string) bool
	managedWork  func(string) bool
}

type connection struct {
	deviceID  string
	sessionID string
	conn      *websocket.Conn
}

// NewBridge creates the disposable terminal attachment surface. managedWork
// reports whether a Session hosts non-terminal Managed Work, which is what
// makes its pane's line discipline mctrl's to maintain.
func NewBridge(adapter *tmux.Adapter, managedWork func(string) bool, activeCheck ...func(string) bool) *Bridge {
	bridge := &Bridge{tmux: adapter, connections: make(map[uint64]connection), managedWork: managedWork}
	if len(activeCheck) > 0 {
		bridge.deviceActive = activeCheck[0]
	}
	return bridge
}

type resizeMessage struct {
	Type string `json:"type"`
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

type errorMessage struct {
	Type string `json:"type"`
	Code string `json:"code"`
}

const (
	terminalAttachFailed = "TERMINAL_ATTACH_FAILED"
	sessionGone          = "SESSION_GONE"
)

// noticeMessage tells the client that mctrl changed something on the Mac while
// the attachment was live. It never blocks input and never ends the
// attachment.
type noticeMessage struct {
	Type    string `json:"type"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

const noticeInputModeRepaired = "INPUT_MODE_REPAIRED"

// runnerProcessName is the pane command tmux reports for a Managed Work pane.
// It is a secondary signal only: the Work record decides, and the command is
// used when the Work store cannot be read.
const runnerProcessName = "mctrl-runner"

func ownsRunnerCommand(command string) bool {
	return strings.TrimSpace(command) == runnerProcessName
}

// ownsPaneTransport reports whether mctrl, rather than the user's own terminal,
// is responsible for the pane's line discipline. A pane that belongs to the
// user's terminal keeps that terminal's settings, always.
func (b *Bridge) ownsPaneTransport(sessionID string, transport tmux.PaneTransport) bool {
	if b.managedWork != nil && b.managedWork(sessionID) {
		return true
	}
	return ownsRunnerCommand(transport.Command)
}

func terminalExitCode(exists bool, err error) string {
	if errors.Is(err, tmux.ErrNoServer) || (err == nil && !exists) {
		return sessionGone
	}
	return terminalAttachFailed
}

func (b *Bridge) sessionExitCode(ctx context.Context, sessionID string) string {
	exists, err := b.tmux.SessionExists(ctx, sessionID)
	return terminalExitCode(exists, err)
}

// repairManagedWorkTransport returns a notice only when it actually changed a
// pane that mctrl owns. Every other outcome, including any failure, returns nil:
// an attachment must never depend on a transport repair.
func (b *Bridge) repairManagedWorkTransport(ctx context.Context, sessionID string) *noticeMessage {
	probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	transport, err := b.tmux.PaneTransport(probeCtx, sessionID)
	if err != nil {
		log.Printf("terminal: pane transport unavailable for %s: %v", sessionID, err)
		return nil
	}
	if !b.ownsPaneTransport(sessionID, transport) {
		return nil
	}
	repaired, err := EnsureRawTransportPath(transport.TTY)
	if err != nil {
		log.Printf("terminal: repair pane transport for %s failed: %v", sessionID, err)
		return nil
	}
	if !repaired {
		return nil
	}
	return &noticeMessage{
		Type:    "notice",
		Code:    noticeInputModeRepaired,
		Message: "This Session's keyboard transport was line-buffered on the Mac. mctrl reset it to raw, so keys now reach the program immediately and Return arrives as Return.",
	}
}

func (b *Bridge) register(deviceID, sessionID string, conn *websocket.Conn) uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.connections == nil {
		b.connections = make(map[uint64]connection)
	}
	b.nextID++
	id := b.nextID
	b.connections[id] = connection{deviceID: deviceID, sessionID: sessionID, conn: conn}
	return id
}

func (b *Bridge) unregister(id uint64) {
	b.mu.Lock()
	delete(b.connections, id)
	b.mu.Unlock()
}

// CloseDevice terminates only mctrl's disposable terminal attachments. It
// never asks tmux or a Managed Work runner to stop.
func (b *Bridge) CloseDevice(deviceID string) {
	b.mu.Lock()
	toClose := make([]*websocket.Conn, 0)
	for _, item := range b.connections {
		if item.deviceID == deviceID {
			toClose = append(toClose, item.conn)
		}
	}
	b.mu.Unlock()
	for _, conn := range toClose {
		_ = conn.Close()
	}
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     func(*http.Request) bool { return true }, // API validates before calling ServeHTTP.
}

func startRawPTY(cmd *exec.Cmd, size pty.Winsize) (*os.File, error) {
	ptmx, tty, err := pty.Open()
	if err != nil {
		return nil, err
	}
	// Configure the slave before the child starts. This closes the race where
	// terminal capability replies can be echoed by the default line
	// discipline before tmux puts its own client into raw mode.
	if _, err := term.MakeRaw(int(tty.Fd())); err != nil {
		_ = ptmx.Close()
		_ = tty.Close()
		return nil, err
	}
	if err := pty.Setsize(ptmx, &size); err != nil {
		_ = ptmx.Close()
		_ = tty.Close()
		return nil, err
	}
	cmd.Stdin = tty
	cmd.Stdout = tty
	cmd.Stderr = tty
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	if err := cmd.Start(); err != nil {
		_ = ptmx.Close()
		_ = tty.Close()
		return nil, err
	}
	_ = tty.Close()
	return ptmx, nil
}

func (b *Bridge) ServeHTTP(w http.ResponseWriter, r *http.Request, sessionID, deviceID string) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	connectionID := b.register(deviceID, sessionID, conn)
	defer b.unregister(connectionID)
	conn.SetReadLimit(1 << 20)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	var writeMu sync.Mutex
	writeControl := func(value interface{}) error {
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		writeMu.Lock()
		defer writeMu.Unlock()
		return conn.WriteMessage(websocket.TextMessage, data)
	}
	if b.deviceActive != nil {
		go func() {
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					if !b.deviceActive(deviceID) {
						_ = writeControl(errorMessage{Type: "error", Code: "DEVICE_REVOKED"})
						_ = conn.Close()
						cancel()
						return
					}
				}
			}
		}()
	}
	if err := b.tmux.ApplySizingPolicy(ctx, sessionID); err != nil {
		checkContext, checkCancel := context.WithTimeout(ctx, 2*time.Second)
		code := b.sessionExitCode(checkContext, sessionID)
		checkCancel()
		_ = writeControl(errorMessage{Type: "error", Code: code})
		return
	}
	cmd, err := b.tmux.AttachCommand(ctx, sessionID)
	if err != nil {
		_ = writeControl(errorMessage{Type: "error", Code: terminalAttachFailed})
		return
	}
	ptmx, err := startRawPTY(cmd, pty.Winsize{Cols: 120, Rows: 40})
	if err != nil {
		_ = writeControl(errorMessage{Type: "error", Code: terminalAttachFailed})
		return
	}
	defer ptmx.Close()

	// A Managed Work runner is responsible for keeping its pane raw, but a
	// runner that predates that contract cannot repair itself while it is
	// already running. Repair the transport here and tell the client, instead of
	// letting a cooked line discipline silently buffer and rewrite keystrokes.
	if notice := b.repairManagedWorkTransport(ctx, sessionID); notice != nil {
		_ = writeControl(notice)
	}

	writeBinary := func(data []byte) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return conn.WriteMessage(websocket.BinaryMessage, data)
	}

	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		buffer := make([]byte, 32*1024)
		for {
			n, readErr := ptmx.Read(buffer)
			if n > 0 {
				if err := writeBinary(buffer[:n]); err != nil {
					return
				}
			}
			if readErr != nil {
				return
			}
		}
	}()

	go func() {
		defer cancel()
		for {
			if b.deviceActive != nil && !b.deviceActive(deviceID) {
				_ = writeControl(errorMessage{Type: "error", Code: "DEVICE_REVOKED"})
				return
			}
			messageType, data, readErr := conn.ReadMessage()
			if readErr != nil {
				return
			}
			if messageType == websocket.BinaryMessage {
				if _, writeErr := ptmx.Write(data); writeErr != nil {
					return
				}
				continue
			}
			if messageType != websocket.TextMessage {
				continue
			}
			var resize resizeMessage
			if json.Unmarshal(data, &resize) == nil && resize.Type == "resize" && resize.Cols > 0 && resize.Rows > 0 {
				_ = pty.Setsize(ptmx, &pty.Winsize{Cols: resize.Cols, Rows: resize.Rows})
			}
		}
	}()

	select {
	case <-ctx.Done():
	case <-readDone:
		// Distinguish a dead attach process from a genuinely gone Session.
		// Keep the protocol factual and never kill the Session from here.
		checkContext, checkCancel := context.WithTimeout(context.Background(), 2*time.Second)
		code := b.sessionExitCode(checkContext, sessionID)
		checkCancel()
		_ = writeControl(errorMessage{Type: "error", Code: code})
		cancel()
	}
	_ = cmd.Wait()
}
