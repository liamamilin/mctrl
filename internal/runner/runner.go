package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var ErrNotFound = errors.New("runner not found")

type PromptStrategy string

const (
	PromptNone     PromptStrategy = "none"
	PromptArgv     PromptStrategy = "argv"
	PromptTerminal PromptStrategy = "terminal_injection"
)

type Detection struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Available bool   `json:"available"`
	Path      string `json:"-"`
	Version   string `json:"-"`
}

type WorkSpec struct {
	WorkID      string
	ProjectID   string
	ProjectPath string
	RunnerID    string
	Prompt      string
}

type LaunchPlan struct {
	RunnerID       string
	Executable     string
	Args           []string
	Env            []string
	Cwd            string
	PromptStrategy PromptStrategy
}

type Runner interface {
	ID() string
	Name() string
	Detect() Detection
	BuildLaunch(WorkSpec) (LaunchPlan, error)
}

type Registry struct {
	runners map[string]Runner
}

func NewRegistry() *Registry {
	return &Registry{runners: map[string]Runner{
		"shell":    shellRunner{},
		"codex":    codexRunner{},
		"opencode": opencodeRunner{},
	}}
}

func (r *Registry) Get(id string) (Runner, error) {
	runner, ok := r.runners[strings.ToLower(strings.TrimSpace(id))]
	if !ok {
		return nil, ErrNotFound
	}
	return runner, nil
}

func (r *Registry) List() []Detection {
	result := make([]Detection, 0, len(r.runners))
	for _, id := range []string{"shell", "codex", "opencode"} {
		result = append(result, r.runners[id].Detect())
	}
	return result
}

func (r *Registry) BuildLaunch(spec WorkSpec) (LaunchPlan, error) {
	runner, err := r.Get(spec.RunnerID)
	if err != nil {
		return LaunchPlan{}, err
	}
	return runner.BuildLaunch(spec)
}

type shellRunner struct{}

func (shellRunner) ID() string   { return "shell" }
func (shellRunner) Name() string { return "Shell" }
func (shellRunner) Detect() Detection {
	path := os.Getenv("SHELL")
	if path == "" {
		path = "/bin/zsh"
	}
	resolved, err := exec.LookPath(path)
	if err != nil {
		return Detection{ID: "shell", Name: "Shell", Available: false}
	}
	return Detection{ID: "shell", Name: "Shell", Available: true, Path: resolved}
}

func (shellRunner) BuildLaunch(spec WorkSpec) (LaunchPlan, error) {
	path := os.Getenv("SHELL")
	if path == "" {
		path = "/bin/zsh"
	}
	resolved, err := exec.LookPath(path)
	if err != nil {
		return LaunchPlan{}, fmt.Errorf("shell executable not found: %w", err)
	}
	cwd, err := existingDirectory(spec.ProjectPath)
	if err != nil {
		return LaunchPlan{}, err
	}
	return LaunchPlan{
		RunnerID:       "shell",
		Executable:     "/bin/sh",
		Args:           []string{"-c", `if [ -n "${MCTRL_READY_FILE:-}" ]; then printf ready > "$MCTRL_READY_FILE"; fi; exec "$MCTRL_SHELL" -l`},
		Env:            []string{"MCTRL_SHELL=" + resolved},
		Cwd:            cwd,
		PromptStrategy: PromptTerminal,
	}, nil
}

type codexRunner struct{}

func (codexRunner) ID() string   { return "codex" }
func (codexRunner) Name() string { return "Codex" }
func (codexRunner) Detect() Detection {
	path, err := findExecutable("codex")
	if err != nil {
		return Detection{ID: "codex", Name: "Codex", Available: false}
	}
	return Detection{ID: "codex", Name: "Codex", Available: true, Path: path, Version: probeVersion(path)}
}

func (codexRunner) BuildLaunch(spec WorkSpec) (LaunchPlan, error) {
	path, err := findExecutable("codex")
	if err != nil {
		return LaunchPlan{}, fmt.Errorf("codex executable not found: %w", err)
	}
	cwd, err := existingDirectory(spec.ProjectPath)
	if err != nil {
		return LaunchPlan{}, err
	}
	args := []string{"exec", "--json", "--skip-git-repo-check", "-C", cwd}
	if strings.TrimSpace(spec.Prompt) != "" {
		// Codex's installed CLI explicitly supports an initial prompt as a
		// positional argument. The observed contract is recorded in
		// docs/runner-probes.md; no terminal replay is used.
		args = append(args, "--", spec.Prompt)
	}
	return LaunchPlan{RunnerID: "codex", Executable: path, Args: args, Cwd: cwd, PromptStrategy: PromptArgv}, nil
}

type opencodeRunner struct{}

func (opencodeRunner) ID() string   { return "opencode" }
func (opencodeRunner) Name() string { return "OpenCode" }
func (opencodeRunner) Detect() Detection {
	path, err := findExecutable("opencode")
	if err != nil {
		return Detection{ID: "opencode", Name: "OpenCode", Available: false}
	}
	return Detection{ID: "opencode", Name: "OpenCode", Available: true, Path: path, Version: probeVersion(path)}
}

func (opencodeRunner) BuildLaunch(spec WorkSpec) (LaunchPlan, error) {
	path, err := findExecutable("opencode")
	if err != nil {
		return LaunchPlan{}, fmt.Errorf("opencode executable not found: %w", err)
	}
	cwd, err := existingDirectory(spec.ProjectPath)
	if err != nil {
		return LaunchPlan{}, err
	}
	args := []string{"run", "--format", "json"}
	if strings.TrimSpace(spec.Prompt) != "" {
		args = append(args, "--", spec.Prompt)
	}
	return LaunchPlan{RunnerID: "opencode", Executable: path, Args: args, Cwd: cwd, PromptStrategy: PromptArgv}, nil
}

func findExecutable(name string) (string, error) {
	if path, err := exec.LookPath(name); err == nil {
		return path, nil
	}
	candidates := []string{
		filepath.Join("/opt/homebrew/bin", name),
		filepath.Join("/usr/local/bin", name),
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, ".opencode", "bin", name))
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() && info.Mode()&0111 != 0 {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("%s executable not found", name)
}

func existingDirectory(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("project path is required")
	}
	clean, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve project path: %w", err)
	}
	info, err := os.Stat(clean)
	if err != nil {
		return "", fmt.Errorf("project path: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("project path is not a directory: %s", clean)
	}
	return clean, nil
}

func probeVersion(path string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}
