package config

import (
	"path/filepath"
	"testing"
)

func TestDefaultUsesLoopbackAndDocumentedProfiles(t *testing.T) {
	cfg := Default()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("default config invalid: %v", err)
	}
	if cfg.ListenAddress != "127.0.0.1" {
		t.Fatalf("default listen address = %q, want loopback", cfg.ListenAddress)
	}
	if cfg.TransportProfile != TransportTrustedLANHTTP {
		t.Fatalf("default transport = %q", cfg.TransportProfile)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	want := Default()
	want.DeviceName = "Test Mac"
	want.PublicURL = "http://mctrl.test:9999"
	want.AllowedOrigins = []string{"http://127.0.0.1:7681"}
	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.DeviceName != want.DeviceName || got.PublicURL != want.PublicURL || len(got.AllowedOrigins) != 1 {
		t.Fatalf("round trip mismatch: %+v", got)
	}
}

func TestTLSTerminatedRequiresLoopbackListenerAndPublicURL(t *testing.T) {
	cfg := Default()
	cfg.TransportProfile = TransportTLSTerminated
	cfg.ListenAddress = "0.0.0.0"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected TLS-terminated profile to reject a public bind")
	}
	cfg.ListenAddress = "127.0.0.1"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected TLS-terminated profile to require public_url")
	}
	cfg.PublicURL = "https://mctrl.example.test"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("loopback TLS-terminated config invalid: %v", err)
	}
	cfg.PublicURL = "http://mctrl.example.test"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected TLS-terminated profile to reject an HTTP public URL")
	}
}

func TestTransportProfilesRejectMismatchedOrigins(t *testing.T) {
	cfg := Default()
	cfg.AllowedOrigins = []string{"https://mctrl.example.test"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("trusted LAN profile accepted an HTTPS allowed origin")
	}
	cfg = Default()
	cfg.TransportProfile = TransportTLSTerminated
	cfg.PublicURL = "https://mctrl.example.test"
	cfg.AllowedOrigins = []string{"http://mctrl.example.test"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("TLS-terminated profile accepted an HTTP allowed origin")
	}
}

func TestValidateRejectsUnknownValues(t *testing.T) {
	cfg := Default()
	cfg.TransportProfile = "magic"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected unknown transport profile to fail validation")
	}
	cfg = Default()
	cfg.Port = 70000
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid port to fail validation")
	}
}
