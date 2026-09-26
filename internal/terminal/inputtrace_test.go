package terminal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTraceTestDirs(t *testing.T) string {
	t.Helper()
	// MCTRL_HOME is the state directory itself, not a home directory.
	stateDir := filepath.Join(t.TempDir(), "state")
	logs := filepath.Join(stateDir, "logs")
	if err := os.MkdirAll(logs, 0o700); err != nil {
		t.Fatalf("create logs dir: %v", err)
	}
	t.Setenv("MCTRL_HOME", stateDir)
	return logs
}

func TestTraceTerminalInputStaysOffWithoutFlag(t *testing.T) {
	logs := newTraceTestDirs(t)

	TraceTerminalInput("ses_test", []byte("a"))

	if _, err := os.Stat(filepath.Join(logs, "input-trace.log")); !os.IsNotExist(err) {
		t.Fatalf("trace written without the flag: %v", err)
	}
}

func TestTraceTerminalInputRecordsFramesWithFlag(t *testing.T) {
	logs := newTraceTestDirs(t)
	if err := os.WriteFile(filepath.Join(logs, inputTraceFlagName), nil, 0o600); err != nil {
		t.Fatalf("write flag: %v", err)
	}

	TraceTerminalInput("ses_test", []byte("a"))
	// A full-width comma is three bytes in UTF-8 and must survive as itself, so
	// the recording can be read without decoding hex.
	TraceTerminalInput("ses_test", []byte("，"))
	TraceTerminalInput("ses_test", []byte{0x1b, '[', 'A'})
	// An empty frame still records that the phone sent one.
	TraceTerminalInput("ses_test", nil)

	data, err := os.ReadFile(filepath.Join(logs, "input-trace.log"))
	if err != nil {
		t.Fatalf("read trace: %v", err)
	}
	recorded := string(data)
	for _, want := range []string{
		"session=ses_test",
		`text="a"`,
		`text="，"`,
		"hex=ef bc 8c",
		`text="\e[A"`,
		`bytes=0 text="" hex=`,
	} {
		if !strings.Contains(recorded, want) {
			t.Errorf("trace missing %q\nrecorded:\n%s", want, recorded)
		}
	}
	lines := strings.Count(strings.TrimRight(recorded, "\n"), "\n") + 1
	if lines != 4 {
		t.Errorf("recorded %d lines, want 4\nrecorded:\n%s", lines, recorded)
	}
}

func TestTraceTerminalInputKeepsEveryFrameInOrder(t *testing.T) {
	logs := newTraceTestDirs(t)
	if err := os.WriteFile(filepath.Join(logs, inputTraceFlagName), nil, 0o600); err != nil {
		t.Fatalf("write flag: %v", err)
	}

	TraceTerminalInput("ses_test", []byte("first"))
	TraceTerminalInput("ses_test", []byte("second"))

	data, err := os.ReadFile(filepath.Join(logs, "input-trace.log"))
	if err != nil {
		t.Fatalf("read trace: %v", err)
	}
	recorded := string(data)
	firstAt := strings.Index(recorded, `text="first"`)
	secondAt := strings.Index(recorded, `text="second"`)
	// Frames accumulate; resetting the file is the caller's job, not the tool's.
	if firstAt < 0 || secondAt < 0 {
		t.Fatalf("both frames must be recorded:\n%s", recorded)
	}
	if firstAt > secondAt {
		t.Errorf("frames out of order:\n%s", recorded)
	}
}

func TestTraceTerminalInputStopsAtTheSizeCap(t *testing.T) {
	logs := newTraceTestDirs(t)
	if err := os.WriteFile(filepath.Join(logs, inputTraceFlagName), nil, 0o600); err != nil {
		t.Fatalf("write flag: %v", err)
	}
	tracePath := filepath.Join(logs, "input-trace.log")
	// A forgotten flag must not be able to fill the disk.
	oversized := make([]byte, inputTraceLimit+1)
	for i := range oversized {
		oversized[i] = 'a'
	}
	if err := os.WriteFile(tracePath, oversized, 0o600); err != nil {
		t.Fatalf("seed trace: %v", err)
	}

	TraceTerminalInput("ses_test", []byte("x"))

	data, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatalf("read trace: %v", err)
	}
	if len(data) != len(oversized) {
		t.Errorf("trace grew past the cap: %d bytes, cap is %d", len(data), inputTraceLimit)
	}
}
