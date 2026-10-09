package tmux

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func (a *Adapter) Socket() string { return a.socket }

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

// This is a read-only local command, not a browser execution capability.
// Match the observed name as well as the ID before a desktop client attaches.
func (a *Adapter) DesktopResumeCommand(profile, id, name string) string {
	command := "mctrl"
	if profile != "" && profile != "v1" {
		command += " --profile " + shellQuote(profile)
	}
	command += " resume"
	if a.socket != "" {
		command += " --socket " + shellQuote(a.socket)
	}
	return command + " --name " + shellQuote(name) + " " + shellQuote(id)
}

// EnsureDesktopSession never respawns a pane or detaches another client.
// The second lookup handles two local invocations creating the same directory
// session concurrently without repeating input or creating another session.
func (a *Adapter) EnsureDesktopSession(ctx context.Context, name, cwd string) (Session, error) {
	if !a.Available() {
		return Session{}, ErrUnavailable
	}
	session, err := a.InspectSession(ctx, name)
	if err == nil {
		return session, nil
	}
	if !errors.Is(err, ErrNoServer) && !errors.Is(err, ErrSessionNotFound) {
		return Session{}, err
	}
	if _, err := a.run(ctx, "new-session", "-d", "-s", name, "-c", cwd); err != nil {
		if existing, lookupErr := a.InspectSession(ctx, name); lookupErr == nil {
			return existing, nil
		}
		return Session{}, err
	}
	session, err = a.InspectSession(ctx, name)
	if err != nil {
		return Session{}, err
	}
	if err := a.ApplySizingPolicy(ctx, session.ID); err != nil {
		return Session{}, err
	}
	return session, nil
}

// The desktop keeps its own terminal capabilities and terminal device.
// AttachCommand is separately configured for the phone's PTY transport.
func (a *Adapter) DesktopClientCommand(ctx context.Context, id string) (*exec.Cmd, error) {
	if !a.Available() {
		return nil, ErrUnavailable
	}
	command := exec.CommandContext(ctx, a.binary, a.commandArgs("attach-session", "-t", id)...)
	command.Env = os.Environ()
	return command, nil
}

func (a *Adapter) SessionForPane(ctx context.Context, pane string) (string, error) {
	if pane == "" {
		return "", errors.New("the current tmux pane is unavailable")
	}
	value, err := a.run(ctx, "display-message", "-p", "-t", pane, "#{session_id}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(value), nil
}

// TMUX has two numeric suffixes; the socket itself can contain commas.
func ClientSocket(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	last := strings.LastIndex(value, ",")
	if last < 0 {
		return "", fmt.Errorf("could not identify the current tmux connection")
	}
	previous := strings.LastIndex(value[:last], ",")
	if previous < 1 {
		return "", fmt.Errorf("could not identify the current tmux connection")
	}
	return value[:previous], nil
}
