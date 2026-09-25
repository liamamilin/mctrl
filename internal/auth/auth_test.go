package auth

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBrowserOriginRequiresOriginAndMatchingTransport(t *testing.T) {
	request := httptest.NewRequest("POST", "http://example.test/api", nil)
	if ValidateBrowserOrigin(request, nil, false) {
		t.Fatal("empty browser Origin was accepted")
	}
	request.Header.Set("Origin", "http://example.test")
	if !ValidateBrowserOrigin(request, nil, false) {
		t.Fatal("same-origin HTTP request was rejected")
	}
	if ValidateBrowserOrigin(request, nil, true) {
		t.Fatal("HTTP Origin was accepted for TLS profile")
	}
	request.Header.Set("Origin", "https://example.test")
	if !ValidateBrowserOrigin(request, nil, true) {
		t.Fatal("same-origin HTTPS request was rejected")
	}
}

func TestMalformedAuthorizationDoesNotFallBackToCookie(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	request.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "cookie-session"})
	request.Header.Set("Authorization", "Basic malformed")
	credential, viaCookie := RequestCredentialWithSource(request)
	if credential != "" || viaCookie {
		t.Fatalf("malformed authorization fell back to cookie: credential=%q viaCookie=%v", credential, viaCookie)
	}
	request.Header.Set("Authorization", "Bearer api-token")
	credential, viaCookie = RequestCredentialWithSource(request)
	if credential != "api-token" || viaCookie {
		t.Fatalf("bearer credential source = %q, %v", credential, viaCookie)
	}
}

