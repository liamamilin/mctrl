package api

import (
	"path/filepath"
	"testing"
	"time"
)

func TestPromptRecoveryMarksUncertainDelivery(t *testing.T) {
	store := newPromptStore(t.TempDir())
	value := promptRecord{RequestID: "prompt-1", DeviceID: "device-1", SessionID: "$1", State: "DELIVERY_STARTED", CreatedAt: time.Now().Add(-3 * time.Minute)}
	if err := store.save(value); err != nil {
		t.Fatal(err)
	}
	if err := store.reconcile(); err != nil {
		t.Fatal(err)
	}
	got, err := store.get(value.DeviceID, value.RequestID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != "DELIVERY_UNKNOWN" {
		t.Fatalf("reconciled prompt state = %q", got.State)
	}
	if filepath.Base(store.path(value.DeviceID, value.RequestID)) == "" {
		t.Fatal("prompt path unexpectedly empty")
	}
}
