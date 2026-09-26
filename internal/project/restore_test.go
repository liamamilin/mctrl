package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	// The store creates projects.json on first write; an empty root is a valid
	// starting point, which is exactly the state this recovery path exists for.
	return NewStore(dir), dir
}

func TestRestoreReinstatesAnExistingID(t *testing.T) {
	store, _ := newTestStore(t)
	newPath := t.TempDir()
	restoredPath := t.TempDir()

	// The recorded failure: Add mints a new id, so a Work referencing the old
	// one is left pointing at nothing.
	added, err := store.Add("replacement", newPath, "shell")
	if err != nil {
		t.Fatal(err)
	}
	restored, err := store.Restore("project-E_zCrQ", "ai-research", restoredPath, "shell")
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if restored.ID != "project-E_zCrQ" {
		t.Fatalf("restored id = %q, want project-E_zCrQ", restored.ID)
	}
	if added.ID == restored.ID {
		t.Fatal("Restore reused the id that Add had already taken")
	}
	items, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("registry holds %d Projects, want 2", len(items))
	}
	byID := map[string]Project{}
	for _, item := range items {
		byID[item.ID] = item
	}
	record, ok := byID["project-E_zCrQ"]
	if !ok {
		t.Fatal("restored Project is not in the registry")
	}
	if record.Name != "ai-research" || record.Path != restoredPath || record.DefaultRunner != "shell" {
		t.Fatalf("restored record = %+v", record)
	}
	if record.CreatedAt == "" {
		t.Fatal("restored record has no created_at")
	}
}

func TestRestoreDefaultsNameAndRunner(t *testing.T) {
	store, _ := newTestStore(t)
	path := t.TempDir()
	restored, err := store.Restore("project-legacy", "", path, "")
	if err != nil {
		t.Fatal(err)
	}
	if restored.Name != filepath.Base(path) {
		t.Fatalf("name = %q, want the directory base %q", restored.Name, filepath.Base(path))
	}
	if restored.DefaultRunner != "shell" {
		t.Fatalf("runner = %q, want shell", restored.DefaultRunner)
	}
}

func TestRestoreRefusesConflicts(t *testing.T) {
	store, _ := newTestStore(t)
	first := t.TempDir()
	second := t.TempDir()

	if _, err := store.Restore("project-dup", "one", first, "shell"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Restore("project-dup", "again", second, "shell"); err == nil {
		t.Fatal("Restore accepted an id that already exists")
	}
	if _, err := store.Add("other", second, "shell"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Restore("project-other-id", "clash", second, "shell"); err == nil {
		t.Fatal("Restore accepted a path another id already owns")
	}
}

func TestRestoreRejectsUnusableInput(t *testing.T) {
	store, _ := newTestStore(t)
	dir := t.TempDir()
	missing := filepath.Join(t.TempDir(), "not-there")
	file := filepath.Join(t.TempDir(), "a-file")
	if err := os.WriteFile(file, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name         string
		id           string
		path         string
		defaultRuner string
	}{
		{name: "empty id", id: "", path: dir},
		{name: "id with a space", id: "project-a b", path: dir},
		{name: "id with a separator", id: "project/a", path: dir},
		{name: "id too long", id: strings.Repeat("p", 129), path: dir},
		{name: "empty path", id: "project-x", path: ""},
		{name: "missing path", id: "project-x", path: missing},
		{name: "path is a file", id: "project-x", path: file},
		{name: "unsupported runner", id: "project-x", path: dir, defaultRuner: "nope"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := store.Restore(test.id, "name", test.path, test.defaultRuner); err == nil {
				t.Fatal("Restore accepted unusable input")
			}
		})
	}
}

func TestRestoreAcceptsLegacyIDShapes(t *testing.T) {
	// Ids minted by older versions look nothing like today's "<slug>-<random>",
	// and the dangling records in the wild use that older shape.
	store, _ := newTestStore(t)
	for index, id := range []string{"project-E_zCrQ", "mctrl-iphone-e2e-3G5a8w", "a", "0"} {
		path := t.TempDir()
		if _, err := store.Restore(id, "legacy", path, "shell"); err != nil {
			t.Fatalf("legacy id %q (case %d) was rejected: %v", id, index, err)
		}
	}
}

func TestRestoreExpandsTilde(t *testing.T) {
	store, _ := newTestStore(t)
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory in this environment")
	}
	restored, err := store.Restore("project-tilde", "home", "~", "shell")
	if err != nil {
		t.Fatal(err)
	}
	if restored.Path != home {
		t.Fatalf("~ expanded to %q, want %q", restored.Path, home)
	}
}
