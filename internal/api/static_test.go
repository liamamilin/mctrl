package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"mctrl/internal/config"
)

func TestHealthReportsControlPlaneDegradationWithoutRawErrors(t *testing.T) {
	cfg := config.Default()
	cfg.RemoteAvailability = config.AvailabilityWorkOnly
	server, err := NewServer(cfg, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	server.RecordControlError(errors.New("/private/path detail"))
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"status":"degraded"`) || strings.Contains(recorder.Body.String(), "/private/path") {
		t.Fatalf("degraded health response = %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestReadJSONBodyRejectsTrailingValues(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"one"}{"name":"two"}`))
	var value map[string]string
	if err := readJSONBody(request, &value); err == nil {
		t.Fatal("multiple JSON values were accepted")
	}
}

func TestEmbeddedPWAHasSecurityAndCachePolicy(t *testing.T) {
	cfg := config.Default()
	cfg.RemoteAvailability = config.AvailabilityWorkOnly
	server, err := NewServer(cfg, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	handler := server.Handler()

	index := httptest.NewRecorder()
	handler.ServeHTTP(index, httptest.NewRequest(http.MethodGet, "/", nil))
	if index.Code != http.StatusOK {
		t.Fatalf("index status = %d", index.Code)
	}
	if policy := index.Header().Get("Content-Security-Policy"); !strings.Contains(policy, "frame-ancestors 'none'") || !strings.Contains(policy, "connect-src 'self' ws: wss:") {
		t.Fatalf("unexpected CSP: %q", policy)
	}
	if cache := index.Header().Get("Cache-Control"); cache != "no-cache" {
		t.Fatalf("index cache policy = %q", cache)
	}

	serviceWorker := httptest.NewRecorder()
	handler.ServeHTTP(serviceWorker, httptest.NewRequest(http.MethodGet, "/sw.js", nil))
	if serviceWorker.Code != http.StatusOK {
		t.Fatalf("service worker status = %d", serviceWorker.Code)
	}
	if strings.Contains(serviceWorker.Body.String(), "__BUILD_ID__") || !regexp.MustCompile(`const CACHE = 'mctrl-shell-[a-f0-9]{16}'`).MatchString(serviceWorker.Body.String()) {
		t.Fatalf("service worker cache was not finalized")
	}
	if cache := serviceWorker.Header().Get("Cache-Control"); cache != "no-cache" {
		t.Fatalf("service worker cache policy = %q", cache)
	}

	manifest := httptest.NewRecorder()
	handler.ServeHTTP(manifest, httptest.NewRequest(http.MethodGet, "/manifest.webmanifest", nil))
	if manifest.Code != http.StatusOK {
		t.Fatalf("manifest status = %d", manifest.Code)
	}
	if contentType := manifest.Header().Get("Content-Type"); contentType != "application/manifest+json" {
		t.Fatalf("manifest content type = %q", contentType)
	}
	if !strings.Contains(manifest.Body.String(), "icon-maskable-512.png") || !strings.Contains(index.Body.String(), "apple-touch-icon.png") {
		t.Fatal("PWA manifest or Apple touch icon was not embedded")
	}
}

func TestEmbeddedHashedAssetsAreImmutable(t *testing.T) {
	cfg := config.Default()
	cfg.RemoteAvailability = config.AvailabilityWorkOnly
	server, err := NewServer(cfg, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	handler := server.Handler()

	index := httptest.NewRecorder()
	handler.ServeHTTP(index, httptest.NewRequest(http.MethodGet, "/", nil))
	match := regexp.MustCompile(`(?:src|href)="(/assets/[^"]+)"`).FindStringSubmatch(index.Body.String())
	if len(match) != 2 {
		t.Fatal("index did not reference a hashed asset")
	}
	asset := httptest.NewRecorder()
	handler.ServeHTTP(asset, httptest.NewRequest(http.MethodGet, match[1], nil))
	if asset.Code != http.StatusOK {
		t.Fatalf("asset status = %d", asset.Code)
	}
	if cache := asset.Header().Get("Cache-Control"); cache != "public, max-age=31536000, immutable" {
		t.Fatalf("asset cache policy = %q", cache)
	}
}
