package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mctrl/internal/work"
)

func TestWorkArchiveRequiresAuthCSRFAndConfirmation(t *testing.T) {
	adapter, _ := isolateTestTmux(t)
	phone := pairPhone(t, adapter)
	item := work.Work{ID: "work_archive", AttemptID: "attempt_archive", State: work.StateRunning, CreatedAt: time.Now().Add(-time.Minute), SessionName: "missing", RunnerPID: 9999999, ChildPID: 9999998}
	if err := phone.server.works.Save(item); err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/work/" + item.ID + "/archive"
	for _, tc := range []struct {
		name                  string
		cookie, csrf, confirm bool
		status                int
	}{
		{"unauthenticated", false, false, true, http.StatusUnauthorized},
		{"missing CSRF", true, false, true, http.StatusForbidden},
		{"missing confirmation", true, true, false, http.StatusBadRequest},
		{"confirmed", true, true, true, http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, phone.httpServer.URL+path, strings.NewReader(`{"confirm":`+map[bool]string{true: "true", false: "false"}[tc.confirm]+`}`))
			request.Header.Set("Origin", phone.httpServer.URL)
			if tc.cookie {
				request.Header.Set("Cookie", "mctrl_session="+phone.session)
			}
			if tc.csrf {
				request.Header.Set("X-CSRF-Token", phone.csrf)
			}
			recorder := httptest.NewRecorder()
			phone.server.Handler().ServeHTTP(recorder, request)
			if recorder.Code != tc.status {
				t.Fatalf("response = %d %s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestArchivedWorkUnblocksProjectAndCannotResumeLaunch(t *testing.T) {
	adapter, _ := isolateTestTmux(t)
	phone := pairPhone(t, adapter)
	project, err := phone.server.projects.Add("Recovered", t.TempDir(), "shell")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	item := work.Work{ID: "work_archived_retry", AttemptID: "attempt_retry", State: work.StateAccepted, ProjectID: project.ID, RunnerID: "shell", RequestID: "same", CreatedAt: now.Add(-time.Minute), ArchivedAt: &now}
	if err := phone.server.works.Save(item); err != nil {
		t.Fatal(err)
	}
	got, failure, err := phone.server.launchWithLock(item, project, createWorkRequest{ProjectID: project.ID, RunnerID: "shell"})
	if err != nil || failure != nil || got.ArchivedAt == nil || got.SessionID != "" {
		t.Fatalf("archived Work resumed: %+v %+v %v", got, failure, err)
	}
	recorder := phone.do(t, http.MethodDelete, "/api/v1/projects/"+project.ID, "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("archived Work blocked removal: %d %s", recorder.Code, recorder.Body.String())
	}
}
