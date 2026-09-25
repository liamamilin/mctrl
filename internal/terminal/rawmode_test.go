package terminal

import (
	"testing"

	"github.com/creack/pty"
	"golang.org/x/sys/unix"

	"mctrl/internal/tmux"
)

func readTermios(t *testing.T, fd int) *unix.Termios {
	t.Helper()
	settings, err := unix.IoctlGetTermios(fd, ioctlReadTermios)
	if err != nil {
		t.Fatalf("read termios: %v", err)
	}
	return settings
}

func writeTermios(t *testing.T, fd int, settings *unix.Termios) {
	t.Helper()
	if err := unix.IoctlSetTermios(fd, ioctlWriteTermios, settings); err != nil {
		t.Fatalf("write termios: %v", err)
	}
}

func TestEnsureRawTransportRepairsCookedTransport(t *testing.T) {
	ptmx, tty, err := pty.Open()
	if err != nil {
		t.Skipf("pty unavailable: %v", err)
	}
	defer ptmx.Close()
	defer tty.Close()

	cooked := readTermios(t, int(tty.Fd()))
	if cooked.Lflag&unix.ICANON == 0 || cooked.Lflag&unix.ECHO == 0 {
		t.Skip("pty did not start in canonical mode with echo")
	}

	repaired, err := EnsureRawTransport(int(tty.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	if !repaired {
		t.Fatal("cooked transport was not reported as repaired")
	}

	raw := readTermios(t, int(tty.Fd()))
	if raw.Lflag&(unix.ICANON|unix.ECHO|unix.ISIG|unix.IEXTEN) != 0 {
		t.Fatalf("lflags still rewrite input: %#x", raw.Lflag)
	}
	if raw.Iflag&(unix.ICRNL|unix.INLCR|unix.IGNCR|unix.IXON) != 0 {
		t.Fatalf("iflags still rewrite input: %#x", raw.Iflag)
	}
	if raw.Oflag&unix.OPOST != 0 {
		t.Fatalf("oflags still post-process output: %#x", raw.Oflag)
	}

	again, err := EnsureRawTransport(int(tty.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	if again {
		t.Fatal("raw transport was reported as repaired twice")
	}
}

func TestEnsureRawTransportRepairsDrift(t *testing.T) {
	ptmx, tty, err := pty.Open()
	if err != nil {
		t.Skipf("pty unavailable: %v", err)
	}
	defer ptmx.Close()
	defer tty.Close()

	if _, err := EnsureRawTransport(int(tty.Fd())); err != nil {
		t.Fatal(err)
	}

	// Model tmux handing the pane back a cooked line discipline.
	drifted := readTermios(t, int(tty.Fd()))
	drifted.Lflag |= unix.ICANON | unix.ECHO
	drifted.Iflag |= unix.ICRNL
	writeTermios(t, int(tty.Fd()), drifted)

	repaired, err := EnsureRawTransport(int(tty.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	if !repaired {
		t.Fatal("drifted transport was not reported as repaired")
	}
	raw, err := TransportIsRaw(int(tty.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	if !raw {
		t.Fatal("transport is still not raw after repair")
	}
}

func TestTransportIsRawRejectsNonTerminal(t *testing.T) {
	ptmx, tty, err := pty.Open()
	if err != nil {
		t.Skipf("pty unavailable: %v", err)
	}
	defer ptmx.Close()
	defer tty.Close()

	// A regular file is not a terminal; the helper must report the failure
	// instead of claiming a repair it cannot verify.
	repaired, err := EnsureRawTransportPath("/dev/null")
	if err == nil {
		t.Skip("/dev/null reports a terminal state on this platform")
	}
	if repaired {
		t.Fatal("a non-terminal device was reported as repaired")
	}
}

func TestEnsureRawTransportPathRepairsPaneDevice(t *testing.T) {
	ptmx, tty, err := pty.Open()
	if err != nil {
		t.Skipf("pty unavailable: %v", err)
	}
	defer ptmx.Close()
	defer tty.Close()

	name := tty.Name()
	repaired, err := EnsureRawTransportPath(name)
	if err != nil {
		t.Fatal(err)
	}
	if !repaired {
		t.Fatalf("cooked %s was not reported as repaired", name)
	}
	raw, err := TransportIsRaw(int(tty.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	if !raw {
		t.Fatalf("%s is not raw after path repair", name)
	}
	again, err := EnsureRawTransportPath(name)
	if err != nil {
		t.Fatal(err)
	}
	if again {
		t.Fatal("path repair reported a second change")
	}
}

func TestEnsureRawTransportPathRejectsEmptyPath(t *testing.T) {
	if _, err := EnsureRawTransportPath(""); err == nil {
		t.Fatal("empty terminal path was accepted")
	}
}

func TestOwnsRunnerCommand(t *testing.T) {
	for _, test := range []struct {
		name    string
		command string
		want    bool
	}{
		{name: "managed work runner", command: "mctrl-runner", want: true},
		{name: "padded command", command: "  mctrl-runner\n", want: true},
		{name: "user shell", command: "zsh", want: false},
		{name: "user application", command: "opencode", want: false},
		{name: "empty", command: "", want: false},
		{name: "lookalike", command: "not-mctrl-runner", want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := ownsRunnerCommand(test.command); got != test.want {
				t.Fatalf("ownsRunnerCommand(%q) = %v, want %v", test.command, got, test.want)
			}
		})
	}
}

func TestOwnsPaneTransportPrefersTheWorkRecord(t *testing.T) {
	bridge := &Bridge{managedWork: func(sessionID string) bool { return sessionID == "work-session" }}
	if !bridge.ownsPaneTransport("work-session", tmux.PaneTransport{Command: "zsh"}) {
		t.Fatal("a Managed Work Session must be mctrl's to repair")
	}
	if bridge.ownsPaneTransport("other-session", tmux.PaneTransport{Command: "zsh"}) {
		t.Fatal("an external Session must never be reconfigured")
	}
	if !bridge.ownsPaneTransport("other-session", tmux.PaneTransport{Command: "mctrl-runner"}) {
		t.Fatal("a pane still running mctrl's runner must be repairable without a Work record")
	}
	bare := &Bridge{}
	if bare.ownsPaneTransport("work-session", tmux.PaneTransport{Command: "zsh"}) {
		t.Fatal("a bridge without a Work lookup must stay conservative")
	}
}
