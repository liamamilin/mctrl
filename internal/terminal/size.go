package terminal

import (
	"github.com/creack/pty"

	"mctrl/internal/config"
)

// mctrl needs one answer to "how big is a whole Session?" for three separate
// decisions, and they must never drift apart:
//
//   - the size a Managed Work pane is launched at,
//   - the initial size of a disposable attach PTY,
//   - the size the phone's FULL overview asks tmux for.
//
// tmux's "largest client" policy only protects a shared window while a desktop
// client is attached. When the phone is the only client, its own viewport
// becomes that largest client and the window shrinks to phone size. A FULL
// overview that reads the pane's current size would then be identical to LOCAL
// and there would be nothing to show, so FULL asks for
// max(configured full size, current pane size) instead.
//
// The configured size is config.TerminalFullSize. Programs lay themselves out
// by width and drop panels below their own thresholds, so it is deliberately
// close to a real desktop terminal rather than a phone viewport.

// CanonicalWinsize converts a configured full Session size into a PTY size.
func CanonicalWinsize(size config.TerminalSize) pty.Winsize {
	clamped := size.Clamped()
	return pty.Winsize{Cols: uint16(clamped.Cols), Rows: uint16(clamped.Rows)}
}

// FullWinsize is the size a FULL overview should request for a Session whose
// pane is currently paneWidth by paneHeight. A pane larger than the configured
// size is authoritative because a desktop client asked for it; a smaller pane is
// treated as phone-driven and raised back to the configured size.
func FullWinsize(configured config.TerminalSize, paneWidth, paneHeight int) pty.Winsize {
	full := configured
	if paneWidth > full.Cols {
		full.Cols = paneWidth
	}
	if paneHeight > full.Rows {
		full.Rows = paneHeight
	}
	return CanonicalWinsize(full)
}

// Bounds accepted from any client for a terminal resize. The bridge clamps to
// these before touching a PTY, and the phone uses them to keep its own terminal
// within a range xterm.js can lay out.
const (
	MinTerminalCols = config.MinTerminalCols
	MinTerminalRows = config.MinTerminalRows
	MaxTerminalCols = config.MaxTerminalCols
	MaxTerminalRows = config.MaxTerminalRows
)

// ClampWinsize keeps a client-requested terminal size inside the supported
// range instead of trusting it.
func ClampWinsize(cols, rows int) pty.Winsize {
	return CanonicalWinsize(config.TerminalSize{Cols: cols, Rows: rows}.Clamped())
}
