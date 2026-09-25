# Domain Model

## Host

```json
{
  "name": "Milin's MacBook",
  "hostname": "milin-macbook",
  "status": "online",
  "remote_ready": true
}
```

Allowed observable status:

- online
- reconnecting
- offline

## Project

```json
{
  "id": "personal-control",
  "name": "Personal Control",
  "path": "/Users/you/Projects/personal-control",
  "default_runner": "codex"
}
```

## Session

Normalized tmux view:

```json
{
  "id": "$3",
  "name": "personal-control",
  "windows": 2,
  "panes": 3,
  "attached": true,
  "active_pane": {
    "command": "opencode",
    "cwd": "/Users/you/Projects/personal-control"
  }
}
```

## Managed Work

```json
{
  "id": "work_01",
  "request_id": "uuid",
  "project_id": "personal-control",
  "runner_id": "codex",
  "session_name": "mctrl-personal-control-a31f",
  "state": "RUNNING",
  "created_at": "...",
  "started_at": "...",
  "runner_pid": 1234,
  "child_pid": 1235,
  "keep_awake": true,
  "launch_stage": "child_started"
}
```

Terminal states:

- EXITED
- LAUNCH_FAILED

Exit facts:

```json
{
  "exit_code": 0,
  "finished_at": "..."
}
```

Unknown recovery state may include:

```json
{
  "termination_reason": "runner_missing_after_recovery"
}
```

## Runner

```json
{
  "id": "codex",
  "name": "Codex",
  "available": true
}
```

## Paired Device

Contains:

- id
- display name
- credential hash or authenticated session binding
- created_at
- last_seen
- revoked_at if revoked

Never store plaintext long-lived credentials server-side.
