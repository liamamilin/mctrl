package terminal

import (
	"errors"
	"testing"

	"mctrl/internal/tmux"
)

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
