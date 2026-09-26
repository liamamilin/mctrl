package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mctrl/internal/config"
	"mctrl/internal/work"
)

func TestProjectRestoreReinstatesTheReferencedID(t *testing.T) {
	t.Setenv(config.ProfileEnv, "v1")
	stateDir := t.TempDir()
	t.Setenv("MCTRL_HOME", stateDir)

	target := t.TempDir()
	if err := runProject([]string{
		"restore",
		"--id", "project-E_zCrQ",
		"--name", "ai-research",
		"--runner", "shell",
		target,
	}); err != nil {
		t.Fatalf("project restore: %v", err)
	}

	// A Work still references the old id, which is the whole reason restore
	// exists: it must resolve again without minting a new id.
	store := work.NewStore(stateDir)
	if err := store.Save(work.Work{
		ID:          "work_restore",
		ProjectID:   "project-E_zCrQ",
		State:       work.StateRunning,
		CreatedAt:   time.Now().UTC(),
		RequestID:   "request_restore",
		DeviceID:    "device_restore",
		ProjectPath: target,
		RunnerID:    "shell",
	}); err != nil {
		t.Fatal(err)
	}

	if err := runProject([]string{"list"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(stateDir, "projects.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "project-E_zCrQ") {
		t.Fatalf("projects.json does not carry the restored id:\n%s", data)
	}
}

func TestProjectRestoreRequiresAnID(t *testing.T) {
	t.Setenv(config.ProfileEnv, "v1")
	t.Setenv("MCTRL_HOME", t.TempDir())
	err := runProject([]string{"restore", t.TempDir()})
	if err == nil {
		t.Fatal("project restore accepted a missing --id")
	}
	if !strings.Contains(err.Error(), "--id") {
		t.Fatalf("error does not mention --id: %v", err)
	}
}

func TestProjectRemoveIsBlockedWhileWorkReferencesTheProject(t *testing.T) {
	t.Setenv(config.ProfileEnv, "v1")
	stateDir := t.TempDir()
	t.Setenv("MCTRL_HOME", stateDir)
	target := t.TempDir()

	if err := runProject([]string{"restore", "--id", "project-live", "--name", "live", target}); err != nil {
		t.Fatal(err)
	}
	store := work.NewStore(stateDir)
	if err := store.Save(work.Work{
		ID:          "work_live",
		ProjectID:   "project-live",
		State:       work.StateRunning,
		CreatedAt:   time.Now().UTC(),
		RequestID:   "request_live",
		DeviceID:    "device_live",
		ProjectPath: target,
		RunnerID:    "shell",
	}); err != nil {
		t.Fatal(err)
	}
	err := runProject([]string{"remove", "project-live"})
	if err == nil {
		t.Fatal("project remove ignored the active Work that references the Project")
	}
	if !strings.Contains(err.Error(), "work_live") {
		t.Fatalf("error does not name the blocking Work: %v", err)
	}
}
