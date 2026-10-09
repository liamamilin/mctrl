package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDesktopConnectionKeepsProfileAndCustomSocket(t *testing.T) {
	root := t.TempDir()
	if err := SaveDesktopConnection(root, "v2", "/tmp/custom socket"); err != nil {
		t.Fatal(err)
	}
	value, err := LoadDesktopConnection(root, "v2")
	if err != nil || value.Socket != "/tmp/custom socket" {
		t.Fatalf("connection = %+v, %v", value, err)
	}
	if _, err := LoadDesktopConnection(root, "v1"); err == nil {
		t.Fatal("other profile connection was accepted")
	}
	info, err := os.Stat(filepath.Join(root, "desktop.json"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("connection permissions are not private")
	}
}
