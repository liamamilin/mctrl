package work

import (
	"context"
	"os"
	"testing"
)

func TestDirectoryFromLsofPreservesLiteralPath(t *testing.T) {
	for _, directory := range []string{"/", "/tmp/dir 'with' spaces", "/tmp/中文", "/tmp/line\nbreak"} {
		if got := directoryFromLsof([]byte("p123\x00\nfcwd\x00n" + directory + "\x00\n")); got != directory {
			t.Fatalf("got %q, want %q", got, directory)
		}
	}
	if got := directoryFromLsof([]byte("p123\x00")); got != "" {
		t.Fatalf("invented directory %q", got)
	}
}

func TestCurrentDirectoryRejectsUnverifiedAndArchivedProcesses(t *testing.T) {
	item := Work{ID: "work_foreign", AttemptID: "attempt_foreign", State: StateRunning, RunnerPID: os.Getpid(), ChildPID: os.Getpid()}
	if directory, err := CurrentDirectory(context.Background(), item); err == nil || directory != "" {
		t.Fatalf("foreign process exposed a directory: %q, %v", directory, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if directory, err := CurrentDirectory(ctx, item); err == nil || directory != "" {
		t.Fatalf("canceled observation: %q, %v", directory, err)
	}
	item.State = StateExited
	if directory, err := CurrentDirectory(context.Background(), item); err == nil || directory != "" {
		t.Fatalf("terminal observation: %q, %v", directory, err)
	}
	item.State = StateRunning
	item.ArchivedAt = &item.CreatedAt
	if directory, err := CurrentDirectory(context.Background(), item); err == nil || directory != "" {
		t.Fatalf("archived observation: %q, %v", directory, err)
	}
}
