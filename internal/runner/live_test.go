package runner

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// Opt-in because it uses the operator's configured provider and credentials.
// Ordinary CI validates plans without consuming model requests.
func TestAuthenticatedAgentContracts(t *testing.T) {
	if os.Getenv("MCTRL_LIVE_RUNNERS") != "1" {
		t.Skip("set MCTRL_LIVE_RUNNERS=1 to test authenticated providers")
	}
	for _, id := range []string{"codex", "opencode"} {
		t.Run(id, func(t *testing.T) {
			root := t.TempDir()
			plan, err := NewRegistry().BuildLaunch(WorkSpec{RunnerID: id, ProjectPath: root, Prompt: "Reply with exactly MCTRL_RUNNER_OK. Do not use tools or edit files."})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			cmd := exec.CommandContext(ctx, plan.Executable, plan.Args...)
			cmd.Dir = plan.Cwd
			output, runErr := cmd.CombinedOutput()
			types := []string{}
			messages := []string{}
			confirmed := false
			for _, line := range strings.Split(string(output), "\n") {
				var event struct {
					Type    string `json:"type"`
					Message string `json:"message"`
					Error   struct {
						Message string `json:"message"`
						Name    string `json:"name"`
					} `json:"error"`
					Item struct {
						Type string `json:"type"`
						Text string `json:"text"`
					} `json:"item"`
					Part struct {
						Text string `json:"text"`
					} `json:"part"`
				}
				if json.Unmarshal([]byte(line), &event) != nil {
					continue
				}
				if event.Type != "" && len(types) < 20 {
					types = append(types, event.Type)
				}
				if event.Type == "error" {
					message := event.Message
					if message == "" {
						message = event.Error.Name + ": " + event.Error.Message
					}
					if len(message) > 240 {
						message = message[:240]
					}
					messages = append(messages, message)
				}
				if event.Type == "item.completed" && event.Item.Type == "agent_message" && strings.TrimSpace(event.Item.Text) == "MCTRL_RUNNER_OK" {
					confirmed = true
				}
				if event.Type == "text" && strings.TrimSpace(event.Part.Text) == "MCTRL_RUNNER_OK" {
					confirmed = true
				}
			}
			if runErr != nil || !confirmed {
				t.Fatalf("%s contract not confirmed: exit=%v, event types=%v, errors=%v", id, runErr, types, messages)
			}
			t.Logf("%s authenticated response confirmed", id)
		})
	}
}
