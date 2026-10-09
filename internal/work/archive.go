package work

import (
	"context"
	"errors"
	"fmt"
	"time"

	"mctrl/internal/tmux"
)

var ErrArchiveUnsafe = errors.New("Work cannot be archived while its Session or processes exist, or their absence is uncertain")

// ArchiveMissing preserves unknown exit facts and the request-id mapping. The
// launch lock serializes this with retries; the record lock prevents a runner
// from registering itself between the evidence check and the archival write.
func ArchiveMissing(ctx context.Context, store *Store, adapter *tmux.Adapter, id string) (Work, error) {
	var archived Work
	err := store.WithLaunchLock(id, func() error {
		item, err := store.Get(id)
		if err != nil {
			return err
		}
		archived, err = store.UpdateIf(id, item.AttemptID, nil, func(current *Work) error {
			if ctx.Err() != nil {
				return ErrArchiveUnsafe
			}
			if current.ArchivedAt != nil {
				return nil
			}
			if current.Terminal() || (current.RunnerPID == 0 && current.ChildPID == 0 && time.Since(current.CreatedAt) < 30*time.Second) {
				return ErrArchiveUnsafe
			}
			// A pending launch claim could otherwise start after archival. The
			// runner also checks ArchivedAt while registering under this lock.
			for _, target := range []string{current.SessionID, current.SessionName} {
				if target == "" {
					continue
				}
				_, inspectErr := adapter.InspectSession(ctx, target)
				if !errors.Is(inspectErr, tmux.ErrSessionNotFound) && !errors.Is(inspectErr, tmux.ErrNoServer) {
					return ErrArchiveUnsafe
				}
			}
			alive, certain, inspectErr := inspectRunnerProcess(current.RunnerPID, current.ID, current.AttemptID)
			if alive || !certain || inspectErr != nil {
				return ErrArchiveUnsafe
			}
			alive, certain, inspectErr = inspectChildProcess(current.ChildPID, current.ChildExecutable)
			if alive || !certain || inspectErr != nil {
				return ErrArchiveUnsafe
			}
			if _, resultErr := store.ReadResult(id); !errors.Is(resultErr, ErrNotFound) {
				return fmt.Errorf("%w: exit evidence needs reconciliation", ErrArchiveUnsafe)
			}
			now := time.Now().UTC()
			current.ArchivedAt = &now
			current.RecoveryStatus = "session_missing_after_recovery"
			if current.PromptDelivery == PromptPending || current.PromptDelivery == PromptStarted {
				current.PromptDelivery = PromptUnknown
			}
			return nil
		})
		return err
	})
	return archived, err
}
