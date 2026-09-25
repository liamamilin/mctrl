package runner

import (
	"encoding/json"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectionJSONMatchesAPIFrontEndContract(t *testing.T) {
	data, err := json.Marshal(Detection{ID: "shell", Name: "Shell", Available: true, Path: "/bin/sh", Version: "hidden"})
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, `"id":"shell"`) || !strings.Contains(text, `"name":"Shell"`) || !strings.Contains(text, `"available":true`) || strings.Contains(text, "Path") || strings.Contains(text, "Version") {
		t.Fatalf("unexpected detection JSON: %s", text)
	}
}

func TestRegistryContainsOnlyFrozenBuiltinRunners(t *testing.T) {
	registry := NewRegistry()
	detections := registry.List()
	if len(detections) != 3 {
		t.Fatalf("detected %d runners, want shell/codex/opencode", len(detections))
	}
	if _, err := registry.Get("unknown"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown runner error = %v", err)
	}
}

func TestShellDetectionRequiresUsableShell(t *testing.T) {
	t.Setenv("SHELL", filepath.Join(t.TempDir(), "missing-shell"))
	if detection := (shellRunner{}).Detect(); detection.Available {
		t.Fatalf("missing Shell reported available: %+v", detection)
	}
}

func TestShellLaunchPlan(t *testing.T) {
	registry := NewRegistry()
	plan, err := registry.BuildLaunch(WorkSpec{RunnerID: "shell", ProjectPath: t.TempDir(), Prompt: "echo hi"})
	if err != nil {
		t.Fatal(err)
	}
	if plan.PromptStrategy != PromptTerminal || filepath.Base(plan.Executable) == "" {
		t.Fatalf("unexpected shell plan: %+v", plan)
	}
	if !containsArg(plan.Args, "MCTRL_READY_FILE") || !containsArg(plan.Env, "MCTRL_SHELL=") {
		t.Fatalf("shell plan has no readiness contract: %+v", plan)
	}
}

func TestAgentPlansMatchObservedPositionalPromptContract(t *testing.T) {
	registry := NewRegistry()
	for _, id := range []string{"codex", "opencode"} {
		if _, err := exec.LookPath(id); err != nil {
			t.Skipf("%s is not installed", id)
		}
		plan, err := registry.BuildLaunch(WorkSpec{RunnerID: id, ProjectPath: t.TempDir(), Prompt: "probe prompt"})
		if err != nil {
			t.Fatalf("%s plan: %v", id, err)
		}
		if plan.PromptStrategy != PromptArgv || !containsArg(plan.Args, "probe prompt") {
			t.Fatalf("%s plan does not carry observed prompt argv: %+v", id, plan)
		}
		if id == "codex" && !containsArg(plan.Args, "--skip-git-repo-check") {
			t.Fatalf("codex plan missing observed flag: %+v", plan)
		}
		if id == "opencode" && !containsArg(plan.Args, "--format") {
			t.Fatalf("opencode plan missing observed format flag: %+v", plan)
		}
	}
}

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want || strings.Contains(arg, want) {
			return true
		}
	}
	return false
}
