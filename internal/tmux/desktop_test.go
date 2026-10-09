package tmux

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestDesktopCommandQuotesSocketAndSessionIdentity(t *testing.T) {
	adapter := NewWithSocket("/tmp/a'b socket")
	command := adapter.DesktopResumeCommand("v2", "$12", "中文 'session'")
	// Let the local shell parse it into arguments without running mctrl.
	output, err := exec.Command("/bin/sh", "-c", "mctrl() { printf '%s\\n' \"$@\"; }; "+command).Output()
	if err != nil {
		t.Fatal(err)
	}
	want := "--profile\nv2\nresume\n--socket\n/tmp/a'b socket\n--name\n中文 'session'\n$12\n"
	if string(output) != want {
		t.Fatalf("quoted command arguments = %q", output)
	}
	for _, value := range []string{"/tmp/s,123,0", "/tmp/a,b socket,123,0"} {
		socket, err := ClientSocket(value)
		if err != nil || socket != value[:strings.LastIndex(value[:strings.LastIndex(value, ",")], ",")] {
			t.Fatalf("client socket = %q, %v", socket, err)
		}
	}
	if _, err := ClientSocket("invalid"); err == nil {
		t.Fatal("invalid client identity accepted")
	}
}

func TestDesktopClientKeepsNativeTerminalEnvironment(t *testing.T) {
	adapter, _ := newTestAdapter(t)
	t.Setenv("TERM", "wezterm")
	t.Setenv("COLORTERM", "truecolor")
	command, err := adapter.DesktopClientCommand(context.Background(), "$1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(command.Env, "\n"), "TERM=wezterm\n") {
		t.Fatal("native terminal capability changed")
	}
	if strings.Contains(strings.Join(command.Args, " "), "-d") {
		t.Fatal("desktop attach detaches existing clients")
	}
	if len(command.Env) != len(os.Environ()) {
		t.Fatal("native environment was modified")
	}
}
