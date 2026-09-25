package power

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Assertion represents a caffeinate process owned by one component.
type Assertion struct {
	cmd     *exec.Cmd
	done    chan struct{}
	waitErr error
	noop    bool
	once    sync.Once
}

func (a *Assertion) Release() {
	if a == nil {
		return
	}
	a.once.Do(func() {
		if a.noop || a.cmd == nil || a.cmd.Process == nil {
			return
		}
		select {
		case <-a.done:
			return
		default:
		}
		_ = a.cmd.Process.Signal(os.Interrupt)
		select {
		case <-a.done:
			return
		case <-time.After(2 * time.Second):
		}
		_ = a.cmd.Process.Signal(syscall.SIGTERM)
		select {
		case <-a.done:
			return
		case <-time.After(time.Second):
		}
		_ = a.cmd.Process.Kill()
		<-a.done
	})
}

func (a *Assertion) PID() int {
	if a == nil || a.cmd == nil || a.cmd.Process == nil {
		return 0
	}
	return a.cmd.Process.Pid
}

func (a *Assertion) Active() bool {
	if a == nil || a.noop || a.cmd == nil || a.cmd.Process == nil {
		return false
	}
	select {
	case <-a.done:
		return false
	default:
		return true
	}
}

func (a *Assertion) Done() <-chan struct{} {
	if a == nil {
		return closedSignal()
	}
	return a.done
}

func (a *Assertion) Err() error {
	if a == nil {
		return errors.New("power assertion is unavailable")
	}
	select {
	case <-a.done:
		return a.waitErr
	default:
		return nil
	}
}

func closedSignal() chan struct{} {
	done := make(chan struct{})
	close(done)
	return done
}

func start(args ...string) (*Assertion, error) {
	binary, err := exec.LookPath("caffeinate")
	if err != nil {
		if runtime.GOOS == "darwin" {
			return nil, fmt.Errorf("caffeinate is unavailable: %w", err)
		}
		// Non-macOS development hosts get an explicit inactive no-op. It must
		// never produce a truthful Remote Ready claim on an unsupported host.
		return &Assertion{done: closedSignal(), noop: true}, nil
	}
	return startCommand(binary, args...)
}

func startCommand(binary string, args ...string) (*Assertion, error) {
	cmd := exec.Command(binary, args...)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start caffeinate: %w", err)
	}
	assertion := &Assertion{cmd: cmd, done: make(chan struct{})}
	go func() {
		assertion.waitErr = cmd.Wait()
		close(assertion.done)
	}()
	return assertion, nil
}

func StartHost(mode string, ownerPID ...int) (*Assertion, error) {
	var args []string
	switch mode {
	case "on_ac":
		args = []string{"-s"}
	case "always":
		args = []string{"-i"}
	case "work_only", "":
		return &Assertion{done: closedSignal(), noop: true}, nil
	default:
		return nil, fmt.Errorf("unknown remote availability mode %q", mode)
	}
	if len(ownerPID) > 0 && ownerPID[0] > 0 {
		args = append(args, "-w", strconv.Itoa(ownerPID[0]))
	}
	return start(args...)
}

func StartWork(ownerPID ...int) (*Assertion, error) {
	if runtime.GOOS != "darwin" {
		return nil, errors.New("Work power assertions are supported only on macOS")
	}
	args := []string{"-i"}
	if len(ownerPID) > 0 && ownerPID[0] > 0 {
		args = append(args, "-w", strconv.Itoa(ownerPID[0]))
	}
	return start(args...)
}

// OnAC reports the current macOS power source. A missing/unknown source is
// treated as unavailable rather than as a false Remote Ready claim.
func OnAC() bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	output, err := exec.Command("pmset", "-g", "batt").CombinedOutput()
	if err != nil {
		return false
	}
	text := strings.ToLower(string(output))
	return strings.Contains(text, "ac power")
}

func AssertionActiveForOwner(ownerPID int) bool { return WorkAssertionActive(ownerPID) }

func WorkAssertionActive(runnerPID int) bool {
	if runnerPID <= 0 || runtime.GOOS != "darwin" {
		return false
	}
	output, err := exec.Command("/bin/ps", "-axo", "pid=,command=").Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(output), "\n") {
		if workAssertionCommandActive(line, runnerPID) {
			return true
		}
	}
	return false
}

func workAssertionCommandActive(line string, runnerPID int) bool {
	fields := strings.Fields(line)
	if len(fields) < 2 || filepath.Base(fields[1]) != "caffeinate" {
		return false
	}
	for argumentIndex := 2; argumentIndex+1 < len(fields); argumentIndex++ {
		if fields[argumentIndex] == "-w" && fields[argumentIndex+1] == strconv.Itoa(runnerPID) {
			return true
		}
	}
	return false
}
