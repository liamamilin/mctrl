package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"mctrl/internal/work"
)

func TestSuperviseRecordsSuccessfulAndFailedExit(t *testing.T) {
	for _, exitCode := range []int{0, 7} {
		t.Run("exit", func(t *testing.T) {
			stateDir, workItem, launchFile := prepareRunnerFixture(t, exitCode, work.StateAccepted)
			if err := supervise(launchFile, workItem.AttemptID); err != nil {
				t.Fatal(err)
			}
			store := work.NewStore(stateDir)
			got, err := store.Get(workItem.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.State != work.StateExited || got.ExitCode == nil || *got.ExitCode != exitCode || got.FinishedAt == nil {
				t.Fatalf("unexpected terminal Work: %+v", got)
			}
			result, err := store.ReadResult(workItem.ID)
			if err != nil {
				t.Fatal(err)
			}
			if result.AttemptID != workItem.AttemptID || result.ExitCode == nil || *result.ExitCode != exitCode || result.ObservedAt.IsZero() {
				t.Fatalf("unexpected canonical result: %+v", result)
			}
			if _, err := store.ReadLaunchSpec(workItem.ID); !errors.Is(err, work.ErrNotFound) {
				t.Fatalf("transient launch evidence remains: %v", err)
			}
		})
	}
}

func TestSuperviseRejectsStaleLaunchForTerminalWork(t *testing.T) {
	stateDir, workItem, launchFile := prepareRunnerFixture(t, 0, work.StateExited)
	marker := filepath.Join(t.TempDir(), "ran")
	shell := filepath.Join(filepath.Dir(marker), "shell")
	script := "#!/bin/sh\ntouch \"" + marker + "\"\nexit 0\n"
	if err := os.WriteFile(shell, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHELL", shell)
	if err := supervise(launchFile, workItem.AttemptID); err == nil {
		t.Fatal("terminal Work accepted stale launch evidence")
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale launch executed a child: %v", err)
	}
	if _, err := work.NewStore(stateDir).Get(workItem.ID); err != nil {
		t.Fatal(err)
	}
}

func TestRunnerLogRotatesBeforeAppend(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runner.log")
	if err := os.WriteFile(path, []byte("old output"), 0600); err != nil {
		t.Fatal(err)
	}
	file, err := openRunnerLog(path, 4)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	backup, err := os.ReadFile(path + ".1")
	if err != nil || string(backup) != "old output" {
		t.Fatalf("runner log backup = %q, %v", backup, err)
	}
}

func TestBestEffortOutputNeverStopsPTYDrain(t *testing.T) {
	sink := bestEffortOutput{primary: failingWriter{}}
	data := []byte("terminal output")
	written, err := sink.Write(data)
	if err != nil || written != len(data) {
		t.Fatalf("diagnostic sink failure escaped: written=%d err=%v", written, err)
	}
}

func TestRecordObservedExitDoesNotUpdateWorkWhenJournalFails(t *testing.T) {
	store := &recordingExitStore{writeErr: errors.New("disk full")}
	item := work.Work{ID: "work_exit", AttemptID: "attempt_exit", RunnerPID: 42}
	command := exitCommand(3)
	err := command.Run()
	if err == nil {
		t.Fatal("expected exit command error")
	}
	if recordErr := recordObservedExit(store, item, command.Process.Pid, err, command, time.Now().UTC(), "child_exit_observed"); recordErr == nil {
		t.Fatal("result journal failure was not returned")
	}
	if len(store.events) != 1 || store.events[0] != "result" {
		t.Fatalf("Work update ran without a durable result: %v", store.events)
	}
}

func TestRecordObservedExitWritesJournalBeforeWorkUpdate(t *testing.T) {
	store := &recordingExitStore{}
	item := work.Work{ID: "work_exit", AttemptID: "attempt_exit", RunnerPID: 42}
	command := exitCommand(3)
	err := command.Run()
	if err == nil {
		t.Fatal("expected exit command error")
	}
	now := time.Now().UTC()
	if err := recordObservedExit(store, item, command.Process.Pid, err, command, now, "child_exit_observed"); err != nil {
		t.Fatal(err)
	}
	if len(store.events) != 2 || store.events[0] != "result" || store.events[1] != "update" {
		t.Fatalf("exit persistence order = %v", store.events)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("sink failed") }

type recordingExitStore struct {
	events   []string
	writeErr error
}

func (s *recordingExitStore) WriteResult(work.Result) error {
	s.events = append(s.events, "result")
	return s.writeErr
}

func (s *recordingExitStore) UpdateIf(_ string, _ string, _ []work.State, fn func(*work.Work) error) (work.Work, error) {
	s.events = append(s.events, "update")
	return work.Work{}, fn(&work.Work{})
}

func (s *recordingExitStore) Get(string) (work.Work, error) { return work.Work{}, work.ErrNotFound }

func prepareRunnerFixture(t *testing.T, exitCode int, state work.State) (string, work.Work, string) {
	t.Helper()
	stateDir := t.TempDir()
	projectPath := t.TempDir()
	workItem := work.Work{
		ID: "work_runner_test", AttemptID: "attempt_runner_test", RequestID: "request_runner_test",
		ProjectID: "project", ProjectPath: projectPath, RunnerID: "shell", SessionName: "mctrl-runner-test",
		State: state, CreatedAt: time.Now().UTC(), KeepAwake: true, LaunchStage: work.StageAccepted,
		PromptDelivery: work.PromptNone,
	}
	store := work.NewStore(stateDir)
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(workItem); err != nil {
		t.Fatal(err)
	}
	spec := work.LaunchSpec{
		WorkID: workItem.ID, AttemptID: workItem.AttemptID, ProjectID: workItem.ProjectID,
		ProjectPath: workItem.ProjectPath, RunnerID: workItem.RunnerID, StorePrompt: false,
		KeepAwake: workItem.KeepAwake, StateDir: stateDir,
	}
	if err := store.WriteLaunchSpec(spec); err != nil {
		t.Fatal(err)
	}
	shell := filepath.Join(t.TempDir(), "shell")
	if err := os.WriteFile(shell, []byte("#!/bin/sh\nexit "+strconv.Itoa(exitCode)+"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHELL", shell)
	toolRoot := t.TempDir()
	fakeCaffeinate := filepath.Join(toolRoot, "caffeinate")
	caffeinateScript := "#!/bin/sh\ntrap 'exit 0' INT TERM\nwhile :; do sleep 1; done\n"
	if err := os.WriteFile(fakeCaffeinate, []byte(caffeinateScript), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", toolRoot+string(os.PathListSeparator)+os.Getenv("PATH"))
	launchFile := filepath.Join(stateDir, "work", workItem.ID+".launch.json")
	return stateDir, workItem, launchFile
}

func exitCommand(code int) *exec.Cmd {
	return exec.Command("/bin/sh", "-c", "exit "+strconv.Itoa(code))
}
