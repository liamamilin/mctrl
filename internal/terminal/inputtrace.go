package terminal

// TEMPORARY INPUT DIAGNOSTIC — delete this file and its call site once the
// Chinese-keyboard punctuation question is settled.
//
// The phone cannot copy a recording out of the page: iOS blocks paste from a
// plain-HTTP page, and the async clipboard API needs a secure context, which
// Trusted LAN HTTP deliberately is not. The daemon, however, already sees every
// byte the phone sends, so the question is answered here instead of on the
// device: does the character cross the WebSocket at all?
//
// The trace is off unless the flag file exists, so nothing is recorded during
// normal use and no keystroke reaches disk by default. Creating the flag needs
// no daemon restart.

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"mctrl/internal/config"
)

const inputTraceFlagName = "INPUT_TRACE"

// inputTraceLimit caps the recording so a forgotten flag cannot fill the disk.
const inputTraceLimit = 256 * 1024

// TraceTerminalInput appends one line per input frame from the phone. It reports
// whether anything was written, so a disabled trace costs one failed stat.
func TraceTerminalInput(sessionID string, data []byte) {
	stateDir, err := config.StateDir()
	if err != nil {
		return
	}
	logDir := filepath.Join(stateDir, "logs")
	flagPath := filepath.Join(logDir, inputTraceFlagName)
	if _, err := os.Stat(flagPath); err != nil {
		return
	}
	// No reset here. Frames accumulate, and whoever starts a round deletes the
	// file, so a stale recording is never silently mixed into a new one and the
	// tool keeps no hidden state of its own.
	tracePath := filepath.Join(logDir, "input-trace.log")
	file, err := os.OpenFile(tracePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer func() { _ = file.Close() }()
	if info, statErr := file.Stat(); statErr == nil && info.Size() > inputTraceLimit {
		return
	}
	line := fmt.Sprintf(
		"%s session=%s bytes=%d text=%s hex=%s\n",
		time.Now().Format("15:04:05.000"),
		sessionID,
		len(data),
		quoteForTrace(data),
		hexForTrace(data),
	)
	_, _ = file.WriteString(line)
}

// quoteForTrace shows printable text as typed, including multi-byte characters,
// so a full-width mark is visible as itself rather than as an escape.
func quoteForTrace(data []byte) string {
	var builder strings.Builder
	builder.WriteByte('"')
	for _, b := range data {
		switch {
		case b == '"' || b == '\\':
			builder.WriteByte('\\')
			builder.WriteByte(b)
		case b == '\n':
			builder.WriteString("\\n")
		case b == '\r':
			builder.WriteString("\\r")
		case b == '\t':
			builder.WriteString("\\t")
		case b == 0x1b:
			builder.WriteString("\\e")
		case b < 0x20 || b == 0x7f:
			builder.WriteString("\\x" + strconv.FormatInt(int64(b), 16))
		default:
			builder.WriteByte(b)
		}
	}
	builder.WriteByte('"')
	return builder.String()
}

func hexForTrace(data []byte) string {
	parts := make([]string, 0, len(data))
	for _, b := range data {
		parts = append(parts, fmt.Sprintf("%02x", b))
	}
	return strings.Join(parts, " ")
}
