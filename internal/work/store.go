package work

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"mctrl/internal/storage"
)

var (
	ErrNotFound        = errors.New("work not found")
	ErrAttemptMismatch = errors.New("work attempt does not match")
	ErrStateConflict   = errors.New("work state transition is not allowed")
)

type Store struct {
	root string
	mu   sync.Mutex
}

func NewStore(root string) *Store {
	return &Store{root: filepath.Join(root, "work")}
}

func (s *Store) Ensure() error {
	return os.MkdirAll(s.root, 0700)
}

func (s *Store) path(id string) string {
	return filepath.Join(s.root, id+".json")
}

func (s *Store) launchPath(id string) string {
	return filepath.Join(s.root, id+".launch.json")
}

func (s *Store) Save(value Work) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveUnlocked(value)
}

func (s *Store) saveUnlocked(value Work) error {
	if err := s.Ensure(); err != nil {
		return err
	}
	if err := validateID(value.ID); err != nil {
		return err
	}
	return storage.WriteJSONAtomic(s.path(value.ID), value, 0600)
}

// SaveIdempotent atomically resolves a launch request across daemon processes
// and daemon restarts. The boolean reports that an existing Work was reused.
func (s *Store) SaveIdempotent(value Work) (Work, bool, error) {
	if err := validateID(value.ID); err != nil {
		return Work{}, false, err
	}
	if err := s.Ensure(); err != nil {
		return Work{}, false, err
	}
	key := sha256.Sum256([]byte(value.DeviceID + "\x00" + value.RequestID))
	lockPath := filepath.Join(s.root, ".request-"+hex.EncodeToString(key[:])+".lock")
	var result Work
	var existed bool
	err := storage.WithLock(lockPath, func() error {
		items, listErr := s.List()
		if listErr != nil {
			return listErr
		}
		for _, item := range items {
			if item.DeviceID == value.DeviceID && item.RequestID == value.RequestID {
				result = item
				existed = true
				return nil
			}
		}
		if saveErr := s.saveUnlocked(value); saveErr != nil {
			return saveErr
		}
		result = value
		return nil
	})
	return result, existed, err
}

func (s *Store) WithLaunchLock(id string, fn func() error) error {
	if err := validateID(id); err != nil {
		return err
	}
	if err := s.Ensure(); err != nil {
		return err
	}
	return storage.WithLock(filepath.Join(s.root, id+".launch.lock"), fn)
}

func (s *Store) Get(id string) (Work, error) {
	if err := validateID(id); err != nil {
		return Work{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getUnlocked(id)
}

func (s *Store) getUnlocked(id string) (Work, error) {
	var value Work
	if err := storage.ReadJSON(s.path(id), &value); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Work{}, ErrNotFound
		}
		return Work{}, err
	}
	return value, nil
}

