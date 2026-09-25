package tmux

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

var (
	ErrUnavailable     = errors.New("tmux unavailable")
	ErrNoServer        = errors.New("tmux server not running")
	ErrSessionNotFound = errors.New("tmux session not found")
)

type Adapter struct {
	binary string
	socket string
}

func New() *Adapter {
	socket := strings.TrimSpace(os.Getenv("MCTRL_TMUX_SOCKET"))
	if socket == "" && runtime.GOOS == "darwin" {
		socket = fmt.Sprintf("/private/tmp/tmux-%d/default", os.Getuid())
	}
	return NewWithSocket(socket)
}

// NewWithSocket creates an adapter for an explicit tmux endpoint. It is used by
// isolated integration tests; normal production callers should use New.
func NewWithSocket(socket string) *Adapter {
	path, err := exec.LookPath("tmux")
	if err != nil {
		path = "tmux"
	}
	return &Adapter{binary: path, socket: strings.TrimSpace(socket)}
}

func (a *Adapter) Available() bool {
	_, err := exec.LookPath(a.binary)
	return err == nil
}

type Pane struct {
	ID           string `json:"id"`
	WindowID     string `json:"window_id,omitempty"`
	PID          int    `json:"pid,omitempty"`
	Command      string `json:"command"`
	StartCommand string `json:"start_command,omitempty"`
	Cwd          string `json:"cwd"`
	Active       bool   `json:"active"`
	WindowActive bool   `json:"-"`
	Dead         bool   `json:"dead,omitempty"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
}

type Session struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Windows     int         `json:"windows"`
	Attached    bool        `json:"attached"`
	Panes       []Pane      `json:"-"`
	ActivePane  *Pane       `json:"active_pane,omitempty"`
	ManagedWork interface{} `json:"managed_work,omitempty"`
}

// MarshalJSON exposes the API's scalar pane count while retaining detailed
// pane facts under pane_details for the detail page.
func (s Session) MarshalJSON() ([]byte, error) {
	type response struct {
		ID          string      `json:"id"`
		Name        string      `json:"name"`
		Windows     int         `json:"windows"`
		Attached    bool        `json:"attached"`
		Panes       int         `json:"panes"`
		PaneDetails []Pane      `json:"pane_details,omitempty"`
		ActivePane  *Pane       `json:"active_pane,omitempty"`
		ManagedWork interface{} `json:"managed_work,omitempty"`
	}
	return json.Marshal(response{ID: s.ID, Name: s.Name, Windows: s.Windows, Attached: s.Attached, Panes: len(s.Panes), PaneDetails: s.Panes, ActivePane: s.ActivePane, ManagedWork: s.ManagedWork})
}

func (a *Adapter) ServerAvailable(ctx context.Context) error {
	if !a.Available() {
		return ErrUnavailable
	}
	_, err := a.run(ctx, "list-sessions")
	if err == nil {
		return nil
	}
	if isNoServerError(err) {
		return ErrNoServer
	}
	return err
}

func tmuxErrorText(err error) string {
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	if separator := strings.Index(message, ": "); separator >= 0 {
		return strings.TrimSpace(message[separator+2:])
	}
	return message
}

// isNoServerError recognizes only tmux's server-level output prefixes. Exact
// prefixes prevent a user-controlled Session name from masquerading as a
// server-state error.
func isNoServerError(err error) bool {
	if err == nil {
		return false
	}
	message := tmuxErrorText(err)
	if strings.HasPrefix(message, "no server running on ") || message == "no sessions" {
		return true
	}
	if !strings.HasPrefix(message, "error connecting to ") {
		return false
	}
	return strings.HasSuffix(message, "(no such file or directory)") ||
		strings.HasSuffix(message, "(connection refused)")
}

func recordFormat(fields []string) (string, string, error) {
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return "", "", fmt.Errorf("create tmux record delimiter: %w", err)
	}
	delimiter := "__mctrl_" + hex.EncodeToString(random) + "__"
	return strings.Join(fields, delimiter), delimiter, nil
}

func parseRecords(output, delimiter string, expectedFields int) ([][]string, error) {
	trimmed := strings.TrimSuffix(output, "\n")
	if trimmed == "" {
		return nil, nil
	}
	records := strings.Split(trimmed, "\n")
	result := make([][]string, 0, len(records))
	for _, record := range records {
		values := strings.Split(record, delimiter)
		if len(values) != expectedFields {
			return nil, fmt.Errorf("tmux record has %d fields; expected %d", len(values), expectedFields)
		}
		result = append(result, values)
	}
	return result, nil
}

func (a *Adapter) ListSessions(ctx context.Context) ([]Session, error) {
	if !a.Available() {
		return nil, ErrUnavailable
	}
	format, delimiter, err := recordFormat([]string{"#{session_id}", "#{session_name}", "#{session_windows}", "#{session_attached}"})
	if err != nil {
		return nil, err
	}
	output, err := a.run(ctx, "list-sessions", "-F", format)
	if err != nil {
		// tmux starts on demand, so a stopped server is a valid empty list.
		if isNoServerError(err) {
			return []Session{}, nil
		}
		log.Printf("tmux list-sessions failed: %v", err)
		return nil, err
	}
	records, err := parseRecords(output, delimiter, 4)
	if err != nil {
		return nil, fmt.Errorf("parse tmux list-sessions: %w", err)
	}
	result := make([]Session, 0, len(records))
	for _, values := range records {
		windows, _ := strconv.Atoi(values[2])
		result = append(result, Session{ID: values[0], Name: values[1], Windows: windows, Attached: values[3] == "1"})
	}
	return result, nil
}

func (a *Adapter) InspectSession(ctx context.Context, id string) (Session, error) {
	if !a.Available() {
		return Session{}, ErrUnavailable
	}
	if err := a.ServerAvailable(ctx); err != nil {
		return Session{}, err
	}
	sessions, err := a.ListSessions(ctx)
	if err != nil {
		return Session{}, err
	}
	var selected *Session
	for index := range sessions {
		if sessions[index].ID == id || sessions[index].Name == id {
			selected = &sessions[index]
			break
		}
	}
	if selected == nil {
		return Session{}, ErrSessionNotFound
	}
	panes, err := a.listPanes(ctx, selected.ID)
	if err != nil {
		return Session{}, err
	}
	selected.Panes = panes
	for index := range panes {
		if panes[index].WindowActive && panes[index].Active {
			pane := panes[index]
			selected.ActivePane = &pane
			break
		}
	}
	if selected.ActivePane == nil {
		for index := range panes {
			if panes[index].Active {
				pane := panes[index]
				selected.ActivePane = &pane
				break
			}
		}
	}
	if selected.ActivePane == nil && len(panes) > 0 {
		pane := panes[0]
		selected.ActivePane = &pane
	}
	return *selected, nil
}

func (a *Adapter) listPanes(ctx context.Context, sessionID string) ([]Pane, error) {
	format, delimiter, err := recordFormat([]string{
		"#{pane_id}", "#{window_id}", "#{window_active}", "#{pane_active}", "#{pane_dead}", "#{pane_pid}",
		"#{pane_current_command}", "#{pane_current_path}", "#{pane_width}", "#{pane_height}", "#{pane_start_command}",
	})
	if err != nil {
		return nil, err
	}
	output, err := a.run(ctx, "list-panes", "-s", "-t", sessionID, "-F", format)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "can't find") || strings.Contains(strings.ToLower(err.Error()), "no such") {
			return nil, ErrSessionNotFound
		}
		return nil, err
	}
	records, err := parseRecords(output, delimiter, 11)
	if err != nil {
		return nil, fmt.Errorf("parse tmux list-panes: %w", err)
	}
	result := make([]Pane, 0, len(records))
	for _, values := range records {
		pid, _ := strconv.Atoi(values[5])
		width, _ := strconv.Atoi(values[8])
		height, _ := strconv.Atoi(values[9])
		result = append(result, Pane{
			ID: values[0], WindowID: values[1], WindowActive: values[2] == "1",
			Active: values[3] == "1", Dead: values[4] == "1", PID: pid,
			Command: values[6], Cwd: values[7], Width: width, Height: height,
			StartCommand: values[10],
		})
	}
	return result, nil
}

func (a *Adapter) CapturePane(ctx context.Context, paneID string, lines int) ([]string, error) {
	if lines < 1 {
		lines = 1
	}
	if lines > 500 {
		lines = 500
	}
	output, err := a.run(ctx, "capture-pane", "-p", "-t", paneID, "-S", fmt.Sprintf("-%d", lines))
	if err != nil {
		return nil, err
	}
	raw := strings.Split(strings.TrimSuffix(output, "\n"), "\n")
	if len(raw) > lines {
		raw = raw[len(raw)-lines:]
	}
	for index := range raw {
		raw[index] = StripANSI(raw[index])
		if len(raw[index]) > 4096 {
			raw[index] = raw[index][:4096]
		}
	}
	return raw, nil
}

func (a *Adapter) SessionExists(ctx context.Context, id string) (bool, error) {
	if !a.Available() {
		return false, ErrUnavailable
	}
	if err := a.ServerAvailable(ctx); err != nil {
		return false, err
	}
	sessions, err := a.ListSessions(ctx)
	if err != nil {
		return false, err
	}
	for _, session := range sessions {
		if session.ID == id || session.Name == id {
			return true, nil
		}
	}
	return false, nil
}

func (a *Adapter) SessionMatches(ctx context.Context, id, expectedName string) (bool, error) {
	session, err := a.InspectSession(ctx, id)
	if errors.Is(err, ErrSessionNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(expectedName) == "" || session.Name == expectedName, nil
}

func isSessionNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(tmuxErrorText(err))
	return strings.Contains(message, "can't find session") ||
		strings.Contains(message, "session not found") ||
		strings.Contains(message, "no such session")
}

func (a *Adapter) CloseSession(ctx context.Context, id string) error {
	session, err := a.InspectSession(ctx, id)
	if err != nil {
		return err
	}
	if _, err := a.run(ctx, "kill-session", "-t", session.ID); err != nil {
		if isSessionNotFoundError(err) {
			return ErrSessionNotFound
		}
		return err
	}
	return nil
}

func (a *Adapter) CreateManagedSession(ctx context.Context, name, cwd string, command []string) (string, error) {
	if !a.Available() {
		return "", ErrUnavailable
	}
	if strings.TrimSpace(name) == "" || len(command) == 0 {
		return "", fmt.Errorf("managed session name and command are required")
	}
	if _, err := a.run(ctx, "new-session", "-d", "-s", name, "-c", cwd); err != nil {
		return "", err
	}
	// Resolve the stable session id before starting the one-shot runner. Set
	// remain-on-exit first so even an immediate runner exit cannot destroy the
	// diagnostic Session.
	sessions, err := a.ListSessions(ctx)
	if err != nil {
		return "", err
	}
	var sessionID string
	for _, session := range sessions {
		if session.Name == name {
			sessionID = session.ID
			break
		}
	}
	if sessionID == "" {
		return "", fmt.Errorf("created session %q but could not resolve its id", name)
	}
	if _, err := a.run(ctx, "set-option", "-t", sessionID, "remain-on-exit", "on"); err != nil {
		return sessionID, err
	}
	inspected, err := a.InspectSession(ctx, sessionID)
	if err != nil {
		return sessionID, err
	}
	if inspected.ActivePane == nil {
		return sessionID, fmt.Errorf("created session %q has no active pane", name)
	}
	respawn := []string{"respawn-pane", "-k", "-t", inspected.ActivePane.ID, "--"}
	respawn = append(respawn, command...)
	if _, err := a.run(ctx, respawn...); err != nil {
		return sessionID, err
	}
	return sessionID, nil
}

func (a *Adapter) RespawnPane(ctx context.Context, paneID string, command []string) error {
	if len(command) == 0 {
		return fmt.Errorf("respawn command is required")
	}
	args := []string{"respawn-pane", "-k", "-t", paneID, "--"}
	args = append(args, command...)
	_, err := a.run(ctx, args...)
	return err
}

func (a *Adapter) ApplySizingPolicy(ctx context.Context, id string) error {
	_, err := a.run(ctx, "set-option", "-t", id, "window-size", "largest")
	return err
}

// SendPrompt performs the dedicated structured prompt action. It targets the
// active pane explicitly and sends literal text followed by Enter; callers
// must persist the request id before invoking it so an uncertain outcome is
// never blindly retried.
func (a *Adapter) SendPrompt(ctx context.Context, sessionID, text string) error {
	session, err := a.InspectSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if session.ActivePane == nil {
		return fmt.Errorf("session has no active pane")
	}
	if _, err := a.run(ctx, "send-keys", "-t", session.ActivePane.ID, "-l", "--", text); err != nil {
		return err
	}
	_, err = a.run(ctx, "send-keys", "-t", session.ActivePane.ID, "Enter")
	return err
}

// PaneTransport describes the terminal device behind a Session's active pane
// and the process that owns it. mctrl uses it to decide whether the pane's
// line discipline belongs to mctrl (a Managed Work runner) or to the user's own
// terminal, which mctrl must never reconfigure.
type PaneTransport struct {
	TTY     string
	Command string
	PID     int
}

// PaneTransport reports the active pane's terminal device and owning process.
func (a *Adapter) PaneTransport(ctx context.Context, sessionID string) (PaneTransport, error) {
	if !a.Available() {
		return PaneTransport{}, ErrUnavailable
	}
	format, delimiter, err := recordFormat([]string{"#{pane_tty}", "#{pane_current_command}", "#{pane_pid}"})
	if err != nil {
		return PaneTransport{}, err
	}
	output, err := a.run(ctx, "display-message", "-p", "-t", sessionID, format)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "can't find") || strings.Contains(strings.ToLower(err.Error()), "no such") {
			return PaneTransport{}, ErrSessionNotFound
		}
		return PaneTransport{}, err
	}
	records, err := parseRecords(output, delimiter, 3)
	if err != nil {
		return PaneTransport{}, fmt.Errorf("parse tmux display-message: %w", err)
	}
	if len(records) == 0 {
		return PaneTransport{}, ErrSessionNotFound
	}
	pid, _ := strconv.Atoi(records[0][2])
	return PaneTransport{TTY: records[0][0], Command: records[0][1], PID: pid}, nil
}

func (a *Adapter) AttachCommand(ctx context.Context, id string) (*exec.Cmd, error) {
	if !a.Available() {
		return nil, ErrUnavailable
	}
	args := a.commandArgs("attach-session", "-t", id)
	cmd := exec.CommandContext(ctx, a.binary, args...)
	cmd.Env = append(os.Environ(),
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
		"LANG=en_US.UTF-8",
		"LC_ALL=en_US.UTF-8",
	)
	return cmd, nil
}

func (a *Adapter) commandArgs(args ...string) []string {
	if strings.TrimSpace(a.socket) == "" {
		return append([]string(nil), args...)
	}
	result := []string{"-S", a.socket}
	return append(result, args...)
}

func (a *Adapter) run(ctx context.Context, args ...string) (string, error) {
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, a.binary, a.commandArgs(args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		command := "tmux"
		if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
			command += " " + args[0]
		}
		return "", fmt.Errorf("%s: %s", command, message)
	}
	return string(output), nil
}

// StripANSI removes the common CSI/OSC control sequences that should never
// be sent to the mobile preview. It intentionally leaves ordinary text and
// tabs/newlines untouched.
func StripANSI(value string) string {
	var b strings.Builder
	b.Grow(len(value))
	for index := 0; index < len(value); {
		if value[index] != 0x1b || index+1 >= len(value) {
			b.WriteByte(value[index])
			index++
			continue
		}
		next := value[index+1]
		switch next {
		case '[':
			index += 2
			for index < len(value) {
				current := value[index]
				index++
				if current >= 0x40 && current <= 0x7e {
					break
				}
			}
		case ']':
			index += 2
			for index < len(value) {
				if value[index] == 0x07 {
					index++
					break
				}
				if value[index] == 0x1b && index+1 < len(value) && value[index+1] == '\\' {
					index += 2
					break
				}
				index++
			}
		case 'P', '^', '_', 'X':
			// DCS/PM/APC/SOS payloads are terminated by ST (ESC \\) or BEL.
			// Drop the entire payload instead of leaking it into previews.
			index += 2
			for index < len(value) {
				if value[index] == 0x07 {
					index++
					break
				}
				if value[index] == 0x1b && index+1 < len(value) && value[index+1] == '\\' {
					index += 2
					break
				}
				index++
			}
		default:
			index += 2
		}
	}
	return b.String()
}
