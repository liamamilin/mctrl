package work

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"mctrl/internal/tmux"
)

// Reconcile uses independent tmux and process evidence. It never fabricates an
// exit code or turns an infrastructure observation into a normal Work exit.
func Reconcile(ctx context.Context, store *Store, adapter *tmux.Adapter) error {
	items, err := store.List()
	if err != nil {
		return err
	}
	var firstErr error
	recordError := func(err error) {
		if firstErr == nil && err != nil {
			firstErr = err
		}
	}
	for _, item := range items {
		switch item.State {
		case StateExited:
			reconcileExitedResult(store, item, recordError)
			continue
		case StateAccepted, StateStarting, StateRunning:
		default:
			continue
		}

		resultStatus := ""
		result, resultErr := store.ReadResult(item.ID)
		if resultErr == nil {
			if item.AttemptID == "" || result.AttemptID != item.AttemptID {
				resultStatus = "result_attempt_mismatch"
			} else {
				_, updateErr := store.UpdateIf(item.ID, item.AttemptID, []State{StateAccepted, StateStarting, StateRunning}, func(current *Work) error {
					current.State = StateExited
					current.ExitCode = result.ExitCode
					current.ExitSignal = result.ExitSignal
					finished := result.ObservedAt
					current.FinishedAt = &finished
					current.LaunchStage = StageChildExited
					current.TerminationReason = result.Reason
					current.RecoveryStatus = ""
					return nil
				})
				recordError(updateErr)
				if updateErr == nil {
					continue
				}
			}
		} else if !errors.Is(resultErr, ErrNotFound) {
			resultStatus = "result_invalid"
		}

		if resultStatus != "" {
			_, updateErr := store.UpdateIf(item.ID, item.AttemptID, activeStates(), func(current *Work) error {
				current.RecoveryStatus = resultStatus
				return nil
			})
			recordError(updateErr)
		}

		// Only a completely untouched ACCEPTED record gets the short scheduling
		// grace period. A created Session or advanced launch stage is evidence
		// that launch actually began and must be reconciled immediately.
		if item.RunnerPID == 0 && item.State == StateAccepted && item.LaunchStage == StageAccepted && time.Since(item.CreatedAt) < 30*time.Second {
			continue
		}

		sessionExists := false
		var sessionErr error
		runnerAlive := false
		runnerCertain := false
		var runnerInspectErr error
		if item.SessionID != "" || item.SessionName != "" {
			target := item.SessionID
			if target == "" {
				target = item.SessionName
			}
			session, inspectErr := adapter.InspectSession(ctx, target)
			switch {
			case errors.Is(inspectErr, tmux.ErrSessionNotFound):
				sessionExists = false
			case inspectErr != nil:
				sessionErr = inspectErr
			default:
				sessionExists = item.SessionName == "" || session.Name == item.SessionName
				if item.RunnerPID == 0 && session.ActivePane != nil && !session.ActivePane.Dead &&
					runnerCommandMatchesAttempt(session.ActivePane.StartCommand, item.ID, item.AttemptID) {
					runnerAlive = true
					runnerCertain = true
					if session.ActivePane.PID > 0 {
						_, updateErr := store.UpdateIf(item.ID, item.AttemptID, activeStates(), func(current *Work) error {
							if current.RunnerPID == 0 {
								current.RunnerPID = session.ActivePane.PID
							}
							return nil
						})
						recordError(updateErr)
					}
				}
			}
		}
		if !runnerCertain {
			runnerAlive, runnerCertain, runnerInspectErr = inspectRunnerProcess(item.RunnerPID, item.ID, item.AttemptID)
		}
		if !runnerCertain {
			status := joinRecoveryStatus(resultStatus, "runner_identity_unknown")
			_, updateErr := store.UpdateIf(item.ID, item.AttemptID, activeStates(), func(current *Work) error {
				current.RecoveryStatus = status
				if current.TerminationReason == "" {
					current.TerminationReason = runnerInspectErr.Error()
				}
				return nil
			})
			recordError(updateErr)
			continue
		}
		if runnerAlive {
			continue
		}

		childAlive, childCertain, childInspectErr := inspectChildProcess(item.ChildPID, item.ChildExecutable)
		status := "runner_missing_after_recovery"
		reason := status
		switch {
		case runnerInspectErr != nil:
			status = joinRecoveryStatus(status, "runner_identity_unknown")
			reason = runnerInspectErr.Error()
		case sessionErr != nil:
			status = "tmux_observation_failed"
			reason = fmt.Sprintf("tmux observation failed: %v", sessionErr)
		case !sessionExists:
			status = "session_missing_after_recovery"
			reason = status
		case !childCertain:
			status = joinRecoveryStatus(status, "child_identity_unknown")
			reason = childInspectErr.Error()
		case childAlive:
			status = "child_without_runner"
			reason = status
		case childInspectErr != nil:
			status = joinRecoveryStatus(status, "child_identity_unknown")
			reason = childInspectErr.Error()
		}
		if childAlive {
			status += ";child_alive"
		}
		status = joinRecoveryStatus(resultStatus, status)
		_, updateErr := store.UpdateIf(item.ID, item.AttemptID, activeStates(), func(current *Work) error {
			current.RecoveryStatus = status
			if current.TerminationReason == "" {
				current.TerminationReason = reason
			}
			if current.PromptDelivery == PromptPending || current.PromptDelivery == PromptStarted {
				current.PromptDelivery = PromptUnknown
			}
			return nil
		})
		recordError(updateErr)
	}
	return firstErr
}

