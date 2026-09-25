package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"mctrl/internal/storage"
)

const (
	SessionCookieName = "mctrl_session"
	CSRFCookieName    = "mctrl_csrf"
	CSRFHeaderName    = "X-CSRF-Token"
)

var (
	ErrUnauthorized           = errors.New("unauthorized")
	ErrPairingClosed          = errors.New("pairing is closed")
	ErrPairTokenInvalid       = errors.New("pair token is invalid or expired")
	ErrDeviceLinkInvalid      = errors.New("device link is invalid or expired")
	ErrInvalidInstallationID  = errors.New("device installation id is invalid")
	ErrInstallationIDConflict = errors.New("device installation id is already bound")
)

const (
	lastSeenWriteInterval = time.Minute
	sessionLifetime       = 30 * 24 * time.Hour
)

type Device struct {
	ID                 string     `json:"id"`
	Name               string     `json:"name"`
	InstallationHash   string     `json:"installation_hash,omitempty"`
	InstallationHashes []string   `json:"installation_hashes,omitempty"`
	CredentialSalt     string     `json:"credential_salt"`
	CredentialHash     string     `json:"credential_hash"`
	CreatedAt          time.Time  `json:"created_at"`
	LastSeen           time.Time  `json:"last_seen"`
	RevokedAt          *time.Time `json:"revoked_at,omitempty"`
	Sessions           []Session  `json:"sessions,omitempty"`
}

type Session struct {
	ID               string    `json:"id"`
	InstallationHash string    `json:"installation_hash,omitempty"`
	TokenSalt        string    `json:"token_salt"`
	TokenHash        string    `json:"token_hash"`
	CSRFSalt         string    `json:"csrf_salt"`
	CSRFHash         string    `json:"csrf_hash"`
	CreatedAt        time.Time `json:"created_at"`
	LastSeen         time.Time `json:"last_seen"`
	ExpiresAt        time.Time `json:"expires_at"`
}

type registryFile struct {
	SchemaVersion      int                `json:"schema_version"`
	Devices            []Device           `json:"devices"`
	PendingDeviceLinks []deviceLinkRecord `json:"pending_device_links,omitempty"`
}

type Principal struct {
	Device    Device
	SessionID string
	ViaCookie bool
}

type PairResult struct {
	Device       Device
	AccessToken  string
	SessionToken string
	CSRFToken    string
	Reused       bool
}

