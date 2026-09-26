package terminal

import "github.com/creack/pty"

// mctrl needs one answer to "how big is a whole Session?" for three separate
// decisions, and they must never drift apart:
//
//   - the size a Managed Work pane is launched at,
//   - the initial size of a disposable attach PTY,
//   - the size the phone's FULL overview asks tmux for.
//
// tmux's "largest client" policy only protects a shared window while a desktop
// client is attached. When the phone is the only client, its own viewport
// becomes the largest client and the window shrinks to phone size. A FULL
// overview that reads the pane's current size would then be identical to LOCAL
// and there would be nothing to show. FULL therefore asks for
// max(configured full size, current pane size) instead of trusting whatever the
// pane happens to be right now.
const (
	// DefaultFullCols and DefaultFullRows are mctrl's canonical full Session
	// size. They match what a Managed Work pane is launched at, so entering FULL
	// restores the layout the Work started with instead of inventing a new one.
	DefaultFullCols = 120
	DefaultFullRows = 40
)

// Bounds accepted from any client for a terminal resize. The bridge clamps to
// these before touching a PTY, and the phone uses them to keep its own terminal
// within a range xterm.js can lay out.
const (
	MinTerminalCols = 20
	MinTerminalRows = 10
	MaxTerminalCols = 500
	MaxTerminalRows = 200
)

// DefaultFullWinsize returns mctrl's canonical full Session size.
func DefaultFullWinsize() pty.Winsize {
	return pty.Winsize{Cols: DefaultFullCols, Rows: DefaultFullRows}
}

// FullWinsize is the size a FULL overview should request for a Session whose
// pane is currently paneWidth by paneHeight. A pane larger than the canonical
// size is authoritative because a desktop client asked for it; a smaller pane is
// treated as phone-driven and raised back to the canonical size.
func FullWinsize(paneWidth, paneHeight int) pty.Winsize {
	cols := DefaultFullCols
	rows := DefaultFullRows
	if paneWidth > cols {
		cols = paneWidth
	}
	if paneHeight > rows {
		rows = paneHeight
	}
	return ClampWinsize(cols, rows)
}

// ClampWinsize keeps a client-requested terminal size inside the supported
// range instead of trusting it.
func ClampWinsize(cols, rows int) pty.Winsize {
	return pty.Winsize{
		Cols: clampDimension(cols, MinTerminalCols, MaxTerminalCols),
		Rows: clampDimension(rows, MinTerminalRows, MaxTerminalRows),
	}
}

func clampDimension(value, low, high int) uint16 {
	if value < low {
		value = low
	}
	if value > high {
		value = high
	}
	return uint16(value)
}
