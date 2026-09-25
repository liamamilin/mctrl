package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"mctrl/internal/config"
)

func TestExtractProfileFlagSupportsGlobalForms(t *testing.T) {
	args, profile, err := extractProfileFlag([]string{"--profile", "v2", "status", "--profile=v3"})
	if err != nil || profile != "v3" || strings.Join(args, " ") != "status" {
		t.Fatalf("profile extraction = args=%v profile=%q err=%v", args, profile, err)
	}
	if _, _, err := extractProfileFlag([]string{"status", "--profile"}); err == nil {
		t.Fatal("profile flag without a value was accepted")
	}
}

func TestV2SetupUsesIsolatedPortAndStateIdentity(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is not installed")
	}
	t.Setenv(config.ProfileEnv, "v2")
	t.Setenv("MCTRL_HOME", t.TempDir())
	t.Setenv("MCTRL_TMUX_SOCKET", "")
	if err := runSetup([]string{"--no-start"}); err != nil {
		t.Fatal(err)
	}
	stateDir := os.Getenv("MCTRL_HOME")
	cfg, err := config.Load(filepath.Join(stateDir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 17682 {
		t.Fatalf("v2 setup port = %d, want 17682", cfg.Port)
	}
	if err := config.ValidateRuntimeIdentity(stateDir, "v2"); err != nil {
		t.Fatal(err)
	}
	if activeLaunchAgentLabel() != "com.mctrl.v2.daemon" {
		t.Fatalf("v2 launch label = %q", activeLaunchAgentLabel())
	}
}

func TestV1AndV2LaunchTargetsAreDistinct(t *testing.T) {
	t.Setenv(config.ProfileEnv, "v1")
	v1Target := launchctlServiceTarget()
	v1Path, err := launchAgentPath()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(config.ProfileEnv, "v2")
	v2Target := launchctlServiceTarget()
	v2Path, err := launchAgentPath()
	if err != nil {
		t.Fatal(err)
	}
	if v1Target == v2Target || v1Path == v2Path || !strings.HasSuffix(v1Path, "com.mctrl.daemon.plist") || !strings.HasSuffix(v2Path, "com.mctrl.v2.daemon.plist") {
		t.Fatalf("launch targets overlap: v1=%s/%s v2=%s/%s", v1Target, v1Path, v2Target, v2Path)
	}
}

func TestRenderedLaunchAgentsCarryProfileIsolation(t *testing.T) {
	v1, err := config.ProfileForName("v1")
	if err != nil {
		t.Fatal(err)
	}
	v2, err := config.ProfileForName("v2")
	if err != nil {
		t.Fatal(err)
	}
	v1Plist := renderLaunchAgent(v1, "/tmp/mctrl", "/tmp/state-v1", "/usr/bin", "")
	if !strings.Contains(v1Plist, "<string>com.mctrl.daemon</string>") || !strings.Contains(v1Plist, "<string>/tmp/state-v1</string>") || strings.Contains(v1Plist, "MCTRL_TMUX_SOCKET") {
		t.Fatalf("v1 plist identity changed: %s", v1Plist)
	}
	v2Plist := renderLaunchAgent(v2, "/tmp/mctrl-v2", "/tmp/state-v2", "/usr/bin", v2.DefaultTmuxSocket)
	for _, expected := range []string{"com.mctrl.v2.daemon", "/tmp/state-v2", "MCTRL_TMUX_SOCKET", v2.DefaultTmuxSocket} {
		if !strings.Contains(v2Plist, expected) {
			t.Fatalf("v2 plist missing %q: %s", expected, v2Plist)
		}
	}
}

func TestAdvertisedPairURLUsesConfiguredPublicOrigin(t *testing.T) {
	cfg := config.Default()
	cfg.PublicURL = "https://mctrl.example.test/"
	if got := advertisedPairURL(cfg); got != "https://mctrl.example.test/pair" {
		t.Fatalf("pair URL = %q", got)
	}
}

func TestPairingConsoleURLUsesLocalBrowserSurface(t *testing.T) {
	cfg := config.Default()
	cfg.ListenAddress = "0.0.0.0"
	cfg.Port = 17681
	got := pairingConsoleURL(cfg, "token-value")
	if got != "http://127.0.0.1:17681/#/pair-console?token=token-value" {
		t.Fatalf("pair console URL = %q", got)
	}
	cfg.PublicURL = "https://mctrl.example.test"
	got = pairingConsoleURL(cfg, "token-value")
	if got != "https://mctrl.example.test/#/pair-console?token=token-value" {
		t.Fatalf("TLS pair console URL = %q", got)
	}
}

func TestPairOpenRequiresRunningDaemon(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MCTRL_HOME", root)
	if err := runSetup([]string{"--no-start"}); err != nil {
		t.Fatal(err)
	}
	if err := runPair([]string{"--open"}); err == nil || !strings.Contains(err.Error(), "daemon is not running") {
		t.Fatalf("pair --open error = %v", err)
	}
}

func TestLaunchctlDomainAndServiceTargetsAreDistinct(t *testing.T) {
	t.Setenv(config.ProfileEnv, "v1")
	domain := launchctlDomain()
	service := launchctlServiceTarget()
	if !strings.HasPrefix(domain, "gui/") || !strings.HasSuffix(service, "/"+launchAgentLabel) {
		t.Fatalf("launchctl targets domain=%q service=%q", domain, service)
	}
	if strings.Contains(service, ".plist") {
		t.Fatalf("service target contains plist path: %q", service)
	}
}

func TestSetupRequiresPublicURLForTLSTerminatedProfile(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is not installed")
	}
	root := t.TempDir()
	t.Setenv("MCTRL_HOME", root)
	err := runSetup([]string{"--transport", "tls_terminated", "--no-start"})
	if err == nil || !strings.Contains(err.Error(), "--public-url") {
		t.Fatalf("TLS setup error = %v", err)
	}
}

func TestSetupPersistsTLSOriginAndLoginPolicy(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is not installed")
	}
	root := t.TempDir()
	t.Setenv("MCTRL_HOME", root)
	if err := runSetup([]string{
		"--transport", "tls_terminated",
		"--public-url", "https://mctrl.example.test",
		"--no-start",
		"--no-start-after-login",
	}); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(filepath.Join(root, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TransportProfile != config.TransportTLSTerminated || cfg.PublicURL != "https://mctrl.example.test" || cfg.StartAfterLogin {
		t.Fatalf("unexpected persisted setup: %+v", cfg)
	}
}
