//go:build darwin

package terminal

import "golang.org/x/sys/unix"

// Darwin exposes the BSD termios ioctls instead of the Linux TCGETS/TCSETS
// pair.
const (
	ioctlReadTermios  = unix.TIOCGETA
	ioctlWriteTermios = unix.TIOCSETA
)
