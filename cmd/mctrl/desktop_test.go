package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
	"mctrl/internal/config"
	"mctrl/internal/tmux"
)

func TestDirectorySessionNamesAreStableAndSeparateDirectories(t *testing.T) {
	first := filepath.Join(t.TempDir(), "项目 : work")
	second := filepath.Join(t.TempDir(), "项目 : work")
	a, b := directorySessionName(first), directorySessionName(second)
	if a == b || strings.ContainsAny(a, ":. /") || !strings.Contains(a, "项目") {
		t.Fatalf("directory session names = %q, %q", a, b)
	}
	if directorySessionName(first) != a {
		t.Fatal("directory identity changed")
	}
}

// Run the actual entry points with native stdin/stdout PTYs. No browser or
// HTTP action creates these Sessions, and the socket is checked before entry.
func TestDesktopCLIProcess(t *testing.T) {
	if os.Getenv("MCTRL_DESKTOP_TEST_PROCESS") != "1" {
		return
	}
	adapter, _, err := desktopAdapter("")
	if err != nil || adapter.Socket() != os.Getenv("MCTRL_DESKTOP_EXPECTED_SOCKET") {
		fmt.Fprintln(os.Stderr, "unexpected test socket", err)
		os.Exit(2)
	}
	var args []string
	for i, value := range os.Args {
		if value == "--" {
			args = os.Args[i+1:]
			break
		}
	}
	if len(args) == 0 {
		os.Exit(2)
	}
	if args[0] == "here" {
		err = runHere(args[1:])
	} else {
		err = runResume(args[1:])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

func TestDesktopHereAndResumeKeepOneSessionAndItsProcess(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	root, err := os.MkdirTemp("", "mctrl-desktop-")
	if err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(root, "s")
	adapter := tmux.NewWithSocket(socket)
	t.Cleanup(func() { _ = exec.Command("tmux", "-S", socket, "kill-server").Run(); _ = os.RemoveAll(root) })
	if err := config.SaveDesktopConnection(root, "v1", socket); err != nil {
		t.Fatal(err)
	}
	cwd := filepath.Join(root, "项目 'same name'")
	if err := os.Mkdir(cwd, 0700); err != nil {
		t.Fatal(err)
	}
	cwd, err = filepath.EvalSymlinks(cwd)
	if err != nil {
		t.Fatal(err)
	}
	start := func(args ...string) (*exec.Cmd, *os.File, <-chan error) {
		t.Helper()
		command := exec.Command(os.Args[0], append([]string{"-test.run=^TestDesktopCLIProcess$", "--"}, args...)...)
		command.Dir = cwd
		command.Env = append(os.Environ(), "MCTRL_DESKTOP_TEST_PROCESS=1", "MCTRL_DESKTOP_EXPECTED_SOCKET="+socket, "MCTRL_PROFILE=v1", "MCTRL_HOME="+root, "MCTRL_TMUX_SOCKET=", "TMUX=", "TMUX_PANE=", "TERM=xterm-256color", "SHELL=/bin/sh")
		terminal, err := pty.StartWithSize(command, &pty.Winsize{Cols: 80, Rows: 24})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = command.Process.Kill(); _ = terminal.Close() })
		go func() { _, _ = io.Copy(io.Discard, terminal) }()
		done := make(chan error, 1)
		go func() { done <- command.Wait() }()
		return command, terminal, done
	}
	observe := func() tmux.Session {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			session, err := adapter.InspectSession(context.Background(), directorySessionName(cwd))
			if err == nil && session.Attached && session.ActivePane != nil {
				return session
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatal("desktop client did not attach to its directory Session")
		return tmux.Session{}
	}
	detach := func(terminal *os.File, done <-chan error) {
		t.Helper()
		_, _ = terminal.Write([]byte("\x02d"))
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("desktop detach did not complete")
		}
	}
	_, terminal, done := start("here")
	first := observe()
	if first.ActivePane.Cwd != cwd {
		t.Fatalf("desktop directory = %q, want %q", first.ActivePane.Cwd, cwd)
	}
	_, _ = terminal.Write([]byte("printf 'DESKTOP_ONCE\\n'\r"))
	deadline := time.Now().Add(5 * time.Second)
	for {
		lines, err := adapter.CapturePane(context.Background(), first.ActivePane.ID, 500)
		if err != nil {
			t.Fatal(err)
		}
		count := 0
		for _, line := range lines {
			if strings.TrimSpace(line) == "DESKTOP_ONCE" {
				count++
			}
		}
		if count == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("desktop command output not observed: %q", lines)
		}
		time.Sleep(10 * time.Millisecond)
	}
	detach(terminal, done)
	_, terminal, done = start("here")
	second := observe()
	if first.ID != second.ID || first.ActivePane.PID != second.ActivePane.PID {
		t.Fatal("here replaced the Session or shell")
	}
	detach(terminal, done)
	_, terminal, done = start("resume", "--socket", socket, "--name", first.Name, first.ID)
	third := observe()
	if first.ID != third.ID || first.ActivePane.PID != third.ActivePane.PID {
		t.Fatal("resume replaced the Session or shell")
	}
	detach(terminal, done)
	_, _, done = start("resume", "--socket", socket, "--name", "an old name", first.ID)
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("stale name was accepted")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("stale resume attached")
	}
	items, err := adapter.ListSessions(context.Background())
	if err != nil || len(items) != 1 {
		t.Fatalf("desktop Session count = %d, err=%v", len(items), err)
	}
	if _, err := os.Stat(filepath.Join(root, "projects.json")); !os.IsNotExist(err) {
		t.Fatal("desktop entry registered a Project")
	}
	if _, err := os.Stat(filepath.Join(root, "work")); !os.IsNotExist(err) {
		t.Fatal("desktop entry created Managed Work")
	}
	previousDirectory := cwd
	cwd = filepath.Join(root, "another", filepath.Base(cwd))
	if err := os.MkdirAll(cwd, 0700); err != nil {
		t.Fatal(err)
	}
	cwd, err = filepath.EvalSymlinks(cwd)
	if err != nil {
		t.Fatal(err)
	}
	_, terminal, done = start("here")
	fourth := observe()
	if fourth.ID == first.ID || fourth.ActivePane.Cwd != cwd {
		t.Fatal("different directories shared a Session")
	}
	detach(terminal, done)
	cwd = previousDirectory
	unchanged, err := adapter.InspectSession(context.Background(), first.ID)
	if err != nil || unchanged.ActivePane.PID != first.ActivePane.PID {
		t.Fatal("another directory disturbed the original Session")
	}
}

func TestDesktopEntryRejectsOtherProfileAndNoninteractiveInput(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MCTRL_HOME", root)
	t.Setenv("MCTRL_PROFILE", "v2")
	t.Setenv("TMUX", "")
	if err := config.EnsureRuntimeIdentity(root, "v1"); err != nil {
		t.Fatal(err)
	}
	if err := runHere(nil); err == nil || !strings.Contains(err.Error(), "not v2") {
		t.Fatalf("other profile = %v", err)
	}
	t.Setenv("MCTRL_PROFILE", "v1")
	input, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	previous := os.Stdin
	os.Stdin = input
	defer func() { os.Stdin = previous }()
	if err := runHere(nil); err == nil || !strings.Contains(err.Error(), "interactive terminal") {
		t.Fatalf("noninteractive entry = %v", err)
	}
}
