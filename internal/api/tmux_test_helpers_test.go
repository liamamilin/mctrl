package api

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"mctrl/internal/tmux"
)

// isolateTestTmux gives each integration test a private tmux server. Tests
// must never enumerate, create, or remove sessions in the developer's or CI
// runner's default tmux endpoint.
func isolateTestTmux(t *testing.T) (*tmux.Adapter, string) {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is not installed")
	}
	dir, err := os.MkdirTemp("", "mctrl-api-tmux-")
	if err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(dir, "s")
	t.Cleanup(func() {
		_ = exec.Command("tmux", "-S", socket, "kill-server").Run()
		_ = os.RemoveAll(dir)
	})
	return tmux.NewWithSocket(socket), socket
}

func testTmuxCommand(socket string, args ...string) *exec.Cmd {
	commandArgs := make([]string, 0, len(args)+2)
	commandArgs = append(commandArgs, "-S", socket)
	commandArgs = append(commandArgs, args...)
	return exec.Command("tmux", commandArgs...)
}
