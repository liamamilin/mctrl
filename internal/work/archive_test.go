package work

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"mctrl/internal/tmux"
)

func lostWork(t *testing.T) (*Store, Work) {
	t.Helper()
	store := NewStore(t.TempDir())
	item := Work{ID: "work_lost", AttemptID: "attempt_lost", DeviceID: "device", RequestID: "same-request", State: StateRunning, ProjectID: "project", SessionName: "missing", RunnerPID: 9999999, ChildPID: 9999998, CreatedAt: time.Now().Add(-time.Minute), LaunchStage: StageChildStarted, PromptDelivery: PromptStarted}
	if err := store.Save(item); err != nil {
		t.Fatal(err)
	}
	return store, item
}

func TestArchiveMissingPreservesFactsAndIdempotency(t *testing.T) {
	store, item := lostWork(t)
	adapter := isolatedUnavailableTmux(t)
	got, err := ArchiveMissing(context.Background(), store, adapter, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ArchivedAt == nil || got.Active() || got.Terminal() || got.State != StateRunning || got.ExitCode != nil || got.FinishedAt != nil || got.LaunchStage != item.LaunchStage || got.PromptDelivery != PromptUnknown {
		t.Fatalf("archival changed exit facts or left Work active: %+v", got)
	}
	repeated, err := ArchiveMissing(context.Background(), store, adapter, item.ID)
	if err != nil || !repeated.ArchivedAt.Equal(*got.ArchivedAt) {
		t.Fatalf("archival is not idempotent: %+v %v", repeated, err)
	}
	retried, exists, err := store.SaveIdempotent(Work{ID: "work_duplicate", DeviceID: item.DeviceID, RequestID: item.RequestID})
	if err != nil || !exists || retried.ID != item.ID || retried.ArchivedAt == nil {
		t.Fatalf("archival lost the request mapping: %+v %v", retried, err)
	}
	now := time.Now().UTC()
	if err := store.WriteResult(Result{WorkID: item.ID, AttemptID: item.AttemptID, ExitCode: intPtr(7), ObservedAt: now, PersistedAt: now, FinishedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := Reconcile(context.Background(), store, adapter); err != nil {
		t.Fatal(err)
	}
	final, _ := store.Get(item.ID)
	if final.ArchivedAt == nil || final.State != StateExited || final.ExitCode == nil || *final.ExitCode != 7 {
		t.Fatalf("late real exit evidence was lost: %+v", final)
	}
}

func TestArchiveMissingRejectsLiveChildAndUncertainObservation(t *testing.T) {
	store, item := lostWork(t)
	adapter := isolatedUnavailableTmux(t)
	item.ChildPID = os.Getpid()
	if err := store.Save(item); err != nil {
		t.Fatal(err)
	}
	if _, err := ArchiveMissing(context.Background(), store, adapter, item.ID); !errors.Is(err, ErrArchiveUnsafe) {
		t.Fatalf("live child archival = %v", err)
	}
	item.ChildPID = 9999998
	if err := store.Save(item); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ArchiveMissing(ctx, store, adapter, item.ID); !errors.Is(err, ErrArchiveUnsafe) {
		t.Fatalf("uncertain tmux observation archival = %v", err)
	}
	got, _ := store.Get(item.ID)
	if got.ArchivedAt != nil {
		t.Fatal("refused archival changed the record")
	}
}

func TestArchiveMissingRejectsExistingSession(t *testing.T) {
	store, item := lostWork(t)
	dir, err := os.MkdirTemp("/private/tmp", "mctrl-archive-")
	if err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(dir, "s")
	adapter := tmux.NewWithSocket(socket)
	t.Cleanup(func() { _ = exec.Command("tmux", "-S", socket, "kill-server").Run(); _ = os.RemoveAll(dir) })
	if output, err := exec.Command("tmux", "-S", socket, "new-session", "-d", "-s", item.SessionName, "sleep 60").CombinedOutput(); err != nil {
		t.Fatalf("create isolated Session: %s %v", output, err)
	}
	if _, err := ArchiveMissing(context.Background(), store, adapter, item.ID); !errors.Is(err, ErrArchiveUnsafe) {
		t.Fatalf("existing Session archival = %v", err)
	}
}

func TestArchiveMissingRejectsExitEvidenceAndFreshLaunch(t *testing.T) {
	store, item := lostWork(t)
	adapter := isolatedUnavailableTmux(t)
	now := time.Now().UTC()
	if err := store.WriteResult(Result{WorkID: item.ID, AttemptID: item.AttemptID, ExitCode: intPtr(0), ObservedAt: now, PersistedAt: now, FinishedAt: now}); err != nil {
		t.Fatal(err)
	}
	if _, err := ArchiveMissing(context.Background(), store, adapter, item.ID); !errors.Is(err, ErrArchiveUnsafe) {
		t.Fatalf("unreconciled exit evidence archival = %v", err)
	}
	fresh := Work{ID: "work_fresh", State: StateAccepted, CreatedAt: now}
	if err := store.Save(fresh); err != nil {
		t.Fatal(err)
	}
	if _, err := ArchiveMissing(context.Background(), store, adapter, fresh.ID); !errors.Is(err, ErrArchiveUnsafe) {
		t.Fatalf("fresh launch archival = %v", err)
	}
}
