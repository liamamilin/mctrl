package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"mctrl/internal/config"
)

func TestDesktopCommandsUseTheDaemonsEffectiveConnection(t *testing.T) {
	adapter, socket := isolateTestTmux(t)
	phone := pairPhone(t, adapter)
	connection, err := config.LoadDesktopConnection(phone.server.stateDir, phone.server.profileName)
	if err != nil || connection.Socket != socket {
		t.Fatalf("recorded connection = %+v, %v", connection, err)
	}
	host := phone.do(t, http.MethodGet, "/api/v1/host", "")
	var info struct {
		Command string `json:"desktop_start_command"`
	}
	if err := json.Unmarshal(host.Body.Bytes(), &info); err != nil || info.Command != "mctrl here" {
		t.Fatalf("desktop entry = %s, %v", host.Body.String(), err)
	}
	name := "desktop's 项目"
	if output, err := testTmuxCommand(socket, "new-session", "-d", "-s", name, "sleep 30").CombinedOutput(); err != nil {
		t.Fatalf("create Session: %s, %v", output, err)
	}
	items := phone.do(t, http.MethodGet, "/api/v1/sessions", "")
	var sessions []struct {
		ID      string `json:"id"`
		Command string `json:"desktop_resume_command"`
	}
	if err := json.Unmarshal(items.Body.Bytes(), &sessions); err != nil || len(sessions) != 1 {
		t.Fatalf("sessions = %s, %v", items.Body.String(), err)
	}
	if !strings.Contains(sessions[0].Command, "--socket '") || !strings.Contains(sessions[0].Command, "--name 'desktop'\\''s 项目'") {
		t.Fatalf("resume command = %q", sessions[0].Command)
	}
	detail := phone.do(t, http.MethodGet, "/api/v1/sessions/"+sessions[0].ID, "")
	var session struct {
		Command string `json:"desktop_resume_command"`
	}
	if err := json.Unmarshal(detail.Body.Bytes(), &session); err != nil || session.Command != sessions[0].Command {
		t.Fatalf("detail resume = %s, %v", detail.Body.String(), err)
	}
	if got := desktopStartCommand("v2"); got != "mctrl --profile v2 here" {
		t.Fatalf("isolated start = %q", got)
	}
}
