package storage

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestWithLockDoesNotBreakLiveOwnerBasedOnAge(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "record.json")
	entered := make(chan struct{})
	release := make(chan struct{})
	completed := make(chan error, 1)

	go func() {
		completed <- WithLock(path, func() error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered

	lockPath := path + ".lock"
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(lockPath, old, old); err != nil {
		t.Fatal(err)
	}
	acquired := make(chan struct{})
	go func() {
		_ = WithLock(path, func() error {
			close(acquired)
			return nil
		})
	}()

	select {
	case <-acquired:
		t.Fatal("live lock was broken because of its age")
	case <-time.After(150 * time.Millisecond):
	}
	close(release)
	if err := <-completed; err != nil {
		t.Fatal(err)
	}
	select {
	case <-acquired:
	case <-time.After(time.Second):
		t.Fatal("lock was not acquired after release")
	}
}

func TestWithLockRecoversDeadOwner(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "record.json")
	lockPath := path + ".lock"
	if err := os.Mkdir(lockPath, 0700); err != nil {
		t.Fatal(err)
	}
	data := []byte(`{"pid":99999999,"token":"dead-owner","created":"2026-01-01T00:00:00Z"}`)
	if err := os.WriteFile(ownerPath(lockPath), data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := WithLock(path, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(lockPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("released lock still exists: %v", err)
	}
}

func TestLockReleaseDoesNotDeleteSuccessor(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "record.json")
	lockPath := path + ".lock"
	if err := os.Mkdir(lockPath, 0700); err != nil {
		t.Fatal(err)
	}
	data := []byte(`{"pid":` + strconv.Itoa(os.Getpid()) + `,"token":"successor","created":"2026-01-01T00:00:00Z"}`)
	if err := os.WriteFile(ownerPath(lockPath), data, 0600); err != nil {
		t.Fatal(err)
	}
	owner := lockOwner{PID: os.Getpid(), Token: "expected", Created: time.Now().UTC()}
	releaseLock(lockPath, owner)
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("non-owner release removed lock: %v", err)
	}
	_ = os.RemoveAll(lockPath)
}
