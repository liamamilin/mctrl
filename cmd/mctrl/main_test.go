package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"mctrl/internal/config"
)

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
