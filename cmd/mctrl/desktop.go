package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"golang.org/x/term"
	"mctrl/internal/config"
	"mctrl/internal/tmux"
)

func directorySessionName(path string) string {
	var label strings.Builder
	for _, char := range filepath.Base(path) {
		if unicode.IsLetter(char) || unicode.IsDigit(char) || char == '-' || char == '_' {
			label.WriteRune(char)
		} else {
			label.WriteByte('-')
		}
		if label.Len() >= 48 {
			break
		}
	}
	name := strings.Trim(label.String(), "-")
	if name == "" {
		name = "work"
	}
	digest := sha256.Sum256([]byte(path))
	return name + "-" + hex.EncodeToString(digest[:6])
}

func desktopAdapter(socket string) (*tmux.Adapter, bool, error) {
	profile, err := config.ActiveProfile()
	if err != nil {
		return nil, false, err
	}
	root, err := config.StateDir()
	if err != nil {
		return nil, false, err
	}
	if err := config.ValidateRuntimeIdentity(root, profile.Name); err != nil {
		return nil, false, err
	}
	adapter := tmux.New()
	if socket != "" {
		adapter = tmux.NewWithSocket(socket)
	} else {
		connection, readErr := config.LoadDesktopConnection(root, profile.Name)
		if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
			return nil, false, readErr
		}
		environmentSocket := strings.TrimSpace(os.Getenv("MCTRL_TMUX_SOCKET"))
		if readErr == nil && (environmentSocket == "" || environmentSocket == profile.DefaultTmuxSocket) {
			adapter = tmux.NewWithSocket(connection.Socket)
		}
	}
	current, err := tmux.ClientSocket(os.Getenv("TMUX"))
	if err != nil {
		return nil, false, err
	}
	if current != "" && filepath.Clean(current) != filepath.Clean(adapter.Socket()) {
		return nil, false, errors.New("this terminal is inside another tmux server; run this command in a new WezTerm tab")
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return nil, false, errors.New("open this command in an interactive terminal such as WezTerm")
	}
	return adapter, current != "", nil
}

func connectDesktop(adapter *tmux.Adapter, session tmux.Session, inside bool) error {
	if inside {
		current, err := adapter.SessionForPane(context.Background(), os.Getenv("TMUX_PANE"))
		if err != nil || current != session.ID {
			return errors.New("open a new WezTerm tab to continue this session; your current tmux session was kept")
		}
		fmt.Printf("Already in %s. Continue your harness here or open it on the phone.\n", session.Name)
		return nil
	}
	command, err := adapter.DesktopClientCommand(context.Background(), session.ID)
	if err != nil {
		return err
	}
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	fmt.Printf("Continuing %s. Find this session in the phone App.\n", session.Name)
	fmt.Println("To leave it running: Ctrl+B, then D.")
	return command.Run()
}

func runHere(args []string) error {
	fs := flagSet("here")
	socket := fs.String("socket", "", "override the recorded Mac connection")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("usage: mctrl here (run from your project directory)")
	}
	adapter, inside, err := desktopAdapter(*socket)
	if err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	cwd, err = filepath.EvalSymlinks(cwd)
	if err != nil {
		return err
	}
	var session tmux.Session
	if inside {
		session, err = adapter.InspectSession(context.Background(), directorySessionName(cwd))
		if err != nil {
			return errors.New("open a new WezTerm tab to start a session for this directory; your current session was kept")
		}
	} else {
		session, err = adapter.EnsureDesktopSession(context.Background(), directorySessionName(cwd), cwd)
	}
	if err != nil {
		return err
	}
	return connectDesktop(adapter, session, inside)
}

func runResume(args []string) error {
	fs := flagSet("resume")
	socket := fs.String("socket", "", "tmux connection from the phone")
	name := fs.String("name", "", "expected Session name from the phone")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 || *name == "" {
		return errors.New("use the complete mctrl resume command copied from the phone")
	}
	adapter, inside, err := desktopAdapter(*socket)
	if err != nil {
		return err
	}
	session, err := adapter.InspectSession(context.Background(), fs.Arg(0))
	if err != nil {
		return fmt.Errorf("this session is no longer available: %w", err)
	}
	if session.Name != *name {
		return errors.New("this Session identity has changed; copy its current command from the phone")
	}
	return connectDesktop(adapter, session, inside)
}
