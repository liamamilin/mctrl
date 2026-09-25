package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRuntimeIdentityProtectsProfileStateRoots(t *testing.T) {
	root := t.TempDir()
	if err := EnsureRuntimeIdentity(root, "v1"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRuntimeIdentity(root, "v1"); err != nil {
		t.Fatalf("v1 identity rejected: %v", err)
	}
	if err := ValidateRuntimeIdentity(root, "v2"); err == nil {
		t.Fatal("v2 opened a v1 state root")
	}

	v2Root := filepath.Join(t.TempDir(), "v2-state")
	if err := EnsureRuntimeIdentity(v2Root, "v2"); err == nil {
		t.Fatal("v2 identity was created in a non-existent state root")
	}

	badRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(badRoot, runtimeIdentityFile), []byte(`{"schema_version":99,"profile":"v2"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRuntimeIdentity(badRoot, "v2"); err == nil {
		t.Fatal("unsupported runtime identity schema was accepted")
	}
}