type deviceLinkRecord struct {
	DeviceID  string    `json:"device_id"`
	TokenSalt string    `json:"token_salt"`
	TokenHash string    `json:"token_hash"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

type DeviceLinkTicket struct {
	Token     string
	ExpiresAt time.Time
}

type DeviceLinkResult struct {
	Device       Device
	SessionToken string
	CSRFToken    string
}

type Registry struct {
	path string
	mu   sync.Mutex
}

func NewRegistry(root string) *Registry {
	return &Registry{path: filepath.Join(root, "devices.json")}
}

func (r *Registry) load() (registryFile, error) {
	var value registryFile
	value.SchemaVersion = 1
	if err := storage.ReadJSON(r.path, &value); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return registryFile{SchemaVersion: 1, Devices: []Device{}}, nil
		}
		return registryFile{}, err
	}
	if value.Devices == nil {
		value.Devices = []Device{}
	}
	if value.SchemaVersion != 1 {
		return registryFile{}, fmt.Errorf("unsupported device schema version %d", value.SchemaVersion)
	}
	seen := make(map[string]struct{}, len(value.Devices))
	seenInstallations := make(map[string]struct{}, len(value.Devices))
	for _, device := range value.Devices {
		if strings.TrimSpace(device.ID) == "" || strings.TrimSpace(device.Name) == "" || device.CredentialHash == "" || device.CredentialSalt == "" {
			return registryFile{}, fmt.Errorf("device registry contains an incomplete record")
		}
		if _, exists := seen[device.ID]; exists {
			return registryFile{}, fmt.Errorf("device registry contains duplicate id %q", device.ID)
		}
		seen[device.ID] = struct{}{}
		for _, installationHash := range deviceInstallationHashes(device) {
			if _, exists := seenInstallations[installationHash]; exists {
				return registryFile{}, fmt.Errorf("device registry contains a duplicate installation identity")
			}
			seenInstallations[installationHash] = struct{}{}
		}
		for sessionIndex := range device.Sessions {
			session := &device.Sessions[sessionIndex]
			if strings.TrimSpace(session.ID) == "" || session.TokenHash == "" || session.TokenSalt == "" {
				return registryFile{}, fmt.Errorf("device %q contains an incomplete session", device.ID)
			}
			if session.InstallationHash != "" && !deviceHasInstallationHash(device, session.InstallationHash) {
				return registryFile{}, fmt.Errorf("device %q contains a session bound to an unknown installation", device.ID)
			}
			if session.ExpiresAt.IsZero() {
				session.ExpiresAt = session.CreatedAt.Add(sessionLifetime)
			}
		}
	}
	for _, link := range value.PendingDeviceLinks {
		if strings.TrimSpace(link.DeviceID) == "" || link.TokenHash == "" || link.TokenSalt == "" || link.ExpiresAt.IsZero() {
			return registryFile{}, fmt.Errorf("device registry contains an incomplete device link")
		}
	}
	return value, nil
}

func (r *Registry) save(value registryFile) error {
	value.SchemaVersion = 1
	return storage.WriteJSONAtomic(r.path, value, 0600)
}

// mutate serializes registry changes across daemon and CLI processes. The
// callback runs while holding both the in-process mutex and the file lock.
func (r *Registry) mutate(fn func(*registryFile) error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return storage.WithLock(r.path, func() error {
		value, err := r.load()
		if err != nil {
			return err
		}
		if err := fn(&value); err != nil {
			return err
		}
		return r.save(value)
	})
}

func hashInstallationID(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if len(raw) < 20 || len(raw) > 128 {
		return "", ErrInvalidInstallationID
	}
	for _, character := range raw {
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			character == '_' || character == '-' {
			continue
		}
		return "", ErrInvalidInstallationID
	}
	sum := sha256.Sum256([]byte(raw))
	return base64.RawURLEncoding.EncodeToString(sum[:]), nil
}

func deviceInstallationHashes(device Device) []string {
	values := make([]string, 0, 1+len(device.InstallationHashes))
	if device.InstallationHash != "" {
		values = append(values, device.InstallationHash)
	}
	values = append(values, device.InstallationHashes...)
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func deviceHasInstallationHash(device Device, installationHash string) bool {
	for _, existing := range deviceInstallationHashes(device) {
		if subtle.ConstantTimeCompare([]byte(existing), []byte(installationHash)) == 1 {
			return true
		}
	}
	return false
}

func addDeviceInstallationHash(device *Device, installationHash string) {
	if installationHash == "" || deviceHasInstallationHash(*device, installationHash) {
		return
	}
	if device.InstallationHash == "" {
		device.InstallationHash = installationHash
		return
	}
	device.InstallationHashes = append(device.InstallationHashes, installationHash)
}

// Pair consumes a short-lived CLI-created pairing token and creates both a
// device credential and a browser session credential. Raw credentials are
// returned only in this response and are never written to disk.
func (r *Registry) Pair(root, pairToken, deviceName string) (PairResult, error) {
	return r.PairForInstallation(root, pairToken, deviceName, "")
}

// PairForInstallation reuses the record bound to a browser installation. A new
// pairing rotates the device and browser credentials, revokes older sessions
// for that installation, and reactivates a previously revoked record.
func (r *Registry) PairForInstallation(root, pairToken, deviceName, installationID string) (PairResult, error) {
	installationHash, err := hashInstallationID(installationID)
	if err != nil {
		return PairResult{}, err
	}
	if err := ConsumePairing(root, pairToken); err != nil {
		return PairResult{}, err
	}
	deviceName = strings.TrimSpace(deviceName)
	if deviceName == "" {
		deviceName = "Paired device"
	}
	if len(deviceName) > 120 {
		deviceName = deviceName[:120]
	}
	accessToken, err := randomToken(32)
	if err != nil {
		return PairResult{}, err
	}
	sessionToken, err := randomToken(32)
	if err != nil {
		return PairResult{}, err
	}
	csrfToken, err := randomToken(24)
	if err != nil {
		return PairResult{}, err
	}
	credentialSalt, err := randomToken(16)
	if err != nil {
		return PairResult{}, err
	}
	credentialHash := hashSecret(credentialSalt, accessToken)
	now := time.Now().UTC()
	sessionID, err := randomToken(16)
	if err != nil {
		return PairResult{}, err
	}
	sessionSalt, err := randomToken(16)
	if err != nil {
		return PairResult{}, err
	}
	csrfSalt, err := randomToken(16)
	if err != nil {
		return PairResult{}, err
	}
	newSession := Session{
		ID:               "session_" + sessionID,
		InstallationHash: installationHash,
		TokenSalt:        sessionSalt,
		TokenHash:        hashSecret(sessionSalt, sessionToken),
		CSRFSalt:         csrfSalt,
		CSRFHash:         hashSecret(csrfSalt, csrfToken),
		CreatedAt:        now,
		LastSeen:         now,
		ExpiresAt:        now.Add(sessionLifetime),
	}

	var paired Device
	reused := false
	if err := r.mutate(func(value *registryFile) error {
		if installationHash != "" {
			for index := range value.Devices {
				if !deviceHasInstallationHash(value.Devices[index], installationHash) {
					continue
				}
				device := &value.Devices[index]
				device.Name = deviceName
				device.CredentialSalt = credentialSalt
				device.CredentialHash = credentialHash
				device.LastSeen = now
				device.RevokedAt = nil
				keptSessions := make([]Session, 0, len(device.Sessions)+1)
				for _, session := range device.Sessions {
					if session.InstallationHash != installationHash {
						keptSessions = append(keptSessions, session)
					}
				}
				device.Sessions = append(keptSessions, newSession)
				paired = cloneDevice(*device)
				reused = true
				return nil
			}
		}

		deviceID, err := randomToken(12)
		if err != nil {
			return err
		}
		device := Device{
			ID:               "device_" + deviceID,
			Name:             deviceName,
			InstallationHash: installationHash,
			CredentialSalt:   credentialSalt,
			CredentialHash:   credentialHash,
			CreatedAt:        now,
			LastSeen:         now,
			Sessions:         []Session{newSession},
		}
		value.Devices = append(value.Devices, device)
		paired = cloneDevice(device)
		return nil
	}); err != nil {
		return PairResult{}, err
	}
	return PairResult{
		Device:       paired,
		AccessToken:  accessToken,
		SessionToken: sessionToken,
		CSRFToken:    csrfToken,
		Reused:       reused,
	}, nil
}

func (r *Registry) CreateDeviceLink(principal Principal, ttl time.Duration) (DeviceLinkTicket, error) {
	if ttl <= 0 || ttl > 10*time.Minute {
		return DeviceLinkTicket{}, fmt.Errorf("device link TTL must be between zero and ten minutes")
	}
	rawToken, err := randomToken(32)
	if err != nil {
		return DeviceLinkTicket{}, err
	}
	tokenSalt, err := randomToken(16)
	if err != nil {
		return DeviceLinkTicket{}, err
	}
	var expiresAt time.Time
	if err := r.mutate(func(value *registryFile) error {
		now := time.Now().UTC()
		expiresAt = now.Add(ttl)
		record := deviceLinkRecord{
			DeviceID:  principal.Device.ID,
			TokenSalt: tokenSalt,
			TokenHash: hashSecret(tokenSalt, rawToken),
			CreatedAt: now,
			ExpiresAt: expiresAt,
		}
		active := false
		for _, device := range value.Devices {
			if device.ID == principal.Device.ID && device.RevokedAt == nil {
				active = true
				break
			}
		}
		if !active {
			return ErrUnauthorized
		}
		pending := make([]deviceLinkRecord, 0, len(value.PendingDeviceLinks)+1)
		for _, existing := range value.PendingDeviceLinks {
			if existing.ExpiresAt.After(now) && existing.DeviceID != principal.Device.ID {
				pending = append(pending, existing)
			}
		}
		value.PendingDeviceLinks = append(pending, record)
		return nil
	}); err != nil {
		return DeviceLinkTicket{}, err
	}
	return DeviceLinkTicket{Token: rawToken, ExpiresAt: expiresAt}, nil
}

func (r *Registry) LinkBrowser(rawToken, installationID string) (DeviceLinkResult, error) {
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return DeviceLinkResult{}, ErrDeviceLinkInvalid
	}
	installationHash, err := hashInstallationID(installationID)
	if err != nil {
		return DeviceLinkResult{}, err
	}
	if installationHash == "" {
		return DeviceLinkResult{}, ErrInvalidInstallationID
	}
	sessionToken, err := randomToken(32)
	if err != nil {
		return DeviceLinkResult{}, err
	}
	csrfToken, err := randomToken(24)
	if err != nil {
		return DeviceLinkResult{}, err
	}
	sessionSalt, err := randomToken(16)
	if err != nil {
		return DeviceLinkResult{}, err
	}
	csrfSalt, err := randomToken(16)
	if err != nil {
		return DeviceLinkResult{}, err
	}
	sessionID, err := randomToken(16)
	if err != nil {
		return DeviceLinkResult{}, err
	}
	var linked Device
	if err := r.mutate(func(value *registryFile) error {
		now := time.Now().UTC()
		newSession := Session{
			ID:               "session_" + sessionID,
			InstallationHash: installationHash,
			TokenSalt:        sessionSalt,
			TokenHash:        hashSecret(sessionSalt, sessionToken),
			CSRFSalt:         csrfSalt,
			CSRFHash:         hashSecret(csrfSalt, csrfToken),
			CreatedAt:        now,
			LastSeen:         now,
			ExpiresAt:        now.Add(sessionLifetime),
		}
		linkIndex := -1
		activeLinks := make([]deviceLinkRecord, 0, len(value.PendingDeviceLinks))
		for _, link := range value.PendingDeviceLinks {
			if link.ExpiresAt.After(now) && matchSecret(link.TokenSalt, link.TokenHash, rawToken) {
				linkIndex = len(activeLinks)
				activeLinks = append(activeLinks, link)
				continue
			}
			if link.ExpiresAt.After(now) {
				activeLinks = append(activeLinks, link)
			}
		}
		value.PendingDeviceLinks = activeLinks
		if linkIndex < 0 {
			return ErrDeviceLinkInvalid
		}
		link := value.PendingDeviceLinks[linkIndex]
		deviceIndex := -1
		for index := range value.Devices {
			device := &value.Devices[index]
			if device.ID == link.DeviceID {
				deviceIndex = index
				continue
			}
			if deviceHasInstallationHash(*device, installationHash) {
				return ErrInstallationIDConflict
			}
		}
		if deviceIndex < 0 || value.Devices[deviceIndex].RevokedAt != nil {
			return ErrUnauthorized
		}
		device := &value.Devices[deviceIndex]
		addDeviceInstallationHash(device, installationHash)
		device.LastSeen = now
		keptSessions := make([]Session, 0, len(device.Sessions)+1)
		for _, session := range device.Sessions {
			if session.InstallationHash != installationHash {
				keptSessions = append(keptSessions, session)
			}
		}
		device.Sessions = append(keptSessions, newSession)
		linked = cloneDevice(*device)
		value.PendingDeviceLinks = append(value.PendingDeviceLinks[:linkIndex], value.PendingDeviceLinks[linkIndex+1:]...)
		return nil
	}); err != nil {
		return DeviceLinkResult{}, err
	}
	return DeviceLinkResult{Device: linked, SessionToken: sessionToken, CSRFToken: csrfToken}, nil
}

func (r *Registry) Validate() error {
	_, err := r.load()
	return err
}

func (r *Registry) Authenticate(raw string) (Principal, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Principal{}, ErrUnauthorized
	}

	// Atomic registry replacement makes an unlocked read consistent. Avoid a
	// cross-process lock and disk rewrite on the common polling path while the
	// persisted LastSeen timestamp is still fresh.
	r.mu.Lock()
	value, loadErr := r.load()
	if loadErr != nil {
		r.mu.Unlock()
		return Principal{}, loadErr
	}
	result, matched, stale := authenticateValue(&value, raw, time.Now().UTC(), false)
	r.mu.Unlock()
	if matched && !stale {
		return result, nil
	}
	if !matched {
		return Principal{}, ErrUnauthorized
	}

	// Refresh LastSeen at most once per interval. Re-check the credential
	// under the cross-process mutation lock so CLI revocation remains decisive.
	var refreshed Principal
	err := r.mutate(func(current *registryFile) error {
		var matched bool
		refreshed, matched, _ = authenticateValue(current, raw, time.Now().UTC(), true)
		if !matched {
			return ErrUnauthorized
		}
		return nil
	})
	if err != nil {
		return Principal{}, err
	}
	return refreshed, nil
}

func authenticateValue(value *registryFile, raw string, now time.Time, touch bool) (Principal, bool, bool) {
	for deviceIndex := range value.Devices {
		device := &value.Devices[deviceIndex]
		if device.RevokedAt != nil {
			continue
		}
		stale := now.Sub(device.LastSeen) >= lastSeenWriteInterval
		if matchSecret(device.CredentialSalt, device.CredentialHash, raw) {
			if touch && stale {
				device.LastSeen = now
			}
			return Principal{Device: cloneDevice(*device)}, true, stale
		}
		for sessionIndex := range device.Sessions {
			session := &device.Sessions[sessionIndex]
			if !session.ExpiresAt.After(now) {
				continue
			}
			if matchSecret(session.TokenSalt, session.TokenHash, raw) {
				if touch && stale {
					device.LastSeen = now
					session.LastSeen = now
				}
				return Principal{Device: cloneDevice(*device), SessionID: session.ID}, true, stale
			}
		}
	}
	return Principal{}, false, false
}

func (r *Registry) CSRF(principal Principal) (string, error) {
	if principal.SessionID == "" {
		return "", nil
	}
	var result string
	err := r.mutate(func(value *registryFile) error {
		for deviceIndex := range value.Devices {
			device := &value.Devices[deviceIndex]
			if device.ID != principal.Device.ID || device.RevokedAt != nil {
				continue
			}
			for sessionIndex := range device.Sessions {
				session := &device.Sessions[sessionIndex]
				if session.ID != principal.SessionID {
					continue
				}
				if !session.ExpiresAt.After(time.Now().UTC()) {
					continue
				}
				raw, err := randomToken(24)
				if err != nil {
					return err
				}
				session.CSRFSalt, err = randomToken(16)
				if err != nil {
					return err
				}
				session.CSRFHash = hashSecret(session.CSRFSalt, raw)
				result = raw
				return nil
			}
		}
		return ErrUnauthorized
	})
	if err != nil {
		return "", err
	}
	return result, nil
}

func (r *Registry) CheckCSRF(principal Principal, raw string) bool {
	if principal.SessionID == "" {
		return true
	}
	if strings.TrimSpace(raw) == "" {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	value, err := r.load()
	if err != nil {
		return false
	}
	now := time.Now().UTC()
	for _, device := range value.Devices {
		if device.ID != principal.Device.ID || device.RevokedAt != nil {
			continue
		}
		for _, session := range device.Sessions {
			if session.ExpiresAt.After(now) && session.ID == principal.SessionID && matchSecret(session.CSRFSalt, session.CSRFHash, raw) {
				return true
			}
		}
	}
	return false
}

func (r *Registry) List() ([]Device, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	value, err := r.load()
	if err != nil {
		return nil, err
	}
	result := make([]Device, 0, len(value.Devices))
	for _, device := range value.Devices {
		result = append(result, cloneDevice(device))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.Before(result[j].CreatedAt) })
	return result, nil
}

func (r *Registry) ListActive() ([]Device, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	value, err := r.load()
	if err != nil {
		return nil, err
	}
	result := make([]Device, 0, len(value.Devices))
	for _, device := range value.Devices {
		if device.RevokedAt == nil {
			result = append(result, cloneDevice(device))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.Before(result[j].CreatedAt) })
	return result, nil
}

func (r *Registry) ListRevoked() ([]Device, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	value, err := r.load()
	if err != nil {
		return nil, err
	}
	result := make([]Device, 0, len(value.Devices))
	for _, device := range value.Devices {
		if device.RevokedAt != nil {
			result = append(result, cloneDevice(device))
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].RevokedAt.After(*result[j].RevokedAt)
	})
	return result, nil
}

func (r *Registry) BindInstallation(principal Principal, raw string) error {
	installationHash, err := hashInstallationID(raw)
	if err != nil {
		return err
	}
	if installationHash == "" {
		return ErrInvalidInstallationID
	}
	return r.mutate(func(value *registryFile) error {
		target := -1
		for index := range value.Devices {
			device := &value.Devices[index]
			if device.ID == principal.Device.ID {
				if device.RevokedAt != nil {
					return ErrUnauthorized
				}
				target = index
				continue
			}
			if deviceHasInstallationHash(*device, installationHash) {
				return ErrInstallationIDConflict
			}
		}
		if target < 0 {
			return ErrUnauthorized
		}
		device := &value.Devices[target]
		addDeviceInstallationHash(device, installationHash)
		if principal.SessionID != "" {
			for sessionIndex := range device.Sessions {
				if device.Sessions[sessionIndex].ID == principal.SessionID {
					device.Sessions[sessionIndex].InstallationHash = installationHash
					break
				}
			}
		}
		return nil
	})
}

func (r *Registry) IsActive(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	value, err := r.load()
	if err != nil {
		return false
	}
	for _, device := range value.Devices {
		if device.ID == id {
			return device.RevokedAt == nil
		}
	}
	return false
}

func (r *Registry) Revoke(id string) error {
	return r.mutate(func(value *registryFile) error {
		for index := range value.Devices {
			if value.Devices[index].ID != id {
				continue
			}
			now := time.Now().UTC()
			value.Devices[index].RevokedAt = &now
			value.Devices[index].Sessions = nil
			pending := make([]deviceLinkRecord, 0, len(value.PendingDeviceLinks))
			for _, link := range value.PendingDeviceLinks {
				if link.DeviceID != id {
					pending = append(pending, link)
				}
			}
			value.PendingDeviceLinks = pending
			return nil
		}
		return os.ErrNotExist
	})
}

func RequestCredentialWithSource(r *http.Request) (string, bool) {
	if value := strings.TrimSpace(r.Header.Get("Authorization")); value != "" {
		parts := strings.Fields(value)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			return parts[1], false
		}
		return "", false
	}
	if cookie, err := r.Cookie(SessionCookieName); err == nil {
		return cookie.Value, true
	}
	return "", false
}

func RequestCredential(r *http.Request) string {
	credential, _ := RequestCredentialWithSource(r)
	return credential
}

func SetSessionCookie(w http.ResponseWriter, token string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   60 * 60 * 24 * 30,
	})
}

func SetCSRFCookie(w http.ResponseWriter, token string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     CSRFCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: false,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   60 * 60 * 24 * 30,
	})
}

func ClearSessionCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func cloneDevice(device Device) Device {
	result := device
	result.InstallationHashes = append([]string(nil), device.InstallationHashes...)
	result.Sessions = append([]Session(nil), device.Sessions...)
	return result
}

func randomToken(size int) (string, error) {
	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		return "", fmt.Errorf("generate random credential: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func hashSecret(salt, raw string) string {
	sum := sha256.Sum256([]byte(salt + "\x00" + raw))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func matchSecret(salt, expected, raw string) bool {
	actual := hashSecret(salt, raw)
	return subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) == 1
}

type Pairing struct {
	Token     string    `json:"token"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

func pairingPath(root string) string { return filepath.Join(root, "pairing.json") }

func CreatePairing(root string, ttl time.Duration) (Pairing, error) {
	token, err := randomToken(32)
	if err != nil {
		return Pairing{}, err
	}
	now := time.Now().UTC()
	pairing := Pairing{Token: token, CreatedAt: now, ExpiresAt: now.Add(ttl)}
	if err := storage.WithLock(pairingPath(root), func() error {
		return storage.WriteJSONAtomic(pairingPath(root), pairing, 0600)
	}); err != nil {
		return Pairing{}, err
	}
	return pairing, nil
}

func ReadPairing(root string) (Pairing, error) {
	var pairing Pairing
	if err := storage.ReadJSON(pairingPath(root), &pairing); err != nil {
		return Pairing{}, err
	}
	if time.Now().UTC().After(pairing.ExpiresAt) {
		return Pairing{}, ErrPairTokenInvalid
	}
	return pairing, nil
}

func VerifyPairing(root, raw string) (Pairing, error) {
	pairing, err := ReadPairing(root)
	if err != nil {
		return Pairing{}, err
	}
	if subtle.ConstantTimeCompare([]byte(pairing.Token), []byte(strings.TrimSpace(raw))) != 1 {
		return Pairing{}, ErrPairTokenInvalid
	}
	return pairing, nil
}

func ConsumePairing(root, raw string) error {
	path := pairingPath(root)
	return storage.WithLock(path, func() error {
		var pairing Pairing
		if err := storage.ReadJSON(path, &pairing); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return ErrPairingClosed
			}
			return err
		}
		if time.Now().UTC().After(pairing.ExpiresAt) || subtle.ConstantTimeCompare([]byte(pairing.Token), []byte(raw)) != 1 {
			return ErrPairTokenInvalid
		}
		return os.Remove(path)
	})
}

