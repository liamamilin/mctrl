package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/creack/pty"
	"golang.org/x/term"

	"mctrl/internal/config"
	"mctrl/internal/power"
	"mctrl/internal/runner"
	"mctrl/internal/version"
	"mctrl/internal/work"
)

func main() {
	workFile := flag.String("work-file", "", "path to the launch evidence JSON")
	attemptID := flag.String("attempt-id", "", "expected launch attempt id")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(version.String())
		return
	}
	if strings.TrimSpace(*workFile) == "" {
		fatal("missing --work-file")
	}
	if err := supervise(*workFile, strings.TrimSpace(*attemptID)); err != nil {
		// The failure is also persisted in the Work record. Returning a
		// non-zero status makes the tmux pane visibly useful while
		// remain-on-exit preserves the diagnostic Session.
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func makeStdinRaw() func() {
	state, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		// Unit tests and non-interactive launches may not have a TTY on stdin.
		return func() {}
	}
	return func() {
		_ = term.Restore(int(os.Stdin.Fd()), state)
	}
}

func supervise(workFile, expectedAttemptID string) error {
	restoreStdin := makeStdinRaw()
	defer restoreStdin()

	data, err := os.ReadFile(workFile)
	if err != nil {
		return fmt.Errorf("read launch evidence: %w", err)
	}
	var preliminary work.LaunchSpec
	if err := decodeJSON(data, &preliminary); err != nil {
		return fmt.Errorf("parse launch evidence: %w", err)
	}
	stateDir := strings.TrimSpace(preliminary.StateDir)
	if workParent := filepath.Dir(workFile); filepath.Base(workParent) == "work" {
		stateDir = filepath.Dir(workParent)
	}
	if stateDir == "" {
		stateDir, err = config.StateDir()
		if err != nil {
			return err
		}
	}
	store := work.NewStore(stateDir)
	spec, claimedPath, err := store.ClaimLaunchSpec(preliminary.WorkID)
	if err != nil {
		return fmt.Errorf("claim launch evidence: %w", err)
	}
	if expectedAttemptID != "" && spec.AttemptID != expectedAttemptID {
		return errors.New("launch evidence attempt does not match the runner command")
	}
	if !spec.StorePrompt {
		defer os.Remove(claimedPath)
	}
	item, err := store.Get(spec.WorkID)
	if err != nil {
		return fmt.Errorf("load Work: %w", err)
	}
	runnerPID := os.Getpid()
	if item.AttemptID == "" || item.AttemptID != spec.AttemptID ||
		item.ProjectID != spec.ProjectID || item.RunnerID != spec.RunnerID ||
		item.ProjectPath != spec.ProjectPath || item.KeepAwake != spec.KeepAwake {
		return errors.New("launch evidence does not match the current Work attempt")
	}
	if (item.State != work.StateAccepted && item.State != work.StateStarting) ||
		(item.RunnerPID != 0 && item.RunnerPID != runnerPID) || item.ChildPID != 0 {
		return fmt.Errorf("Work is not eligible for launch: state=%s runner_pid=%d child_pid=%d", item.State, item.RunnerPID, item.ChildPID)
	}
	logPath := filepath.Join(stateDir, "logs", "runner", spec.WorkID+".log")
	_ = os.MkdirAll(filepath.Dir(logPath), 0700)
	var runnerLog io.Writer = os.Stdout
	if logFile, logErr := openRunnerLog(logPath, 10<<20); logErr == nil {
		runnerLog = bestEffortOutput{primary: os.Stdout, secondary: logFile}
		defer logFile.Close()
	}
	item, err = store.UpdateIf(spec.WorkID, spec.AttemptID, []work.State{work.StateAccepted, work.StateStarting}, func(current *work.Work) error {
		if (current.RunnerPID != 0 && current.RunnerPID != runnerPID) || current.ChildPID != 0 {
			return work.ErrStateConflict
		}
		current.State = work.StateStarting
		current.RunnerPID = runnerPID
		current.LaunchStage = work.AdvanceLaunchStage(current.LaunchStage, work.StageRunnerStarted)
		return nil
	})
	if err != nil {
		return fmt.Errorf("record runner start: %w", err)
	}

	registry := runner.NewRegistry()
	adapter, err := registry.Get(spec.RunnerID)
	if err != nil {
		return failLaunch(store, spec.WorkID, spec.AttemptID, "RUNNER_NOT_FOUND", err.Error())
	}
	plan, err := adapter.BuildLaunch(runner.WorkSpec{
		WorkID:      spec.WorkID,
		ProjectID:   spec.ProjectID,
		ProjectPath: spec.ProjectPath,
		RunnerID:    spec.RunnerID,
		Prompt:      spec.Prompt,
	})
	if err != nil {
		code := "RUNNER_NOT_FOUND"
		if strings.Contains(strings.ToLower(err.Error()), "project path") {
			code = "PROJECT_PATH_MISSING"
		}
		return failLaunch(store, spec.WorkID, spec.AttemptID, code, err.Error())
	}

	var assertion *power.Assertion
	if spec.KeepAwake {
		assertion, err = power.StartWork(os.Getpid())
		if err != nil || !assertion.Active() {
			if err == nil {
				err = errors.New("caffeinate did not remain active")
			}
			return failLaunch(store, spec.WorkID, spec.AttemptID, "POWER_ASSERTION_FAILED", err.Error())
		}
	}
	var assertionReleasing atomic.Bool
	if assertion != nil {
		defer func() {
			assertionReleasing.Store(true)
			assertion.Release()
		}()
		go func() {
			<-assertion.Done()
			if assertionReleasing.Load() {
				return
			}
			assertionErr := assertion.Err()
			message := "caffeinate exited before the child"
			if assertionErr != nil {
				message += ": " + assertionErr.Error()
			}
			_, _ = store.UpdateIf(spec.WorkID, spec.AttemptID, []work.State{work.StateRunning}, func(current *work.Work) error {
				current.ErrorCode = "POWER_ASSERTION_LOST"
				current.ErrorMessage = message
				current.RecoveryStatus = "power_assertion_lost"
				return nil
			})
		}()
	}

	readyDir := ""
	readyFile := ""
	if plan.PromptStrategy == runner.PromptTerminal && strings.TrimSpace(spec.Prompt) != "" {
		readyDir, err = os.MkdirTemp("", "mctrl-ready-")
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: create Shell readiness channel: %v\n", err)
		} else {
			readyFile = filepath.Join(readyDir, "ready")
			defer os.RemoveAll(readyDir)
		}
	}
	command := exec.Command(plan.Executable, plan.Args...)
	command.Dir = plan.Cwd
	command.Env = append(os.Environ(), plan.Env...)
	if readyFile != "" {
		command.Env = append(command.Env, "MCTRL_READY_FILE="+readyFile)
	}
	initialSize := &pty.Winsize{Cols: 120, Rows: 40}
	if rows, cols, sizeErr := pty.Getsize(os.Stdin); sizeErr == nil && rows > 0 && cols > 0 {
		initialSize.Rows = uint16(rows)
		initialSize.Cols = uint16(cols)
	}
	ptmx, err := pty.StartWithSize(command, initialSize)
	if err != nil {
		return failLaunch(store, spec.WorkID, spec.AttemptID, "WORK_LAUNCH_FAILED", err.Error())
	}
	childWaited := false
	childPID := 0
	defer func() {
		if childWaited {
			return
		}
		_ = command.Process.Kill()
		waitErr := command.Wait()
		childWaited = true
		if childPID == 0 || item.AttemptID == "" {
			return
		}
		observedAt := time.Now().UTC()
		if journalErr := recordObservedExit(store, item, childPID, waitErr, command, observedAt, "runner_failure_after_child_start"); journalErr != nil {
			fmt.Fprintf(os.Stderr, "warning: persist child exit after runner failure: %v\n", journalErr)
		}
	}()
	defer ptmx.Close()
	resizeDone := make(chan struct{})
	defer close(resizeDone)
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		lastRows, lastCols := initialSize.Rows, initialSize.Cols
		for {
			select {
			case <-resizeDone:
				return
			case <-ticker.C:
				rows, cols, err := pty.Getsize(os.Stdin)
				if err != nil || rows <= 0 || cols <= 0 || (uint16(rows) == lastRows && uint16(cols) == lastCols) {
					continue
				}
				lastRows, lastCols = uint16(rows), uint16(cols)
				_ = pty.Setsize(ptmx, &pty.Winsize{Cols: lastCols, Rows: lastRows})
			}
		}
	}()

	// Drain the PTY continuously; otherwise a full terminal buffer can block
	// the child. For interactive Shell, relay the runner's tmux stdin too.
	outputDone := make(chan struct{})
	go func() {
		defer close(outputDone)
		_, _ = io.Copy(runnerLog, ptmx)
	}()
	// Relay the tmux pane's stdin for every runner. Structured argv runners
	// generally do not consume it, but forwarding it keeps the terminal
	// bridge truthful if an installed CLI enters an interactive mode.
	go func() {
		_, _ = io.Copy(ptmx, os.Stdin)
	}()

	childPID = command.Process.Pid
	observedStart := time.Now().UTC()
	item, err = store.UpdateIf(spec.WorkID, spec.AttemptID, []work.State{work.StateStarting}, func(current *work.Work) error {
		current.State = work.StateRunning
		current.ChildPID = childPID
		current.ChildExecutable = plan.Executable
		current.StartedAt = timePtr(observedStart)
		current.LaunchStage = work.AdvanceLaunchStage(current.LaunchStage, work.StageChildStarted)
		return nil
	})
	if err != nil {
		_ = command.Process.Kill()
		waitErr := command.Wait()
		childWaited = true
		observedAt := time.Now().UTC()
		journalErr := recordObservedExit(store, item, childPID, waitErr, command, observedAt, "runner_persistence_error_after_child_start")
		if journalErr != nil {
			return fmt.Errorf("record child start: %v; persist observed exit: %w", err, journalErr)
		}
		return fmt.Errorf("record child start: %w", err)
	}

	if plan.PromptStrategy == runner.PromptTerminal && strings.TrimSpace(spec.Prompt) != "" {
		if _, err := store.UpdateIf(spec.WorkID, spec.AttemptID, []work.State{work.StateRunning}, func(current *work.Work) error {
			current.PromptDelivery = work.PromptStarted
			current.LaunchStage = work.AdvanceLaunchStage(current.LaunchStage, work.StagePromptDeliveryStarted)
			return nil
		}); err != nil {
			return fmt.Errorf("record prompt start: %w", err)
		}
		// The Shell wrapper creates a private readiness file immediately
		// before exec'ing the user's interactive shell. Wait for that fact
		// instead of replaying a prompt after an arbitrary delay.
		ready := waitForFile(readyFile, 5*time.Second)
		var writeErr error
		deliveryUnknown := false
		if !ready {
			writeErr = errors.New("shell readiness was not confirmed")
		} else {
			promptBytes := []byte(spec.Prompt + "\r")
			written, writeError := ptmx.Write(promptBytes)
			switch {
			case writeError != nil && written > 0:
				writeErr = writeError
				deliveryUnknown = true
			case writeError != nil:
				writeErr = writeError
			case written != len(promptBytes):
				writeErr = io.ErrShortWrite
				deliveryUnknown = true
			}
		}
		if writeErr != nil {
			if _, updateErr := store.UpdateIf(spec.WorkID, spec.AttemptID, []work.State{work.StateRunning}, func(current *work.Work) error {
				if deliveryUnknown {
					current.PromptDelivery = work.PromptUnknown
					current.LaunchStage = work.AdvanceLaunchStage(current.LaunchStage, work.StagePromptDeliveryFailed)
				} else {
					current.PromptDelivery = work.PromptFailed
					current.LaunchStage = work.AdvanceLaunchStage(current.LaunchStage, work.StagePromptDeliveryFailed)
				}
				current.ErrorCode = "PROMPT_DELIVERY_FAILED"
				current.ErrorMessage = writeErr.Error()
				return nil
			}); updateErr != nil {
				return fmt.Errorf("record prompt failure: %w", updateErr)
			}
		} else {
			if _, updateErr := store.UpdateIf(spec.WorkID, spec.AttemptID, []work.State{work.StateRunning}, func(current *work.Work) error {
				current.PromptDelivery = work.PromptConfirmed
				current.LaunchStage = work.AdvanceLaunchStage(current.LaunchStage, work.StagePromptDeliveryConfirmed)
				return nil
			}); updateErr != nil {
				return fmt.Errorf("record prompt confirmation: %w", updateErr)
			}
		}
	} else if plan.PromptStrategy == runner.PromptArgv {
		if _, updateErr := store.UpdateIf(spec.WorkID, spec.AttemptID, []work.State{work.StateRunning}, func(current *work.Work) error {
			current.PromptDelivery = work.PromptConfirmed
			current.LaunchStage = work.AdvanceLaunchStage(current.LaunchStage, work.StagePromptDeliveryConfirmed)
			return nil
		}); updateErr != nil {
			return fmt.Errorf("record argv prompt confirmation: %w", updateErr)
		}
	}
	if strings.TrimSpace(spec.Prompt) == "" {
		if _, updateErr := store.UpdateIf(spec.WorkID, spec.AttemptID, []work.State{work.StateRunning}, func(current *work.Work) error {
			current.PromptDelivery = work.PromptNone
			return nil
		}); updateErr != nil {
			return fmt.Errorf("record empty prompt state: %w", updateErr)
		}
	}
	waitErr := command.Wait()
	observedAt := time.Now().UTC()
	childWaited = true
	closeOutput := func() {
		_ = ptmx.Close()
		select {
		case <-outputDone:
		case <-time.After(250 * time.Millisecond):
		}
	}
	closeOutput()
	if err := recordObservedExit(store, item, childPID, waitErr, command, observedAt, "child_exit_observed"); err != nil {
		return fmt.Errorf("persist child exit: %w", err)
	}
	return nil
}

