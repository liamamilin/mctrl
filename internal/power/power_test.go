package power

import (
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"
)

func TestPowerHelperProcess(t *testing.T) {
	switch os.Getenv("MCTRL_POWER_HELPER") {
	case "":
		return
	case "exit":
		os.Exit(7)
	case "wait":
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
		<-signals
		os.Exit(0)
	default:
		os.Exit(9)
	}
}

func startPowerHelper(t *testing.T, mode string, args ...string) (*Assertion, error) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	t.Setenv("MCTRL_POWER_HELPER", mode)
	helperArgs := []string{"-test.run=^TestPowerHelperProcess$", "--"}
	helperArgs = append(helperArgs, args...)
	return startCommand(executable, helperArgs...)
}

func TestAssertionTracksProcessAndReleases(t *testing.T) {
	assertion, err := startPowerHelper(t, "wait", "-i", "-w", "123")
	if err != nil {
		t.Fatal(err)
	}
	if !assertion.Active() || assertion.PID() <= 0 {
		t.Fatal("fresh assertion is not active")
	}
	assertion.Release()
	assertion.Release()
	if assertion.Active() {
		t.Fatal("released assertion remains active")
	}
	select {
	case <-assertion.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("released assertion did not finish")
	}
}

func TestAssertionReportsUnexpectedExit(t *testing.T) {
	assertion, err := startPowerHelper(t, "exit", "-i")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-assertion.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("assertion did not observe child exit")
	}
	if assertion.Active() {
		t.Fatal("exited assertion reports active")
	}
	if assertion.Err() == nil {
		t.Fatal("unexpected assertion exit has no error")
	}
	assertion.Release()
}

func TestWorkAssertionCommandUsesExactOwnerPID(t *testing.T) {
	if !workAssertionCommandActive("123 /usr/bin/caffeinate -i -w 12", 12) {
		t.Fatal("exact owner PID was not detected")
	}
	if workAssertionCommandActive("123 /usr/bin/caffeinate -i -w 123", 12) {
		t.Fatal("owner PID prefix was falsely matched")
	}
	if workAssertionCommandActive("123 /bin/sh -c caffeinate -i -w 12", 12) {
		t.Fatal("non-caffeinate command was falsely matched")
	}
}