func TestAuthenticationThrottlesLastSeenPersistence(t *testing.T) {
	root := t.TempDir()
	pairing, err := CreatePairing(root, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry(root)
	result, err := registry.Pair(root, pairing.Token, "Polling Phone")
	if err != nil {
		t.Fatal(err)
	}
	stale := time.Now().UTC().Add(-2 * lastSeenWriteInterval)
	if err := registry.mutate(func(value *registryFile) error {
		value.Devices[0].LastSeen = stale
		for index := range value.Devices[0].Sessions {
			value.Devices[0].Sessions[index].LastSeen = stale
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := registry.Authenticate(result.AccessToken); err != nil {
		t.Fatal(err)
	}
	firstBytes, err := os.ReadFile(filepath.Join(root, "devices.json"))
	if err != nil {
		t.Fatal(err)
	}
	devices, err := registry.List()
	if err != nil {
		t.Fatal(err)
	}
	firstSeen := devices[0].LastSeen
	if !firstSeen.After(stale) {
		t.Fatalf("LastSeen was not refreshed: %v", firstSeen)
	}
	if _, err := registry.Authenticate(result.AccessToken); err != nil {
		t.Fatal(err)
	}
	secondBytes, err := os.ReadFile(filepath.Join(root, "devices.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstBytes, secondBytes) {
		t.Fatal("fresh authentication rewrote the device registry")
	}
}

func TestBrowserSessionExpiresWithoutRevokingDeviceCredential(t *testing.T) {
	root := t.TempDir()
	pairing, err := CreatePairing(root, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry(root)
	result, err := registry.Pair(root, pairing.Token, "Expiring Phone")
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.mutate(func(value *registryFile) error {
		value.Devices[0].Sessions[0].ExpiresAt = time.Now().UTC().Add(-time.Minute)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Authenticate(result.SessionToken); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expired browser session error = %v", err)
	}
	if _, err := registry.Authenticate(result.AccessToken); err != nil {
		t.Fatalf("device credential was revoked with browser session: %v", err)
	}
}

func TestPairForInstallationReusesAndRevivesDevice(t *testing.T) {
	root := t.TempDir()
	registry := NewRegistry(root)
	installationID := "installation-id-AAAAAAAAAAAA"
	firstPairing, err := CreatePairing(root, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	first, err := registry.PairForInstallation(root, firstPairing.Token, "My iPhone", installationID)
	if err != nil {
		t.Fatal(err)
	}
	if first.Reused || first.Device.InstallationHash == "" {
		t.Fatalf("first pair result = %+v", first)
	}
	if err := registry.Revoke(first.Device.ID); err != nil {
		t.Fatal(err)
	}

	secondPairing, err := CreatePairing(root, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	second, err := registry.PairForInstallation(root, secondPairing.Token, "My iPhone PWA", installationID)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Reused || second.Device.ID != first.Device.ID || second.Device.Name != "My iPhone PWA" || second.Device.RevokedAt != nil {
		t.Fatalf("reused pair result = %+v", second)
	}
	if _, err := registry.Authenticate(first.AccessToken); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("old device credential error = %v", err)
	}
	if _, err := registry.Authenticate(first.SessionToken); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("old browser session error = %v", err)
	}
	if _, err := registry.Authenticate(second.SessionToken); err != nil {
		t.Fatalf("new browser session was not activated: %v", err)
	}
	devices, err := registry.List()
	if err != nil {
		t.Fatal(err)
	}
	active, err := registry.ListActive()
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 || len(active) != 1 || active[0].ID != first.Device.ID {
		t.Fatalf("devices = %#v, active = %#v", devices, active)
	}
}

func TestDeviceLinkAddsSecondBrowserToOneDeviceRecord(t *testing.T) {
	root := t.TempDir()
	registry := NewRegistry(root)
	pairing, err := CreatePairing(root, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	paired, err := registry.PairForInstallation(root, pairing.Token, "My iPhone", "primary-installation-AAAA")
	if err != nil {
		t.Fatal(err)
	}
	principal, err := registry.Authenticate(paired.SessionToken)
	if err != nil {
		t.Fatal(err)
	}
	ticket, err := registry.CreateDeviceLink(principal, 2*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := os.ReadFile(filepath.Join(root, "devices.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(persisted), ticket.Token) {
		t.Fatal("plaintext browser link token was persisted")
	}

	linked, err := registry.LinkBrowser(ticket.Token, "safari-installation-BBBB")
	if err != nil {
		t.Fatal(err)
	}
	if linked.Device.ID != paired.Device.ID || len(linked.Device.Sessions) != 2 {
		t.Fatalf("linked device = %+v", linked)
	}
	if _, err := registry.Authenticate(paired.SessionToken); err != nil {
		t.Fatalf("original PWA session was revoked by linking: %v", err)
	}
	if _, err := registry.Authenticate(linked.SessionToken); err != nil {
		t.Fatalf("new Safari session was not activated: %v", err)
	}
	devices, err := registry.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 || len(deviceInstallationHashes(devices[0])) != 2 {
		t.Fatalf("linked devices = %#v", devices)
	}
	if _, err := registry.LinkBrowser(ticket.Token, "safari-installation-BBBB"); !errors.Is(err, ErrDeviceLinkInvalid) {
		t.Fatalf("second link use error = %v", err)
	}

	replacementTicket, err := registry.CreateDeviceLink(principal, 2*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	replacement, err := registry.LinkBrowser(replacementTicket.Token, "safari-installation-BBBB")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Authenticate(linked.SessionToken); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("old Safari session error = %v", err)
	}
	if _, err := registry.Authenticate(replacement.SessionToken); err != nil {
		t.Fatalf("replacement Safari session error = %v", err)
	}
	if _, err := registry.Authenticate(paired.SessionToken); err != nil {
		t.Fatalf("replacement Safari session revoked PWA session: %v", err)
	}
	devices, err = registry.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 || len(devices[0].Sessions) != 2 {
		t.Fatalf("sessions after same-browser replacement = %#v", devices)
	}
}

func TestRevokeCancelsOutstandingDeviceLinks(t *testing.T) {
	root := t.TempDir()
	registry := NewRegistry(root)
	pairing, err := CreatePairing(root, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	paired, err := registry.PairForInstallation(root, pairing.Token, "Phone", "revoke-installation-AAAA")
	if err != nil {
		t.Fatal(err)
	}
	principal, err := registry.Authenticate(paired.SessionToken)
	if err != nil {
		t.Fatal(err)
	}
	ticket, err := registry.CreateDeviceLink(principal, 2*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Revoke(paired.Device.ID); err != nil {
		t.Fatal(err)
	}
	reactivation, err := CreatePairing(root, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.PairForInstallation(root, reactivation.Token, "Phone", "revoke-installation-AAAA"); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.LinkBrowser(ticket.Token, "late-safari-installation-BBBB"); !errors.Is(err, ErrDeviceLinkInvalid) {
		t.Fatalf("revoked link error = %v", err)
	}
}

func TestInvalidInstallationIDDoesNotConsumePairingToken(t *testing.T) {
	root := t.TempDir()
	registry := NewRegistry(root)
	pairing, err := CreatePairing(root, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.PairForInstallation(root, pairing.Token, "Phone", "short"); !errors.Is(err, ErrInvalidInstallationID) {
		t.Fatalf("invalid installation error = %v", err)
	}
	if _, err := registry.PairForInstallation(root, pairing.Token, "Phone", "valid-installation-CCCCCCCC"); err != nil {
		t.Fatalf("valid pairing after rejected identity failed: %v", err)
	}
}

func TestBindInstallationMigratesExistingDeviceWithoutAddingARecord(t *testing.T) {
	root := t.TempDir()
	registry := NewRegistry(root)
	pairing, err := CreatePairing(root, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	paired, err := registry.Pair(root, pairing.Token, "Existing Browser")
	if err != nil {
		t.Fatal(err)
	}
	principal, err := registry.Authenticate(paired.SessionToken)
	if err != nil {
		t.Fatal(err)
	}
	installationID := "existing-installation-BBBBBBBB"
	if err := registry.BindInstallation(principal, installationID); err != nil {
		t.Fatal(err)
	}
	if err := registry.BindInstallation(principal, installationID); err != nil {
		t.Fatal(err)
	}
	devices, err := registry.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 || devices[0].ID != principal.Device.ID || devices[0].InstallationHash == "" || devices[0].Sessions[0].InstallationHash == "" {
		t.Fatalf("bound devices = %#v", devices)
	}
}

func TestPairingIsOneTimeAndCredentialsAreHashed(t *testing.T) {
	root := t.TempDir()
	pairing, err := CreatePairing(root, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry(root)
	result, err := registry.Pair(root, pairing.Token, "Phone")
	if err != nil {
		t.Fatal(err)
	}
	if result.AccessToken == "" || result.SessionToken == "" || result.CSRFToken == "" {
		t.Fatal("pairing did not return all credentials")
	}
	registryBytes, err := os.ReadFile(filepath.Join(root, "devices.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{result.AccessToken, result.SessionToken, result.CSRFToken} {
		if strings.Contains(string(registryBytes), secret) {
			t.Fatal("plaintext credential was persisted")
		}
	}
	if _, err := registry.Pair(root, pairing.Token, "Phone 2"); !errors.Is(err, ErrPairingClosed) {
		t.Fatalf("second pair error = %v, want pairing closed", err)
	}
	principal, err := registry.Authenticate(result.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if principal.Device.Name != "Phone" {
		t.Fatalf("authenticated device = %q", principal.Device.Name)
	}
	cookieRequest := httptest.NewRequest("GET", "http://example.test/", nil)
	cookieRequest.AddCookie(&http.Cookie{Name: SessionCookieName, Value: result.SessionToken})
	principal, err = registry.Authenticate(RequestCredential(cookieRequest))
	if err != nil {
		t.Fatal(err)
	}
	if principal.SessionID == "" {
		t.Fatal("session credential did not resolve a session")
	}
	csrf, err := registry.CSRF(principal)
	if err != nil {
		t.Fatal(err)
	}
	if !registry.CheckCSRF(principal, csrf) {
		t.Fatal("fresh CSRF token was rejected")
	}
	if err := registry.Revoke(principal.Device.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Authenticate(result.AccessToken); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("revoked credential error = %v", err)
	}
}
