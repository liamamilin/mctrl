package storage

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// WriteJSONAtomic writes a complete JSON document and renames it into place.
func WriteJSONAtomic(path string, value interface{}, mode os.FileMode) error {
	if mode == 0 {
		mode = 0600
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".mctrl-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func ReadJSON(path string, value interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, value); err != nil {
		return fmt.Errorf("parse %s: %w", filepath.Base(path), err)
	}
	return nil
}

type lockOwner struct {
	PID     int       `json:"pid"`
	Token   string    `json:"token"`
	Created time.Time `json:"created"`
}

func ownerPath(lockPath string) string { return filepath.Join(lockPath, "owner.json") }

// WithLock provides a cross-process critical section for file-backed records.
// A live owner is never removed based only on lock age. The owner token also
// prevents a delayed release from deleting a successor's lock.
func WithLock(path string, fn func() error) error {
	lockPath := path + ".lock"
	owner, err := acquireLock(lockPath)
	if err != nil {
		return err
	}
	defer releaseLock(lockPath, owner)
	return fn()
}

func acquireLock(lockPath string) (lockOwner, error) {
	deadline := time.Now().Add(5 * time.Second)
	for {
		owner := lockOwner{PID: os.Getpid(), Token: randomLockToken(), Created: time.Now().UTC()}
		err := os.Mkdir(lockPath, 0700)
		if err == nil {
			data, marshalErr := json.Marshal(owner)
			if marshalErr != nil {
				_ = os.RemoveAll(lockPath)
				return lockOwner{}, marshalErr
			}
			if writeErr := os.WriteFile(ownerPath(lockPath), data, 0600); writeErr != nil {
				_ = os.RemoveAll(lockPath)
				return lockOwner{}, writeErr
			}
			return owner, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return lockOwner{}, err
		}

		existing, readErr := readLockOwner(lockPath)
		switch {
		case readErr == nil && processExists(existing.PID):
			// The owner is alive even if the critical section is slow. Never
			// break its lock solely because its mtime is old.
		case readErr == nil && !processExists(existing.PID):
			_ = os.RemoveAll(lockPath)
			continue
		default:
			// A process may have created the directory and not yet persisted its
			// owner. Give it a grace period before recovering an invalid lock.
			if info, statErr := os.Stat(lockPath); statErr == nil && time.Since(info.ModTime()) > 30*time.Second {
				_ = os.RemoveAll(lockPath)
				continue
			}
		}
		if time.Now().After(deadline) {
			return lockOwner{}, fmt.Errorf("timed out acquiring lock for %s", filepath.Base(pathWithoutLockSuffix(lockPath)))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func readLockOwner(lockPath string) (lockOwner, error) {
	data, err := os.ReadFile(ownerPath(lockPath))
	if err != nil {
		return lockOwner{}, err
	}
	var owner lockOwner
	if err := json.Unmarshal(data, &owner); err != nil {
		return lockOwner{}, err
	}
	if owner.PID <= 0 || strings.TrimSpace(owner.Token) == "" {
		return lockOwner{}, errors.New("invalid lock owner")
	}
	return owner, nil
}

func releaseLock(lockPath string, expected lockOwner) {
	owner, err := readLockOwner(lockPath)
	if err != nil || owner.PID != expected.PID || owner.Token != expected.Token {
		return
	}
	_ = os.RemoveAll(lockPath)
}

func processExists(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, os.ErrPermission)
}

func randomLockToken() string {
	data := make([]byte, 16)
	if _, err := rand.Read(data); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	return hex.EncodeToString(data)
}

func pathWithoutLockSuffix(lockPath string) string {
	return strings.TrimSuffix(lockPath, ".lock")
}
