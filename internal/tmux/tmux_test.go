package tmux

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
)

func TestStripANSI(t *testing.T) {
	input := "\x1b[31mred\x1b[0m plain\x1b]0;title\x07"
	got := StripANSI(input)
	if strings.Contains(got, "\x1b") || got != "red plain" {
		t.Fatalf("StripANSI() = %q", got)
	}
}

func TestAdapterUsesExplicitTmuxSocket(t *testing.T) {
	t.Setenv("MCTRL_TMUX_SOCKET", "/private/tmp/custom-tmux.sock")
	adapter := New()
	args := adapter.commandArgs("list-sessions")
	if len(args) != 3 || args[0] != "-S" || args[1] != "/private/tmp/custom-tmux.sock" || args[2] != "list-sessions" {
		t.Fatalf("tmux command args = %v", args)
	}
}

func TestAbsentTmuxSocketIsEmptyAndUnavailable(t *testing.T) {
	adapter, _ := newTestAdapter(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	sessions, err := adapter.ListSessions(ctx)
	if err != nil || len(sessions) != 0 {
		t.Fatalf("initial list sessions = %#v, err=%v", sessions, err)
	}
	if err := adapter.ServerAvailable(ctx); !errors.Is(err, ErrNoServer) {
		t.Fatalf("server availability error = %v", err)
	}
	exists, err := adapter.SessionExists(ctx, "missing")
	if exists || !errors.Is(err, ErrNoServer) {
		t.Fatalf("session existence = %v, err=%v", exists, err)
	}
}

func TestNoServerErrorClassificationAnchorsTmuxOutput(t *testing.T) {
	for _, test := range []struct {
		err   error
		match bool
	}{
		{errors.New("tmux list-sessions: no server running on /private/tmp/tmux.sock"), true},
		{errors.New("tmux list-sessions: error connecting to /private/tmp/tmux.sock (No such file or directory)"), true},
		{errors.New("tmux list-sessions: error connecting to /private/tmp/tmux.sock (Connection refused)"), true},
		{errors.New("tmux has-session: can't find session: no server running"), false},
		{errors.New("tmux has-session: can't find session: no sessions"), false},
		{errors.New("tmux has-session: permission denied"), false},
	} {
		if got := isNoServerError(test.err); got != test.match {
			t.Fatalf("isNoServerError(%q) = %v", test.err, got)
		}
	}
}

func TestSessionNamesCannotMasqueradeAsNoServer(t *testing.T) {
	adapter, _ := newTestAdapter(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	keepAlive := "mctrl-keepalive-" + strings.ReplaceAll(time.Now().UTC().Format("150405.000000"), ".", "")
	if _, err := adapter.CreateManagedSession(ctx, keepAlive, t.TempDir(), []string{"/bin/sh", "-c", "sleep 30"}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"no server running", "no sessions", "error connecting to /tmp/fake"} {
		exists, err := adapter.SessionExists(ctx, name)
		if err != nil || exists {
			t.Fatalf("Session name %q was misclassified: exists=%v err=%v", name, exists, err)
		}
	}
}

func TestRecordFormatAndParser(t *testing.T) {
	fields := []string{"$1", "session name", "2", "1"}
	format, delimiter, err := recordFormat(fields)
	if err != nil {
		t.Fatal(err)
	}
	output := format + "\n" + strings.ReplaceAll(format, "$1", "$2") + "\n"
	records, err := parseRecords(output, delimiter, 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || len(records[0]) != 4 || records[0][1] != "session name" || records[1][0] != "$2" {
		t.Fatalf("parsed records = %#v", records)
	}
}

func TestMalformedTmuxRecordDoesNotExposeOutput(t *testing.T) {
	_, err := parseRecords("private-terminal-output\n", "__missing__", 2)
	if err == nil {
		t.Fatal("expected malformed record error")
	}
	if strings.Contains(err.Error(), "private-terminal-output") {
		t.Fatalf("parse error exposed tmux output: %v", err)
	}
}

func TestErrorsDoNotExposeTerminalArguments(t *testing.T) {
	root := t.TempDir()
	fake := filepath.Join(root, "tmux")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\necho 'tmux unavailable' >&2\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", root)
	adapter := New()
	_, err := adapter.run(context.Background(), "send-keys", "-l", "--", "private prompt")
	if err == nil {
		t.Fatal("expected fake tmux failure")
	}
	if strings.Contains(err.Error(), "private prompt") {
		t.Fatalf("tmux error exposed terminal input: %v", err)
	}
}

func TestInspectSessionUsesActiveWindowPane(t *testing.T) {
	adapter, socket := newTestAdapter(t)
	name := "mctrl-pane-test-" + strings.ReplaceAll(time.Now().UTC().Format("150405.000000"), ".", "")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := adapter.run(ctx, "new-session", "-d", "-s", name, "-n", "one", "sleep 5"); err != nil {
		t.Fatalf("tmux new session: %v", err)
	}
	defer testTmuxCommand(socket, "kill-session", "-t", name).Run()
	if _, err := adapter.run(ctx, "new-window", "-t", name, "-n", "two", "sleep 5"); err != nil {
		t.Fatal(err)
	}
	sessions, err := adapter.ListSessions(ctx)
	if err != nil || len(sessions) == 0 {
		t.Fatalf("list sessions: %v", err)
	}
	var id string
	for _, session := range sessions {
		if session.Name == name {
			id = session.ID
		}
	}
	session, err := adapter.InspectSession(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(session.Panes) != 2 || session.ActivePane == nil || !session.ActivePane.WindowActive {
		t.Fatalf("active pane selection is wrong: %+v", session)
	}
}

func TestPhoneAttachDoesNotShrinkDesktopLayout(t *testing.T) {
	adapter, socket := newTestAdapter(t)
	name := "mctrl-size-test-" + strings.ReplaceAll(time.Now().UTC().Format("150405.000000"), ".", "")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_, createErr := adapter.CreateManagedSession(ctx, name, t.TempDir(), []string{"/bin/sh", "-c", "sleep 30"})
	defer testTmuxCommand(socket, "kill-session", "-t", name).Run()
	if createErr != nil {
		t.Fatal(createErr)
	}
	if err := adapter.ApplySizingPolicy(ctx, name); err != nil {
		t.Fatal(err)
	}

	startClient := func(cols, rows uint16) func() {
		attachCtx, attachCancel := context.WithCancel(context.Background())
		cmd, err := adapter.AttachCommand(attachCtx, name)
		if err != nil {
			attachCancel()
			t.Fatal(err)
		}
		ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: cols, Rows: rows})
		if err != nil {
			attachCancel()
			t.Fatal(err)
		}
		go func() { _, _ = io.Copy(io.Discard, ptmx) }()
		return func() {
			attachCancel()
			_ = ptmx.Close()
			_ = cmd.Wait()
		}
	}
	stopDesktop := startClient(120, 40)
	defer stopDesktop()
	stopPhone := startClient(44, 22)
	defer stopPhone()

	deadline := time.Now().Add(5 * time.Second)
	var width, height int
	for time.Now().Before(deadline) {
		session, inspectErr := adapter.InspectSession(ctx, name)
		if inspectErr == nil && session.ActivePane != nil {
			width, height = session.ActivePane.Width, session.ActivePane.Height
			if width >= 100 && height >= 30 {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if width < 100 || height < 30 {
		t.Fatalf("phone attachment shrank desktop-oriented pane to %dx%d", width, height)
	}
}

func TestAttachCommandForcesUTF8Locale(t *testing.T) {
	adapter, _ := newTestAdapter(t)
	cmd, err := adapter.AttachCommand(context.Background(), "missing-session-for-env-check")
	if err != nil {
		t.Fatal(err)
	}
	environment := make(map[string]string)
	for _, entry := range cmd.Env {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) == 2 {
			environment[parts[0]] = parts[1]
		}
	}
	if environment["TERM"] != "xterm-256color" || environment["LANG"] != "en_US.UTF-8" || environment["LC_ALL"] != "en_US.UTF-8" {
		t.Fatalf("attach environment = %#v", environment)
	}
}

func TestRealTmuxSessionLifecycle(t *testing.T) {
	adapter, socket := newTestAdapter(t)
	name := "mctrl-test-" + strings.ReplaceAll(time.Now().UTC().Format("150405.000000"), ".", "")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	id, err := adapter.CreateManagedSession(ctx, name, t.TempDir(), []string{"/bin/sh", "-c", "sleep 1"})
	defer func() {
		_ = testTmuxCommand(socket, "kill-session", "-t", name).Run()
	}()
	if err != nil {
		t.Fatalf("tmux managed session: %v", err)
	}
	if id == "" {
		t.Fatal("created session has no stable id")
	}
	session, err := adapter.InspectSession(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if session.Name != name || session.ActivePane == nil || session.ActivePane.PID <= 0 {
		t.Fatalf("unexpected session: %+v", session)
	}
	if err := adapter.ApplySizingPolicy(ctx, id); err != nil {
		t.Fatalf("apply sizing policy: %v", err)
	}
	policy, err := testTmuxCommand(socket, "show-options", "-v", "-t", id, "window-size").Output()
	if err != nil || strings.TrimSpace(string(policy)) != "largest" {
		t.Fatalf("window-size policy = %q, err=%v", strings.TrimSpace(string(policy)), err)
	}
	lines, err := adapter.CapturePane(ctx, session.ActivePane.ID, 10)
	if err != nil {
		t.Fatalf("capture pane: %v", err)
	}
	if len(lines) == 0 {
		t.Fatal("capture returned no lines")
	}
	time.Sleep(1500 * time.Millisecond)
	if exists, existsErr := adapter.SessionExists(ctx, id); existsErr != nil || !exists {
		t.Fatalf("managed session did not remain after command exit: exists=%v err=%v", exists, existsErr)
	}
}
