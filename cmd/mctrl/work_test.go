package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mctrl/internal/work"
)

func TestWorkArchiveRequiresExplicitConfirmation(t *testing.T) {
	t.Setenv("MCTRL_PROFILE", "v1")
	root := t.TempDir()
	t.Setenv("MCTRL_HOME", root)
	store := work.NewStore(root)
	item := work.Work{ID: "work_cli", State: work.StateRunning, CreatedAt: time.Now().Add(-time.Minute)}
	if err := store.Save(item); err != nil {
		t.Fatal(err)
	}
	if err := runWork([]string{"archive", item.ID}); err == nil {
		t.Fatal("archive accepted without confirmation")
	}
	got, _ := store.Get(item.ID)
	if got.ArchivedAt != nil {
		t.Fatal("unconfirmed archive changed Work")
	}
}

func TestWorkCommandsRejectAnotherProfilesState(t *testing.T) {
	t.Setenv("MCTRL_PROFILE", "v2")
	root := t.TempDir()
	t.Setenv("MCTRL_HOME", root)
	if err := os.WriteFile(filepath.Join(root, "runtime.json"), []byte(`{"schema_version":1,"profile":"v1"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := runWork([]string{"list"}); err == nil || !strings.Contains(err.Error(), "not v2") {
		t.Fatalf("wrong profile was accepted: %v", err)
	}
}
