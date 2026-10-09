package api

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	qrcode "github.com/skip2/go-qrcode"
	"mctrl/internal/auth"
	"mctrl/internal/config"
)

func TestPairQRAdvertisesPreferredOriginAndOptionalFallback(t *testing.T) {
	for _, mode := range []string{"lan", "loopback", "tls"} {
		t.Run(mode, func(t *testing.T) {
			cfg := config.Default()
			cfg.RemoteAvailability = config.AvailabilityWorkOnly
			if mode == "lan" {
				cfg.ListenAddress = "0.0.0.0"
			}
			if mode == "tls" {
				cfg.TransportProfile, cfg.PublicURL = config.TransportTLSTerminated, "https://secure.example.test"
			}
			server, err := NewServer(cfg, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer server.Close()
			pairing, err := auth.CreatePairing(server.StateDir(), time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			console := "http://127.0.0.1:7681"
			if mode == "tls" {
				console = cfg.PublicURL
			}
			request := httptest.NewRequest(http.MethodPost, console+"/api/v1/pair/qr", strings.NewReader(`{"token":"`+pairing.Token+`"}`))
			request.RemoteAddr = "127.0.0.1:54321"
			request.Header.Set("Origin", console)
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			server.Handler().ServeHTTP(recorder, request)
			if recorder.Code != http.StatusOK {
				t.Fatalf("QR status = %d %s", recorder.Code, recorder.Body.String())
			}
			var payload map[string]string
			if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			want := config.AdvertisedPairAddresses(cfg)
			if payload["pair_url"] != want.Primary || payload["fallback_pair_url"] != want.Fallback {
				t.Fatalf("QR chose different addresses: %+v", payload)
			}
			if mode != "lan" {
				if _, exists := payload["fallback_pair_url"]; exists {
					t.Fatal("explicit/loopback origin gained a LAN fallback")
				}
			}
			png, err := qrcode.Encode(want.Primary+"#token="+url.QueryEscape(pairing.Token), qrcode.Medium, 512)
			if err != nil {
				t.Fatal(err)
			}
			if payload["data_url"] != "data:image/png;base64,"+base64.StdEncoding.EncodeToString(png) {
				t.Fatal("QR did not encode the preferred phone address and one-time token")
			}
			if _, err := auth.VerifyPairing(server.StateDir(), pairing.Token); err != nil {
				t.Fatalf("reading the QR consumed its token: %v", err)
			}

			// The temporary address uses the same pairing window, not a second
			// credential or relaxed origin check. One successful pairing consumes it.
			pairAt := want.Primary
			if want.Fallback != "" {
				pairAt = want.Fallback
			}
			origin := strings.TrimSuffix(pairAt, "/pair")
			pairRequest := httptest.NewRequest(http.MethodPost, origin+"/api/v1/pair", strings.NewReader(`{"token":"`+pairing.Token+`","device_name":"Phone"}`))
			pairRequest.Header.Set("Origin", origin)
			pairRequest.Header.Set("Content-Type", "application/json")
			paired := httptest.NewRecorder()
			server.Handler().ServeHTTP(paired, pairRequest)
			if paired.Code != http.StatusCreated {
				t.Fatalf("manual pair status = %d %s", paired.Code, paired.Body.String())
			}
			if _, err := auth.VerifyPairing(server.StateDir(), pairing.Token); err == nil {
				t.Fatal("successful pairing left the shared token reusable")
			}
		})
	}
}
