package terminal

import "testing"

func TestFullWinsizeRaisesASmallPaneToTheCanonicalSize(t *testing.T) {
	// The reported failure: the phone was the only attached client, so tmux had
	// already shrunk the shared window to the phone's own viewport. FULL must not
	// inherit that size.
	full := FullWinsize(46, 22)
	if full.Cols != DefaultFullCols || full.Rows != DefaultFullRows {
		t.Fatalf("FullWinsize(46, 22) = %dx%d, want %dx%d", full.Cols, full.Rows, DefaultFullCols, DefaultFullRows)
	}
}

func TestFullWinsizeKeepsALargerDesktopPane(t *testing.T) {
	// A desktop client made the window bigger than the canonical size, so the
	// desktop stays authoritative.
	full := FullWinsize(272, 60)
	if full.Cols != 272 || full.Rows != 60 {
		t.Fatalf("FullWinsize(272, 60) = %dx%d, want 272x60", full.Cols, full.Rows)
	}
}

func TestFullWinsizeWithoutAPane(t *testing.T) {
	full := FullWinsize(0, 0)
	if full.Cols != DefaultFullCols || full.Rows != DefaultFullRows {
		t.Fatalf("FullWinsize(0, 0) = %dx%d, want %dx%d", full.Cols, full.Rows, DefaultFullCols, DefaultFullRows)
	}
}

func TestFullWinsizeNeverShrinksTheCanonicalSize(t *testing.T) {
	for _, pane := range [][2]int{{0, 0}, {1, 1}, {20, 10}, {46, 22}, {119, 39}, {120, 40}} {
		full := FullWinsize(pane[0], pane[1])
		if full.Cols < DefaultFullCols || full.Rows < DefaultFullRows {
			t.Fatalf("FullWinsize(%d, %d) = %dx%d, smaller than the canonical %dx%d",
				pane[0], pane[1], full.Cols, full.Rows, DefaultFullCols, DefaultFullRows)
		}
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
		{name: "desktop pane", cols: 272, rows: 60, wantCols: 272, wantRows: 60},
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

func TestDefaultFullWinsizeIsWithinTheSupportedRange(t *testing.T) {
	full := DefaultFullWinsize()
	if full.Cols < MinTerminalCols || full.Cols > MaxTerminalCols {
		t.Fatalf("default full cols %d is outside the supported range", full.Cols)
	}
	if full.Rows < MinTerminalRows || full.Rows > MaxTerminalRows {
		t.Fatalf("default full rows %d is outside the supported range", full.Rows)
	}
	if clamped := ClampWinsize(int(full.Cols), int(full.Rows)); clamped != full {
		t.Fatalf("default full size %dx%d is not stable under clamping", full.Cols, full.Rows)
	}
}
