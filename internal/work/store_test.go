package work

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestWorkDTODoesNotExposeLaunchSecrets(t *testing.T) {
	item := Work{ID: "work_dto", RequestID: "request", RequestFingerprint: "secret-fingerprint", DeviceID: "device", ProjectPath: "/private/project", Prompt: "secret prompt", State: StateAccepted}
	data, err := json.Marshal(item.DTO())
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, forbidden := range []string{"secret prompt", "secret-fingerprint", "/private/project", "device"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("DTO exposed %q: %s", forbidden, text)
		}
	}
}

func TestStorePersistsIdempotencyAndUpdates(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	item := Work{ID: "work_test", RequestID: "request-1", DeviceID: "device-1", ProjectID: "p", RunnerID: "shell", State: StateAccepted, CreatedAt: time.Now().UTC(), LaunchStage: StageAccepted, PromptDelivery: PromptPending}
	if err := store.Save(item); err != nil {
		t.Fatal(err)
	}
	got, err := store.FindByRequest("device-1", "request-1")
	if err != nil || got.ID != item.ID {
		t.Fatalf("find request = %+v, %v", got, err)
	}
	updated, err := store.Update(item.ID, func(current *Work) error {
		current.State = StateRunning
		current.ChildPID = 42
		return nil
	})
	if err != nil || updated.ChildPID != 42 {
		t.Fatalf("update = %+v, %v", updated, err)
	}
	if err := store.WriteLaunchSpec(LaunchSpec{WorkID: item.ID, AttemptID: "attempt_test", ProjectPath: root, Prompt: "hello"}); err != nil {
		t.Fatal(err)
	}
	spec, err := store.ReadLaunchSpec(item.ID)
	if err != nil || spec.Prompt != "hello" {
		t.Fatalf("launch spec = %+v, %v", spec, err)
	}
	claimed, claimedPath, err := store.ClaimLaunchSpec(item.ID)
	if err != nil || claimed.AttemptID != spec.AttemptID || claimedPath == "" {
		t.Fatalf("claim launch spec = %+v %q %v", claimed, claimedPath, err)
	}
	if _, err := store.ReadLaunchSpec(item.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("claimed launch spec remained reusable: %v", err)
	}
	if _, _, err := store.ClaimLaunchSpec(item.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second runner claimed the same launch spec: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "work", item.ID+".json")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get("../devices"); err == nil {
		t.Fatal("path traversal was accepted by Work store")
	}
	now := time.Now().UTC()
	if err := store.WriteResult(Result{WorkID: item.ID, AttemptID: "attempt_test", ExitCode: intPtr(0), ObservedAt: now, PersistedAt: now, FinishedAt: now}); err != nil {
		t.Fatal(err)
	}
	items, err := store.List()
	if err != nil || len(items) != 1 || items[0].ID != item.ID {
		t.Fatalf("result file leaked into Work list: %+v, %v", items, err)
	}
}

func TestSaveIdempotentIsAtomic(t *testing.T) {
	store := NewStore(t.TempDir())
	items := []Work{
		{ID: "work_a", DeviceID: "device", RequestID: "same", State: StateAccepted, CreatedAt: time.Now()},
		{ID: "work_b", DeviceID: "device", RequestID: "same", State: StateAccepted, CreatedAt: time.Now()},
	}
	results := make([]Work, 2)
	exists := make([]bool, 2)
	var wg sync.WaitGroup
	for index := range items {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			results[index], exists[index], _ = store.SaveIdempotent(items[index])
		}(index)
	}
	wg.Wait()
	if results[0].ID != results[1].ID || exists[0] == exists[1] {
		t.Fatalf("idempotent results = %+v / %+v, exists=%v", results[0], results[1], exists)
	}
}

func TestReconcileLeavesAcceptedWorkRecoverable(t *testing.T) {
	store := NewStore(t.TempDir())
	item := Work{ID: "work_accepted", RequestID: "request-accepted", DeviceID: "device", ProjectID: "p", RunnerID: "shell", SessionName: "mctrl-does-not-exist", State: StateAccepted, CreatedAt: time.Now().Add(-time.Minute), LaunchStage: StageAccepted, PromptDelivery: PromptPending}
	if err := store.Save(item); err != nil {
		t.Fatal(err)
	}
	if err := Reconcile(context.Background(), store, isolatedUnavailableTmux(t)); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != StateAccepted || got.RecoveryStatus == "" {
		t.Fatalf("accepted Work was not left recoverable: %+v", got)
	}
}

