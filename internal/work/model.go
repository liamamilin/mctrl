package work

import "time"

type State string

const (
	StateAccepted     State = "ACCEPTED"
	StateStarting     State = "STARTING"
	StateRunning      State = "RUNNING"
	StateExited       State = "EXITED"
	StateLaunchFailed State = "LAUNCH_FAILED"
)

type LaunchStage string

const (
	StageAccepted                LaunchStage = "accepted"
	StageSessionCreated          LaunchStage = "session_created"
	StageRunnerStarted           LaunchStage = "runner_started"
	StageChildStarted            LaunchStage = "child_started"
	StagePromptDeliveryStarted   LaunchStage = "prompt_delivery_started"
	StagePromptDeliveryConfirmed LaunchStage = "prompt_delivery_confirmed"
	StagePromptDeliveryFailed    LaunchStage = "prompt_delivery_failed"
	StageChildExited             LaunchStage = "child_exited"
)

type PromptDelivery string

const (
	PromptPending   PromptDelivery = "pending"
	PromptStarted   PromptDelivery = "started"
	PromptConfirmed PromptDelivery = "confirmed"
	PromptFailed    PromptDelivery = "failed"
	PromptUnknown   PromptDelivery = "unknown"
	PromptNone      PromptDelivery = "none"
)

type Work struct {
	ID                 string         `json:"id"`
	RequestID          string         `json:"request_id"`
	RequestFingerprint string         `json:"request_fingerprint,omitempty"`
	AttemptID          string         `json:"attempt_id,omitempty"`
	DeviceID           string         `json:"device_id"`
	ProjectID          string         `json:"project_id"`
	ProjectPath        string         `json:"project_path"`
	RunnerID           string         `json:"runner_id"`
	SessionName        string         `json:"session_name"`
	SessionID          string         `json:"session_id,omitempty"`
	State              State          `json:"state"`
	CreatedAt          time.Time      `json:"created_at"`
	StartedAt          *time.Time     `json:"started_at,omitempty"`
	FinishedAt         *time.Time     `json:"finished_at,omitempty"`
	RunnerPID          int            `json:"runner_pid,omitempty"`
	ChildPID           int            `json:"child_pid,omitempty"`
	ChildExecutable    string         `json:"child_executable,omitempty"`
	ExitCode           *int           `json:"exit_code,omitempty"`
	ExitSignal         int            `json:"exit_signal,omitempty"`
	KeepAwake          bool           `json:"keep_awake"`
	LaunchStage        LaunchStage    `json:"launch_stage"`
	PromptDelivery     PromptDelivery `json:"prompt_delivery"`
	TerminationReason  string         `json:"termination_reason,omitempty"`
	RecoveryStatus     string         `json:"recovery_status,omitempty"`
	ErrorCode          string         `json:"error_code,omitempty"`
	ErrorMessage       string         `json:"error_message,omitempty"`
	// Prompt is never returned by the API. It is only persisted when the
	// operator explicitly enables store_prompts.
	Prompt string `json:"prompt,omitempty"`
}

type LaunchSpec struct {
	WorkID      string `json:"work_id"`
	AttemptID   string `json:"attempt_id"`
	ProjectID   string `json:"project_id"`
	ProjectPath string `json:"project_path"`
	RunnerID    string `json:"runner_id"`
	Prompt      string `json:"prompt"`
	StorePrompt bool   `json:"store_prompt"`
	KeepAwake   bool   `json:"keep_awake"`
	StateDir    string `json:"state_dir"`
}

type Result struct {
	WorkID      string    `json:"work_id"`
	AttemptID   string    `json:"attempt_id"`
	RunnerPID   int       `json:"runner_pid,omitempty"`
	ChildPID    int       `json:"child_pid,omitempty"`
	ExitCode    *int      `json:"exit_code,omitempty"`
	ExitSignal  int       `json:"exit_signal,omitempty"`
	ObservedAt  time.Time `json:"observed_at"`
	PersistedAt time.Time `json:"persisted_at"`
	FinishedAt  time.Time `json:"finished_at"` // compatibility alias for observed_at
	Reason      string    `json:"reason,omitempty"`
}

func NewID() string {
	return "work_" + randomToken(12)
}

func NewRequestID() string {
	return randomToken(16)
}

func NewAttemptID() string {
	return "attempt_" + randomToken(12)
}

func AdvanceLaunchStage(current, observed LaunchStage) LaunchStage {
	rank := map[LaunchStage]int{
		StageAccepted:                0,
		StageSessionCreated:          1,
		StageRunnerStarted:           2,
		StageChildStarted:            3,
		StagePromptDeliveryStarted:   4,
		StagePromptDeliveryConfirmed: 5,
		StagePromptDeliveryFailed:    5,
		StageChildExited:             6,
	}
	currentRank, currentOK := rank[current]
	observedRank, observedOK := rank[observed]
	if observedOK && (!currentOK || observedRank > currentRank) {
		return observed
	}
	return current
}

func (w Work) Terminal() bool {
	return w.State == StateExited || w.State == StateLaunchFailed
}

func (w Work) DTO() map[string]interface{} {
	result := map[string]interface{}{
		"id":              w.ID,
		"request_id":      w.RequestID,
		"project_id":      w.ProjectID,
		"runner_id":       w.RunnerID,
		"session_name":    w.SessionName,
		"state":           w.State,
		"created_at":      w.CreatedAt,
		"keep_awake":      w.KeepAwake,
		"launch_stage":    w.LaunchStage,
		"prompt_delivery": w.PromptDelivery,
	}
	if w.StartedAt != nil {
		result["started_at"] = w.StartedAt
	}
	if w.FinishedAt != nil {
		result["finished_at"] = w.FinishedAt
	}
	if w.SessionID != "" {
		result["session_id"] = w.SessionID
	}
	if w.RunnerPID != 0 {
		result["runner_pid"] = w.RunnerPID
	}
	if w.ChildPID != 0 {
		result["child_pid"] = w.ChildPID
	}
	if w.ExitCode != nil {
		result["exit_code"] = *w.ExitCode
	}
	if w.ExitSignal != 0 {
		result["exit_signal"] = w.ExitSignal
	}
	if w.TerminationReason != "" {
		result["termination_reason"] = w.TerminationReason
	}
	if w.RecoveryStatus != "" {
		result["recovery_status"] = w.RecoveryStatus
	}
	if w.ErrorCode != "" {
		result["error_code"] = w.ErrorCode
		result["error_message"] = w.ErrorMessage
		result["error"] = map[string]string{"code": w.ErrorCode, "message": w.ErrorMessage}
	}
	return result
}
