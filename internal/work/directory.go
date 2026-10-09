package work

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// CurrentDirectory observes the registered child, which lives behind the
// runner's PTY. tmux's pane_current_path describes the runner, not a Shell's
// directory after cd. This fact is never persisted as a lifecycle transition.
func CurrentDirectory(ctx context.Context, item Work) (string, error) {
	if !item.Active() || item.State != StateRunning || item.RunnerPID <= 0 || item.ChildPID <= 0 {
		return "", fmt.Errorf("no live registered child")
	}
	runner, err := exec.CommandContext(ctx, "ps", "-p", strconv.Itoa(item.RunnerPID), "-o", "state=,command=").Output()
	fields := strings.Fields(string(runner))
	if err != nil || len(fields) < 2 || strings.HasPrefix(fields[0], "Z") || !runnerCommandMatchesAttempt(strings.Join(fields[1:], " "), item.ID, item.AttemptID) {
		return "", fmt.Errorf("runner identity cannot be verified")
	}
	owned := func() bool {
		output, err := exec.CommandContext(ctx, "ps", "-p", strconv.Itoa(item.ChildPID), "-o", "ppid=,state=").Output()
		fields := strings.Fields(string(output))
		return err == nil && len(fields) == 2 && fields[0] == strconv.Itoa(item.RunnerPID) && !strings.HasPrefix(fields[1], "Z")
	}
	if !owned() {
		return "", fmt.Errorf("child ownership cannot be verified")
	}
	var directory string
	switch runtime.GOOS {
	case "darwin":
		output, readErr := exec.CommandContext(ctx, "/usr/sbin/lsof", "-a", "-p", strconv.Itoa(item.ChildPID), "-d", "cwd", "-Fn0").Output()
		err = readErr
		directory = directoryFromLsof(output)
	case "linux":
		directory, err = os.Readlink(fmt.Sprintf("/proc/%d/cwd", item.ChildPID))
	default:
		err = fmt.Errorf("process directory observation unsupported")
	}
	if err != nil || !filepath.IsAbs(directory) || !owned() || ctx.Err() != nil {
		return "", fmt.Errorf("child directory cannot be verified")
	}
	return directory, nil
}

func directoryFromLsof(output []byte) string {
	for _, field := range strings.Split(string(output), "\x00") {
		field = strings.TrimPrefix(field, "\n")
		if strings.HasPrefix(field, "n/") {
			return field[1:]
		}
	}
	return ""
}
