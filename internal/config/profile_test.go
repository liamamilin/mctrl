package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestProfileDefaultsKeepV1AndIsolateV2(t *testing.T) {
	v1, err := ProfileForName("v1")
	if err != nil {
		t.Fatal(err)
	}
	if v1.StateDir("/tmp/example") != filepath.Join("/tmp/example", ".mctrl") || v1.LaunchAgentLabel != "com.mctrl.daemon" || v1.DefaultPort != 7681 {
		t.Fatalf("v1 profile changed historical defaults: %+v", v1)
	}
	v2, err := ProfileForName("v2")
	if err != nil {
		t.Fatal(err)
	}
	if v2.StateDir("/tmp/example") != filepath.Join("/tmp/example", ".mctrl-v2") || v2.LaunchAgentLabel != "com.mctrl.v2.daemon" || v2.DefaultPort != 17682 {
		t.Fatalf("v2 profile is not isolated: %+v", v2)
	}
	if v1.PairAppName != "Mctrl Pair" || v1.BundleIdentifier != "com.mctrl.pair-launcher" {
		t.Fatalf("v1 app identity changed: %+v", v1)
	}
	expectedSocket := filepath.Join("/private/tmp", fmt.Sprintf("tmux-%d/mctrl-v2", os.Getuid()))
	if v2.DefaultTmuxSocket != expectedSocket {
		t.Fatalf("v2 socket = %q, want %q", v2.DefaultTmuxSocket, expectedSocket)
	}
	if v2.PairAppName != "Mctrl V2 Pair" || v2.BundleIdentifier != "com.mctrl.v2.pair-launcher" {
		t.Fatalf("v2 endpoint metadata is incomplete: %+v", v2)
	}
}

func TestProfileNamesAreValidated(t *testing.T) {
	for _, name := range []string{"", "v0", "v01", "v2x", "prod", "v-2", "v10"} {
		if _, err := ProfileForName(name); err == nil && name != "" {
			t.Fatalf("invalid profile %q was accepted", name)
		}
	}
}

func TestStateDirUsesProfileUnlessHomeIsExplicit(t *testing.T) {
	t.Setenv(ProfileEnv, "v2")
	t.Setenv("MCTRL_HOME", "")
	state, err := StateDir()
	if err != nil {
		t.Fatal(err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	if state != filepath.Join(home, ".mctrl-v2") {
		t.Fatalf("v2 state directory = %q", state)
	}
	explicit := filepath.Join(t.TempDir(), "state")
	t.Setenv("MCTRL_HOME", explicit)
	state, err = StateDir()
	if err != nil || state != explicit {
		t.Fatalf("explicit state directory = %q, err=%v", state, err)
	}
}

func TestV2DoesNotAdoptUnmarkedExistingConfig(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "config.json"), []byte(`{"schema_version":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(ProfileEnv, "v2")
	t.Setenv("MCTRL_HOME", root)
	if _, err := EnsureStateDir(); err == nil {
		t.Fatal("v2 adopted an unmarked existing config")
	}
}

func TestApplyProfileEnvironmentDoesNotOverrideExplicitTmuxSocket(t *testing.T) {
	t.Setenv("MCTRL_TMUX_SOCKET", "/tmp/explicit.sock")
	v2, err := ProfileForName("v2")
	if err != nil {
		t.Fatal(err)
	}
	if err := v2.ApplyEnvironment(); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("MCTRL_TMUX_SOCKET"); got != "/tmp/explicit.sock" {
		t.Fatalf("explicit tmux socket was overwritten: %q", got)
	}
}