// ValidateOriginForProfile permits an absent Origin for non-browser API
// clients but enforces the configured transport scheme when one is supplied.
func ValidateOriginForProfile(r *http.Request, allowed []string, secureProfile bool) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	normalized, ok := parseOrigin(origin)
	if !ok || secureProfile != strings.HasPrefix(normalized, "https://") {
		return false
	}
	return ValidateOrigin(r, allowed)
}

// ValidateBrowserOrigin is the strict browser policy used by pairing,
// cookie-authenticated mutations, and WebSocket upgrades.
func ValidateBrowserOrigin(r *http.Request, allowed []string, secureProfile bool) bool {
	if strings.TrimSpace(r.Header.Get("Origin")) == "" {
		return false
	}
	return ValidateOriginForProfile(r, allowed, secureProfile)
}

// ValidateOrigin accepts same-origin browser requests and explicitly listed
// origins. An absent Origin is allowed for CLI/API clients; WebSocket callers
// should use ValidateBrowserOrigin instead.
func ValidateOrigin(r *http.Request, allowed []string) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	if origin == "null" {
		return false
	}
	parsed, ok := parseOrigin(origin)
	if !ok {
		return false
	}
	for _, candidate := range allowed {
		if allowedOrigin, allowedOK := parseOrigin(strings.TrimRight(strings.TrimSpace(candidate), "/")); allowedOK && strings.EqualFold(allowedOrigin, origin) {
			return true
		}
	}
	host := strings.ToLower(strings.TrimSpace(r.Host))
	return host != "" && strings.EqualFold(parsedHost(parsed), host)
}

func parseOrigin(origin string) (string, bool) {
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(origin), "/"))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", false
	}
	return strings.ToLower(parsed.Scheme + "://" + parsed.Host), true
}

func parsedHost(origin string) string {
	parsed, err := url.Parse(origin)
	if err != nil {
		return ""
	}
	return parsed.Host
}