func reconcileExitedResult(store *Store, item Work, recordError func(error)) {
	result, err := store.ReadResult(item.ID)
	status := ""
	if errors.Is(err, ErrNotFound) {
		status = "result_missing_after_recovery"
	} else if err != nil {
		status = "result_invalid"
	} else if item.AttemptID == "" || result.AttemptID != item.AttemptID {
		status = "result_attempt_mismatch"
	} else if !sameExitFacts(item.ExitCode, item.ExitSignal, result) ||
		(item.FinishedAt != nil && !item.FinishedAt.Equal(result.ObservedAt)) {
		status = "result_fact_mismatch"
	}
	_, updateErr := store.UpdateIf(item.ID, item.AttemptID, nil, func(current *Work) error {
		if status == "" {
			current.RecoveryStatus = ""
		} else {
			current.RecoveryStatus = status
		}
		return nil
	})
	recordError(updateErr)
}

func sameExitFacts(exitCode *int, exitSignal int, result Result) bool {
	if result.ExitCode == nil || exitCode == nil {
		return result.ExitCode == nil && exitCode == nil && exitSignal == result.ExitSignal
	}
	return *exitCode == *result.ExitCode && exitSignal == result.ExitSignal
}

func activeStates() []State { return []State{StateAccepted, StateStarting, StateRunning} }

func joinRecoveryStatus(existing, next string) string {
	if existing == "" {
		return next
	}
	if next == "" || strings.Contains(existing, next) {
		return existing
	}
	return existing + ";" + next
}

func runnerCommandMatchesAttempt(command, workID, attemptID string) bool {
	command = strings.ToLower(command)
	if !strings.Contains(command, strings.ToLower(workID+".launch.json")) {
		return false
	}
	return attemptID == "" || strings.Contains(command, strings.ToLower(attemptID))
}

func inspectRunnerProcess(pid int, workID, attemptID string) (alive bool, certain bool, err error) {
	if pid <= 0 {
		return false, true, nil
	}
	if err := syscall.Kill(pid, 0); err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ESRCH) {
			return false, true, nil
		}
		if !errors.Is(err, os.ErrPermission) && !errors.Is(err, syscall.EPERM) {
			return false, false, err
		}
	}
	state, command, err := processStateAndCommand(pid)
	if err != nil {
		return false, false, err
	}
	if strings.HasPrefix(state, "Z") {
		return false, true, nil
	}
	return runnerCommandMatchesAttempt(command, workID, attemptID), true, nil
}

func inspectChildProcess(pid int, executable string) (alive bool, certain bool, err error) {
	if pid <= 0 {
		return false, true, nil
	}
	if err := syscall.Kill(pid, 0); err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ESRCH) {
			return false, true, nil
		}
		if !errors.Is(err, os.ErrPermission) && !errors.Is(err, syscall.EPERM) {
			return false, false, err
		}
	}
	state, command, err := processStateAndCommand(pid)
	if err != nil {
		return false, false, err
	}
	if strings.HasPrefix(state, "Z") {
		return false, true, nil
	}
	if strings.TrimSpace(executable) == "" {
		return true, true, nil
	}
	return commandContainsExecutable(command, executable), true, nil
}

func processStateAndCommand(pid int) (string, string, error) {
	output, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "state=,command=").Output()
	if err != nil {
		return "", "", fmt.Errorf("inspect process %d: %w", pid, err)
	}
	fields := strings.Fields(string(output))
	if len(fields) == 0 {
		return "", "", fmt.Errorf("process %d returned no identity", pid)
	}
	return fields[0], strings.Join(fields[1:], " "), nil
}

func commandContainsExecutable(command, executable string) bool {
	command = strings.TrimSpace(command)
	executable = strings.TrimSpace(executable)
	if command == "" || executable == "" {
		return false
	}
	for _, field := range strings.Fields(command) {
		if field == executable || filepath.Base(field) == filepath.Base(executable) {
			return true
		}
	}
	return false
}
