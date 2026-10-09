package config

import (
	"context"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// PairAddresses separates the preferred browser origin from a temporary LAN
// fallback. Neither URL includes a credential; both use the same one-time code.
type PairAddresses struct {
	Primary  string
	Fallback string
}

func AdvertisedPairAddresses(c Config) PairAddresses {
	if strings.TrimSpace(c.PublicURL) != "" || !wildcardListener(c.ListenAddress) {
		return pairAddresses(c, "", "", "")
	}
	name, _ := os.Hostname()
	return pairAddresses(c, localPairHostname(), lanAddress(), name)
}

func wildcardListener(host string) bool {
	return host == "" || host == "0.0.0.0" || host == "::"
}

func pairAddresses(c Config, localName, lanIP, hostName string) PairAddresses {
	if publicURL := strings.TrimRight(strings.TrimSpace(c.PublicURL), "/"); publicURL != "" {
		return PairAddresses{Primary: publicURL + "/pair"}
	}
	scheme := "http"
	if c.TransportProfile == TransportTLSTerminated {
		scheme = "https"
	}
	urlFor := func(host string) string {
		return scheme + "://" + net.JoinHostPort(host, strconv.Itoa(c.Port)) + "/pair"
	}
	if !wildcardListener(c.ListenAddress) {
		return PairAddresses{Primary: urlFor(c.ListenAddress)}
	}
	if stableName := normalizeLocalHostname(localName); stableName != "" && c.TransportProfile == TransportTrustedLANHTTP {
		addresses := PairAddresses{Primary: urlFor(stableName)}
		if lanIP != "" {
			addresses.Fallback = urlFor(lanIP)
		}
		return addresses
	}
	if lanIP != "" {
		return PairAddresses{Primary: urlFor(lanIP)}
	}
	if hostName == "" {
		hostName = "127.0.0.1"
	}
	return PairAddresses{Primary: urlFor(hostName)}
}

// Use macOS's Bonjour identity, not the hostname obtained from DHCP/reverse DNS.
// A bounded lookup falls back to the current LAN address when unavailable.
func localPairHostname() string {
	if runtime.GOOS != "darwin" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "/usr/sbin/scutil", "--get", "LocalHostName").Output()
	if err != nil {
		return ""
	}
	return normalizeLocalHostname(string(output))
}

func normalizeLocalHostname(name string) string {
	name = strings.TrimSpace(name)
	if strings.HasSuffix(strings.ToLower(name), ".local") {
		name = name[:len(name)-len(".local")]
	}
	if name == "" || len(name) > 63 || name[0] == '-' || name[len(name)-1] == '-' || strings.EqualFold(name, "localhost") {
		return ""
	}
	for _, char := range name {
		if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '-') {
			return ""
		}
	}
	return name + ".local"
}
