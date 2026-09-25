package api

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"mctrl/internal/storage"
)

type promptRecord struct {
	RequestID   string    `json:"request_id"`
	DeviceID    string    `json:"device_id"`
	SessionID   string    `json:"session_id"`
	Fingerprint string    `json:"fingerprint,omitempty"`
	State       string    `json:"state"`
	CreatedAt   time.Time `json:"created_at"`
	FinishedAt  time.Time `json:"finished_at,omitempty"`
	Error       string    `json:"error,omitempty"`
}

type promptStore struct {
	root string
	mu   sync.Mutex
}

func newPromptStore(stateDir string) *promptStore {
	return &promptStore{root: filepath.Join(stateDir, "prompts")}
}

func (s *promptStore) path(deviceID, requestID string) string {
	key := sha256.Sum256([]byte(deviceID + "\x00" + requestID))
	return filepath.Join(s.root, hex.EncodeToString(key[:])+".json")
}

func (s *promptStore) get(deviceID, requestID string) (promptRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getUnlocked(deviceID, requestID)
}

func (s *promptStore) getUnlocked(deviceID, requestID string) (promptRecord, error) {
	var value promptRecord
	if err := storage.ReadJSON(s.path(deviceID, requestID), &value); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return promptRecord{}, os.ErrNotExist
		}
		return promptRecord{}, err
	}
	return value, nil
}

// claim persists the request before any terminal action. A repeated request
// observes the existing record and therefore never replays delivery.
func (s *promptStore) claim(value promptRecord) (promptRecord, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.root, 0700); err != nil {
		return promptRecord{}, false, err
	}
	var existing promptRecord
	path := s.path(value.DeviceID, value.RequestID)
	err := storage.WithLock(path, func() error {
		if readErr := storage.ReadJSON(path, &existing); readErr == nil {
			return nil
		} else if !errors.Is(readErr, os.ErrNotExist) {
			return readErr
		}
		return storage.WriteJSONAtomic(path, value, 0600)
	})
	if err != nil {
		return promptRecord{}, false, err
	}
	return existing, existing.RequestID != "", nil
}

func (s *promptStore) reconcile() error {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	var firstErr error
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(s.root, entry.Name())
		var value promptRecord
		if err := storage.ReadJSON(path, &value); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if value.State == "DELIVERY_STARTED" && time.Since(value.CreatedAt) > 2*time.Minute {
			value.State = "DELIVERY_UNKNOWN"
			value.Error = "delivery outcome was not confirmed before recovery"
			value.FinishedAt = time.Now().UTC()
			if err := storage.WriteJSONAtomic(path, value, 0600); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func (s *promptStore) save(value promptRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.root, 0700); err != nil {
		return err
	}
	return storage.WriteJSONAtomic(s.path(value.DeviceID, value.RequestID), value, 0600)
}
