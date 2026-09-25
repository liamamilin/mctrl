package tmux

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// newTestAdapter gives each real-tmux test a private server endpoint. This
// keeps integration tests independent from the user's desktop tmux sessions.
func newTestAdapter(t *testing.T) (*Adapter, string) {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is not installed")
	}
	dir, err := os.MkdirTemp("", "mctrl-tmux-")
	if err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(dir, "s")
	t.Cleanup(func() {
		_ = exec.Command("tmux", "-S", socket, "kill-server").Run()
		_ = os.RemoveAll(dir)
	})
	return NewWithSocket(socket), socket
}

func testTmuxCommand(socket string, args ...string) *exec.Cmd {
	commandArgs := make([]string, 0, len(args)+2)
	commandArgs = append(commandArgs, "-S", socket)
	commandArgs = append(commandArgs, args...)
	return exec.Command("tmux", commandArgs...)
}
