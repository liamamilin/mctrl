package work

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"mctrl/internal/tmux"
)

// isolatedUnavailableTmux prevents recovery tests from reading the user's or
// CI runner's default tmux server. Reconcile only needs a deterministic socket
// with no running server.
func isolatedUnavailableTmux(t *testing.T) *tmux.Adapter {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is not installed")
	}
	dir, err := os.MkdirTemp("", "mctrl-work-tmux-")
	if err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(dir, "s")
	t.Cleanup(func() {
		_ = exec.Command("tmux", "-S", socket, "kill-server").Run()
		_ = os.RemoveAll(dir)
	})
	return tmux.NewWithSocket(socket)
}
