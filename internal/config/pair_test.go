package config

import (
	"strings"
	"testing"
)

func TestLANPairingSurvivesAddressChange(t *testing.T) {
	cfg := Default()
	cfg.ListenAddress, cfg.Port = "0.0.0.0", 17681
	before := pairAddresses(cfg, "My-Mac", "192.168.2.105", "unrelated-dhcp-name")
	after := pairAddresses(cfg, "My-Mac", "192.168.2.226", "another-dhcp-name")
	if before.Primary != "http://My-Mac.local:17681/pair" || after.Primary != before.Primary {
		t.Fatalf("phone origin changed with DHCP: before=%+v after=%+v", before, after)
	}
	if before.Fallback != "http://192.168.2.105:17681/pair" || after.Fallback != "http://192.168.2.226:17681/pair" {
		t.Fatalf("fallback did not track the current LAN: before=%+v after=%+v", before, after)
	}
}

func TestPairAddressPolicies(t *testing.T) {
	for _, test := range []struct {
		name, listener, public, local, ip, host, primary, fallback string
	}{
		{"loopback", "127.0.0.1", "", "My-Mac", "192.168.2.226", "", "http://127.0.0.1:17681/pair", ""},
		{"IPv6 loopback", "::1", "", "My-Mac", "", "", "http://[::1]:17681/pair", ""},
		{"explicit bind", "192.168.2.226", "", "My-Mac", "192.168.2.105", "", "http://192.168.2.226:17681/pair", ""},
		{"custom public origin", "0.0.0.0", "http://custom.test:9999/", "My-Mac", "192.168.2.226", "", "http://custom.test:9999/pair", ""},
		{"IPv6 wildcard", "::", "", "My-Mac.local", "192.168.2.226", "", "http://My-Mac.local:17681/pair", "http://192.168.2.226:17681/pair"},
		{"no Bonjour identity", "0.0.0.0", "", "", "192.168.2.226", "", "http://192.168.2.226:17681/pair", ""},
		{"invalid Bonjour identity", "0.0.0.0", "", "not/a/host", "192.168.2.226", "", "http://192.168.2.226:17681/pair", ""},
		{"Bonjour without IPv4", "0.0.0.0", "", "My-Mac", "", "", "http://My-Mac.local:17681/pair", ""},
		{"legacy hostname fallback", "0.0.0.0", "", "", "", "fallback-host", "http://fallback-host:17681/pair", ""},
		{"no network identity", "0.0.0.0", "", "", "", "", "http://127.0.0.1:17681/pair", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg := Default()
			cfg.ListenAddress, cfg.Port, cfg.PublicURL = test.listener, 17681, test.public
			got := pairAddresses(cfg, test.local, test.ip, test.host)
			if got.Primary != test.primary || got.Fallback != test.fallback {
				t.Fatalf("pair addresses = %+v", got)
			}
		})
	}
	cfg := Default()
	cfg.TransportProfile, cfg.PublicURL = TransportTLSTerminated, "https://secure.test"
	got := pairAddresses(cfg, "My-Mac", "192.168.2.226", "")
	if got.Primary != "https://secure.test/pair" || got.Fallback != "" {
		t.Fatalf("TLS origin gained a plaintext fallback: %+v", got)
	}
}

func TestLocalHostnameValidation(t *testing.T) {
	for _, name := range []string{"", "localhost", "localhost.local", "a.b", "my mac", "-mac", "mac-", "mac/path", "mac:123", "mac\nother", strings.Repeat("a", 64)} {
		if got := normalizeLocalHostname(name); got != "" {
			t.Errorf("invalid hostname %q became %q", name, got)
		}
	}
	if got := normalizeLocalHostname("  My-Mac.LOCAL\n"); got != "My-Mac.local" {
		t.Fatalf("canonical Bonjour hostname = %q", got)
	}
}