// Update serializes updates inside this process and uses a cross-process lock
// for daemon/runner updates to the same Work file.
func (s *Store) Update(id string, fn func(*Work) error) (Work, error) {
	if err := validateID(id); err != nil {
		return Work{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.Ensure(); err != nil {
		return Work{}, err
	}
	var result Work
	err := storage.WithLock(s.path(id), func() error {
		current, err := s.getUnlocked(id)
		if err != nil {
			return err
		}
		if err := fn(&current); err != nil {
			return err
		}
		if err := s.saveUnlocked(current); err != nil {
			return err
		}
		result = current
		return nil
	})
	return result, err
}

// UpdateIf applies fn only when the persisted record still belongs to the
// expected launch attempt and is in one of the allowed states.
func (s *Store) UpdateIf(id, expectedAttemptID string, allowed []State, fn func(*Work) error) (Work, error) {
	if err := validateID(id); err != nil {
		return Work{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.Ensure(); err != nil {
		return Work{}, err
	}
	var result Work
	err := storage.WithLock(s.path(id), func() error {
		current, err := s.getUnlocked(id)
		if err != nil {
			return err
		}
		if expectedAttemptID != "" && current.AttemptID != expectedAttemptID {
			return ErrAttemptMismatch
		}
		if len(allowed) > 0 && !containsState(allowed, current.State) {
			return ErrStateConflict
		}
		if err := fn(&current); err != nil {
			return err
		}
		if err := s.saveUnlocked(current); err != nil {
			return err
		}
		result = current
		return nil
	})
	return result, err
}

func containsState(states []State, value State) bool {
	for _, state := range states {
		if state == value {
			return true
		}
	}
	return false
}

func (s *Store) List() ([]Work, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []Work{}, nil
		}
		return nil, err
	}
	items := make([]Work, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".json") || strings.Contains(name, ".launch.") || strings.HasSuffix(name, ".result.json") {
			continue
		}
		var value Work
		if err := storage.ReadJSON(filepath.Join(s.root, name), &value); err != nil {
			// A partially written/corrupt record must remain visible to the
			// caller rather than silently disappearing.
			items = append(items, Work{ID: strings.TrimSuffix(name, ".json"), State: StateLaunchFailed, ErrorCode: "INTERNAL_ERROR", ErrorMessage: err.Error()})
			continue
		}
		items = append(items, value)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
	return items, nil
}

func (s *Store) FindByRequest(deviceID, requestID string) (Work, error) {
	items, err := s.List()
	if err != nil {
		return Work{}, err
	}
	for _, item := range items {
		if item.DeviceID == deviceID && item.RequestID == requestID {
			return item, nil
		}
	}
	return Work{}, ErrNotFound
}

func (s *Store) WriteLaunchSpec(spec LaunchSpec) error {
	if err := validateID(spec.WorkID); err != nil {
		return err
	}
	if err := validateID(spec.AttemptID); err != nil {
		return errors.New("launch attempt id is required")
	}
	if err := s.Ensure(); err != nil {
		return err
	}
	path := s.launchPath(spec.WorkID)
	return storage.WithLock(path, func() error {
		return storage.WriteJSONAtomic(path, spec, 0600)
	})
}

// ClaimLaunchSpec atomically removes the daemon handoff path and returns the
// immutable path owned by this runner. A second runner therefore cannot consume
// the same evidence, and an old runner can only remove its own claimed file.
func (s *Store) ClaimLaunchSpec(id string) (LaunchSpec, string, error) {
	if err := validateID(id); err != nil {
		return LaunchSpec{}, "", err
	}
	if err := s.Ensure(); err != nil {
		return LaunchSpec{}, "", err
	}
	path := s.launchPath(id)
	var spec LaunchSpec
	claimedPath := ""
	err := storage.WithLock(path, func() error {
		if err := storage.ReadJSON(path, &spec); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return ErrNotFound
			}
			return err
		}
		if spec.WorkID != id || validateID(spec.AttemptID) != nil {
			return errors.New("launch evidence does not match its Work")
		}
		claimedPath = fmt.Sprintf("%s.%s.claimed.json", path, randomToken(6))
		return os.Rename(path, claimedPath)
	})
	return spec, claimedPath, err
}

func (s *Store) DeleteLaunchSpec(id string) error {
	if err := validateID(id); err != nil {
		return err
	}
	path := s.launchPath(id)
	return storage.WithLock(path, func() error {
		err := os.Remove(path)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	})
}

func (s *Store) ReadLaunchSpec(id string) (LaunchSpec, error) {
	if err := validateID(id); err != nil {
		return LaunchSpec{}, err
	}
	var spec LaunchSpec
	if err := storage.ReadJSON(s.launchPath(id), &spec); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return LaunchSpec{}, ErrNotFound
		}
		return LaunchSpec{}, err
	}
	return spec, nil
}

func (s *Store) ResultPath(id string) string {
	return filepath.Join(s.root, id+".result.json")
}

func (s *Store) WriteResult(result Result) error {
	if err := validateID(result.WorkID); err != nil {
		return err
	}
	if err := validateResult(result); err != nil {
		return err
	}
	return storage.WriteJSONAtomic(s.ResultPath(result.WorkID), result, 0600)
}

func (s *Store) ReadResult(id string) (Result, error) {
	if err := validateID(id); err != nil {
		return Result{}, err
	}
	var result Result
	if err := storage.ReadJSON(s.ResultPath(id), &result); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Result{}, ErrNotFound
		}
		return Result{}, err
	}
	if result.WorkID != id {
		return Result{}, errors.New("result Work id does not match its path")
	}
	if err := validateResult(result); err != nil {
		return Result{}, err
	}
	return result, nil
}

func validateResult(result Result) error {
	if err := validateID(result.AttemptID); err != nil {
		return errors.New("result attempt id is required")
	}
	if result.ExitCode == nil && result.ExitSignal == 0 {
		return errors.New("result exit code or signal is required")
	}
	if result.ObservedAt.IsZero() || result.PersistedAt.IsZero() || result.FinishedAt.IsZero() {
		return errors.New("result timestamps are required")
	}
	if result.PersistedAt.Before(result.ObservedAt) || !result.FinishedAt.Equal(result.ObservedAt) {
		return errors.New("result timestamps are inconsistent")
	}
	return nil
}

func validateID(id string) error {
	if strings.TrimSpace(id) == "" || strings.ContainsAny(id, `/\\`) || id == "." || id == ".." {
		return fmt.Errorf("invalid work id %q", id)
	}
	return nil
}

func randomToken(bytes int) string {
	data := make([]byte, bytes)
	if _, err := rand.Read(data); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return base64.RawURLEncoding.EncodeToString(data)
}