func TestReconcileDoesNotInventExitCode(t *testing.T) {
	store := NewStore(t.TempDir())
	item := Work{ID: "work_missing", RequestID: "request-missing", DeviceID: "device", ProjectID: "p", RunnerID: "shell", SessionName: "mctrl-does-not-exist", State: StateRunning, CreatedAt: time.Now().Add(-time.Minute), RunnerPID: 999999, LaunchStage: StageChildStarted, PromptDelivery: PromptConfirmed}
	if err := store.Save(item); err != nil {
		t.Fatal(err)
	}
	if err := Reconcile(context.Background(), store, isolatedUnavailableTmux(t)); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != StateRunning || got.ExitCode != nil || got.RecoveryStatus == "" || got.FinishedAt != nil {
		t.Fatalf("recovery invented or lost facts: %+v", got)
	}
}

func TestReconcileAppliesCanonicalAttemptResult(t *testing.T) {
	store := NewStore(t.TempDir())
	item := Work{ID: "work_result", AttemptID: "attempt_result", RequestID: "request", State: StateRunning, CreatedAt: time.Now().Add(-time.Minute), RunnerPID: 999999, LaunchStage: StageChildStarted, PromptDelivery: PromptConfirmed}
	if err := store.Save(item); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := store.WriteResult(Result{WorkID: item.ID, AttemptID: item.AttemptID, ExitCode: intPtr(7), ObservedAt: now, PersistedAt: now, FinishedAt: now, Reason: "child_exit_observed"}); err != nil {
		t.Fatal(err)
	}
	if err := Reconcile(context.Background(), store, isolatedUnavailableTmux(t)); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != StateExited || got.ExitCode == nil || *got.ExitCode != 7 || got.PromptDelivery != PromptConfirmed {
		t.Fatalf("canonical result was not reconciled: %+v", got)
	}
}

func TestReconcileMarksMissingTerminalResultWithoutInventingExit(t *testing.T) {
	store := NewStore(t.TempDir())
	finished := time.Now().UTC()
	item := Work{ID: "work_missing_result", AttemptID: "attempt_missing_result", RequestID: "request", State: StateExited, CreatedAt: finished, ExitCode: intPtr(0), FinishedAt: &finished, LaunchStage: StageChildExited}
	if err := store.Save(item); err != nil {
		t.Fatal(err)
	}
	if err := Reconcile(context.Background(), store, isolatedUnavailableTmux(t)); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != StateExited || got.ExitCode == nil || *got.ExitCode != 0 || got.RecoveryStatus != "result_missing_after_recovery" {
		t.Fatalf("missing terminal result was not surfaced: %+v", got)
	}
}

func TestReconcileRejectsResultFromAnotherAttempt(t *testing.T) {
	store := NewStore(t.TempDir())
	item := Work{ID: "work_stale_result", AttemptID: "attempt_current", RequestID: "request", State: StateRunning, CreatedAt: time.Now().Add(-time.Minute), SessionName: "mctrl-does-not-exist", RunnerPID: 999999, LaunchStage: StageChildStarted, PromptDelivery: PromptConfirmed}
	if err := store.Save(item); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := store.WriteResult(Result{WorkID: item.ID, AttemptID: "attempt_old", ExitCode: intPtr(0), ObservedAt: now, PersistedAt: now, FinishedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := Reconcile(context.Background(), store, isolatedUnavailableTmux(t)); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != StateRunning || got.ExitCode != nil || !strings.Contains(got.RecoveryStatus, "result_attempt_mismatch") || got.PromptDelivery != PromptConfirmed {
		t.Fatalf("stale result changed factual state: %+v", got)
	}
}

func TestConditionalUpdateCannotRegressTerminalWork(t *testing.T) {
	store := NewStore(t.TempDir())
	finished := time.Now().UTC()
	item := Work{ID: "work_terminal", AttemptID: "attempt_terminal", RequestID: "request", State: StateExited, CreatedAt: finished, ExitCode: intPtr(3), FinishedAt: &finished, LaunchStage: StageChildExited}
	if err := store.Save(item); err != nil {
		t.Fatal(err)
	}
	_, err := store.UpdateIf(item.ID, item.AttemptID, []State{StateStarting}, func(current *Work) error {
		current.State = StateStarting
		return nil
	})
	if !errors.Is(err, ErrStateConflict) {
		t.Fatalf("terminal transition error = %v", err)
	}
	got, err := store.Get(item.ID)
	if err != nil || got.State != StateExited || got.ExitCode == nil || *got.ExitCode != 3 {
		t.Fatalf("terminal Work regressed: %+v, %v", got, err)
	}
}

func TestProcessIdentityRejectsReusedPID(t *testing.T) {
	alive, certain, err := inspectRunnerProcess(os.Getpid(), "work_missing", "attempt_missing")
	if err != nil || !certain || alive {
		t.Fatalf("unrelated current process was accepted as runner: alive=%v certain=%v err=%v", alive, certain, err)
	}
}

func intPtr(value int) *int { return &value }