func failLaunch(store *work.Store, id, attemptID, code, message string) error {
	if _, err := store.UpdateIf(id, attemptID, []work.State{work.StateAccepted, work.StateStarting}, func(current *work.Work) error {
		if current.ChildPID != 0 || current.State == work.StateRunning {
			return work.ErrStateConflict
		}
		current.State = work.StateLaunchFailed
		current.ErrorCode = code
		current.ErrorMessage = message
		current.FinishedAt = timePtr(time.Now().UTC())
		return nil
	}); err != nil {
		return fmt.Errorf("%s; persist launch failure: %w", message, err)
	}
	return errors.New(message)
}

type exitStore interface {
	WriteResult(work.Result) error
	UpdateIf(string, string, []work.State, func(*work.Work) error) (work.Work, error)
	Get(string) (work.Work, error)
}

type childExitObservation struct {
	ExitCode *int
	Signal   int
	Reason   string
}

func recordObservedExit(store exitStore, item work.Work, childPID int, waitErr error, command *exec.Cmd, observedAt time.Time, reason string) error {
	observation, err := observeChildExit(waitErr, command)
	if err != nil {
		return err
	}
	if reason == "" {
		reason = observation.Reason
	}
	persistedAt := time.Now().UTC()
	result := work.Result{
		WorkID:      item.ID,
		AttemptID:   item.AttemptID,
		RunnerPID:   item.RunnerPID,
		ChildPID:    childPID,
		ExitCode:    observation.ExitCode,
		ExitSignal:  observation.Signal,
		ObservedAt:  observedAt,
		PersistedAt: persistedAt,
		FinishedAt:  observedAt,
		Reason:      reason,
	}
	// The result journal is canonical. If the Work update fails or the process
	// crashes next, daemon reconciliation can rebuild the state from this file.
	if err := store.WriteResult(result); err != nil {
		return fmt.Errorf("write exit result: %w", err)
	}
	_, err = store.UpdateIf(item.ID, item.AttemptID, []work.State{work.StateStarting, work.StateRunning}, func(current *work.Work) error {
		current.State = work.StateExited
		current.ExitCode = observation.ExitCode
		current.ExitSignal = observation.Signal
		current.FinishedAt = timePtr(observedAt)
		current.LaunchStage = work.StageChildExited
		current.TerminationReason = reason
		current.RecoveryStatus = ""
		return nil
	})
	if err == nil {
		return nil
	}
	current, getErr := store.Get(item.ID)
	if getErr == nil && current.Terminal() && current.AttemptID == item.AttemptID &&
		((observation.ExitCode == nil && current.ExitCode == nil && current.ExitSignal == observation.Signal) ||
			(observation.ExitCode != nil && current.ExitCode != nil && *current.ExitCode == *observation.ExitCode)) {
		return nil
	}
	return err
}

