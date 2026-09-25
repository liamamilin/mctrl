package terminal

import (
	"bytes"
	"errors"
	"io"
	"os/exec"
	"testing"
	"time"

	"github.com/creack/pty"

	"mctrl/internal/tmux"
)

func TestStartRawPTYDoesNotEchoControlBytes(t *testing.T) {
	cmd := exec.Command("/bin/cat")
	ptmx, err := startRawPTY(cmd, pty.Winsize{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	defer ptmx.Close()
	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}()

	input := []byte("\x1b[10;rgb:1/2/3\a")
	if _, err := ptmx.Write(input); err != nil {
		t.Fatal(err)
	}
	_ = ptmx.SetReadDeadline(time.Now().Add(time.Second))
	got := make([]byte, len(input))
	if _, err := io.ReadFull(ptmx, got); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, input) {
		t.Fatalf("raw pty changed control bytes: got %q, want %q", got, input)
	}
}

func TestTerminalExitCode(t *testing.T) {
	for _, test := range []struct {
		name   string
		exists bool
		err    error
		want   string
	}{
		{name: "missing from live server", want: sessionGone},
		{name: "server stopped", err: tmux.ErrNoServer, want: sessionGone},
		{name: "session still exists", exists: true, want: terminalAttachFailed},
		{name: "tmux unavailable", err: tmux.ErrUnavailable, want: terminalAttachFailed},
		{name: "probe failed", err: errors.New("probe failed"), want: terminalAttachFailed},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := terminalExitCode(test.exists, test.err); got != test.want {
				t.Fatalf("terminalExitCode() = %q, want %q", got, test.want)
			}
		})
	}
}
