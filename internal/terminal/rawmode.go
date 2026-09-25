package terminal

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

// mctrl transports terminal bytes through a kernel line discipline twice: once
// on the disposable attach PTY and once on the tmux pane PTY that a Managed Work
// runner owns. A cooked line discipline silently rewrites the user's typing, so
// every transport mctrl controls is required to be raw:
//
//   - ICANON buffers keystrokes until Return, so nothing reaches the program
//     while the user types.
//   - ICRNL rewrites Return into a line feed, so a program that binds Return to
//     "submit" (and a line feed to "newline") submits nothing and inserts a
//     line break instead.
//   - ECHO and ECHOCTL print a second, caret-annotated copy of the input over
//     the program's own output.
//
// tmux does not keep a pane raw for us: a pane created by a detached server, or
// one whose client attaches later, keeps the line discipline it was created
// with. mctrl therefore verifies the mode instead of assuming it.
const (
	lflagInputMask = unix.ICANON | unix.ECHO | unix.ECHONL | unix.ISIG | unix.IEXTEN
	iflagInputMask = unix.ICRNL | unix.INLCR | unix.IGNCR | unix.IXON
)

// TransportIsRaw reports whether fd still carries the raw settings mctrl
// requires for a terminal transport.
func TransportIsRaw(fd int) (bool, error) {
	settings, err := unix.IoctlGetTermios(fd, ioctlReadTermios)
	if err != nil {
		return false, fmt.Errorf("read terminal attributes: %w", err)
	}
	return settings.Lflag&lflagInputMask == 0 &&
		settings.Iflag&iflagInputMask == 0 &&
		settings.Oflag&unix.OPOST == 0, nil
}

// EnsureRawTransport restores the raw settings mctrl requires and reports
// whether the transport had drifted. A transport that is already raw costs one
// read and no write, so callers may poll it.
func EnsureRawTransport(fd int) (bool, error) {
	raw, err := TransportIsRaw(fd)
	if err != nil {
		return false, err
	}
	if raw {
		return false, nil
	}
	if _, err := term.MakeRaw(fd); err != nil {
		return false, fmt.Errorf("set raw terminal mode: %w", err)
	}
	raw, err = TransportIsRaw(fd)
	if err != nil {
		return false, err
	}
	if !raw {
		return false, fmt.Errorf("terminal mode did not become raw")
	}
	return true, nil
}

// openTransportDevice opens a terminal device for inspection without ever
// taking it as a controlling terminal.
func openTransportDevice(path string, flag int) (*os.File, error) {
	if path == "" {
		return nil, fmt.Errorf("empty terminal path")
	}
	device, err := os.OpenFile(path, flag|unix.O_NOCTTY, 0)
	if err != nil {
		return nil, fmt.Errorf("open terminal %s: %w", path, err)
	}
	return device, nil
}

// TransportIsRawPath reports whether the terminal device at path still carries
// the raw settings mctrl requires.
func TransportIsRawPath(path string) (bool, error) {
	device, err := openTransportDevice(path, os.O_RDONLY)
	if err != nil {
		return false, err
	}
	defer device.Close()
	return TransportIsRaw(int(device.Fd()))
}

// EnsureRawTransportPath repairs the line discipline of the terminal device at
// path. It is used for a Managed Work pane whose runner cannot repair itself,
// and never takes ownership of the device: the descriptor is opened with
// O_NOCTTY so the repair cannot become a controlling terminal, and it is closed
// again immediately.
func EnsureRawTransportPath(path string) (bool, error) {
	device, err := openTransportDevice(path, os.O_RDWR)
	if err != nil {
		return false, err
	}
	defer device.Close()
	repaired, err := EnsureRawTransport(int(device.Fd()))
	if err != nil {
		return false, fmt.Errorf("repair terminal %s: %w", path, err)
	}
	return repaired, nil
}