func observeChildExit(waitErr error, command *exec.Cmd) (childExitObservation, error) {
	if command == nil || command.ProcessState == nil {
		if waitErr != nil {
			return childExitObservation{}, fmt.Errorf("child exit is unavailable: %w", waitErr)
		}
		return childExitObservation{}, errors.New("child exit is unavailable")
	}
	if status, ok := command.ProcessState.Sys().(syscall.WaitStatus); ok {
		if status.Signaled() {
			return childExitObservation{Signal: int(status.Signal()), Reason: fmt.Sprintf("terminated_by_signal_%d", status.Signal())}, nil
		}
		if status.Exited() {
			code := status.ExitStatus()
			return childExitObservation{ExitCode: &code, Reason: "child_exit_observed"}, nil
		}
	}
	code := command.ProcessState.ExitCode()
	if code < 0 {
		return childExitObservation{}, errors.New("child exit code is unknown")
	}
	return childExitObservation{ExitCode: &code, Reason: "child_exit_observed"}, nil
}

func openRunnerLog(path string, maxBytes int64) (*os.File, error) {
	if info, err := os.Stat(path); err == nil && info.Size() >= maxBytes {
		backup := path + ".1"
		_ = os.Remove(backup)
		if err := os.Rename(path, backup); err != nil {
			return nil, err
		}
	}
	return os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
}

type bestEffortOutput struct {
	primary   io.Writer
	secondary io.Writer
}

func (w bestEffortOutput) Write(data []byte) (int, error) {
	written, _ := w.primary.Write(data)
	if written > 0 && w.secondary != nil {
		_, _ = w.secondary.Write(data[:written])
	}
	// A diagnostic sink failure must never stop PTY draining and block child
	// output. Report a successful drain even when one sink rejected the bytes.
	return len(data), nil
}

func waitForFile(path string, timeout time.Duration) bool {
	if path == "" {
		return false
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	_, err := os.Stat(path)
	return err == nil
}

func timePtr(value time.Time) *time.Time { return &value }

func fatal(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(2)
}

// Kept local to avoid making the launch evidence format an API concern.
func decodeJSON(data []byte, destination interface{}) error {
	return json.Unmarshal(data, destination)
}
