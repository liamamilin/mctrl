package terminal

import (
	"testing"

	"mctrl/internal/config"
)

func configured(cols, rows int) config.TerminalSize {
	return config.TerminalSize{Cols: cols, Rows: rows}
}

func TestFullWinsizeRaisesASmallPaneToTheConfiguredSize(t *testing.T) {
	// The reported failure: the phone was the only attached client, so tmux had
	// already shrunk the shared window to the phone's own viewport. FULL must not
	// inherit that size.
	full := FullWinsize(configured(240, 60), 46, 22)
	if full.Cols != 240 || full.Rows != 60 {
		t.Fatalf("FullWinsize(240x60, 46, 22) = %dx%d, want 240x60", full.Cols, full.Rows)
	}
}

func TestFullWinsizeKeepsALargerDesktopPane(t *testing.T) {
	// A desktop client made the window bigger than the configured size, so the
	// desktop stays authoritative.
	full := FullWinsize(configured(240, 60), 272, 61)
	if full.Cols != 272 || full.Rows != 61 {
		t.Fatalf("FullWinsize(240x60, 272, 61) = %dx%d, want 272x61", full.Cols, full.Rows)
	}
}

func TestFullWinsizeFollowsTheConfiguredSize(t *testing.T) {
	// The configured size is the product's desktop-like default; raising it must
	// change what FULL asks for, with no code change.
	full := FullWinsize(configured(300, 80), 46, 22)
	if full.Cols != 300 || full.Rows != 80 {
		t.Fatalf("FullWinsize(300x80, 46, 22) = %dx%d, want 300x80", full.Cols, full.Rows)
	}
}

func TestFullWinsizeNeverShrinksTheConfiguredSize(t *testing.T) {
	for _, pane := range [][2]int{{0, 0}, {1, 1}, {20, 10}, {46, 22}, {239, 59}, {240, 60}} {
		full := FullWinsize(config.DefaultFullTerminalSize(), pane[0], pane[1])
		if full.Cols < config.DefaultFullTerminalCols || full.Rows < config.DefaultFullTerminalRows {
			t.Fatalf("FullWinsize(%d, %d) = %dx%d, smaller than the configured %dx%d",
				pane[0], pane[1], full.Cols, full.Rows,
				config.DefaultFullTerminalCols, config.DefaultFullTerminalRows)
		}
	}
}

func TestDefaultFullTerminalSizeIsWithinTheSupportedRange(t *testing.T) {
	size := config.DefaultFullTerminalSize()
	if err := size.Validate(); err != nil {
		t.Fatalf("default full size is invalid: %v", err)
	}
	canonical := CanonicalWinsize(size)
	if clamped := ClampWinsize(int(canonical.Cols), int(canonical.Rows)); clamped != canonical {
		t.Fatalf("default full size %dx%d is not stable under clamping", canonical.Cols, canonical.Rows)
	}
}

func TestClampWinsizeBoundsClientRequests(t *testing.T) {
	for _, test := range []struct {
		name     string
		cols     int
		rows     int
		wantCols uint16
		wantRows uint16
	}{
		{name: "typical", cols: 120, rows: 40, wantCols: 120, wantRows: 40},
		{name: "phone viewport", cols: 46, rows: 22, wantCols: 46, wantRows: 22},
		{name: "desktop pane", cols: 272, rows: 61, wantCols: 272, wantRows: 61},
		{name: "configured default", cols: 240, rows: 60, wantCols: 240, wantRows: 60},
		{name: "absurdly large", cols: 5000, rows: 9000, wantCols: MaxTerminalCols, wantRows: MaxTerminalRows},
		{name: "absurdly small", cols: 0, rows: 0, wantCols: MinTerminalCols, wantRows: MinTerminalRows},
		{name: "negative", cols: -20, rows: -5, wantCols: MinTerminalCols, wantRows: MinTerminalRows},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := ClampWinsize(test.cols, test.rows)
			if got.Cols != test.wantCols || got.Rows != test.wantRows {
				t.Fatalf("ClampWinsize(%d, %d) = %dx%d, want %dx%d", test.cols, test.rows, got.Cols, got.Rows, test.wantCols, test.wantRows)
			}
		})
	}
}

func TestBridgeUsesTheSuppliedFullSize(t *testing.T) {
	bridge := NewBridge(nil, nil, func() config.TerminalSize { return configured(300, 80) })
	if got := bridge.canonicalFullSize(); got.Cols != 300 || got.Rows != 80 {
		t.Fatalf("canonicalFullSize() = %dx%d, want 300x80", got.Cols, got.Rows)
	}
	fallback := NewBridge(nil, nil, nil)
	want := CanonicalWinsize(config.DefaultFullTerminalSize())
	if got := fallback.canonicalFullSize(); got != want {
		t.Fatalf("default canonicalFullSize() = %dx%d, want %dx%d", got.Cols, got.Rows, want.Cols, want.Rows)
	}
}
