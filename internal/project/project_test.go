package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAddNormalizesAndDeduplicatesProjects(t *testing.T) {
	root := t.TempDir()
	projectPath := t.TempDir()
	store := NewStore(root)
	first, err := store.Add("  Test Project  ", projectPath, " CODEX ")
	if err != nil {
		t.Fatal(err)
	}
	if first.Name != "Test Project" || first.DefaultRunner != "codex" || !filepath.IsAbs(first.Path) {
		t.Fatalf("project was not normalized: %+v", first)
	}
	if first.CreatedAt == "" {
		t.Fatal("project created_at was not recorded")
	}
	second, err := store.Add("Different Name", projectPath, "shell")
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID || second.Name != first.Name {
		t.Fatalf("same path created a different Project: %+v", second)
	}
	items, err := store.List()
	if err != nil || len(items) != 1 {
		t.Fatalf("project list = %+v, %v", items, err)
	}
}

func TestAddExpandsTildeToHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(t.TempDir())
	item, err := store.Add("Home", "~", "shell")
	if err != nil {
		t.Fatal(err)
	}
	if item.Path != filepath.Clean(home) {
		t.Fatalf("tilde path = %q, want %q", item.Path, home)
	}
}

func TestAddRejectsMissingPathsAndUnknownRunners(t *testing.T) {
	store := NewStore(t.TempDir())
	if _, err := store.Add("Missing", filepath.Join(t.TempDir(), "missing"), "shell"); err == nil {
		t.Fatal("missing Project path was accepted")
	}
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Add("File", file, "shell"); err == nil {
		t.Fatal("file Project path was accepted")
	}
	if _, err := store.Add("Unknown", t.TempDir(), "plugin"); err == nil {
		t.Fatal("unknown default runner was accepted")
	}
}

func TestRegistryValidationAndRemove(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	item, err := store.Add("Validated", t.TempDir(), "opencode")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := store.Remove(item.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(item.ID); err != ErrNotFound {
		t.Fatalf("removed Project error = %v", err)
	}
	if err := store.Remove(item.ID); err != ErrNotFound {
		t.Fatalf("second remove error = %v", err)
	}
}
