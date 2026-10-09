# REST API Contract

Base:

```text
/api/v1
```

## General rules

- JSON for REST.
- stable error codes.
- structured active operations require authentication.
- state-changing requests must pass Origin/CSRF policy.
- `request_id` is required for idempotent Launch operations.

## Host

### GET /host

```json
{
  "name": "Milin's MacBook",
  "hostname": "milin-macbook",
  "status": "online",
  "remote_ready": true,
  "transport": "trusted_lan_http",
  "public_url": "http://192.168.1.20:7681",
  "version": "0.1.0-dev",
  "terminal_full_size": { "cols": 120, "rows": 40 }
}
```

`terminal_full_size` is the Mac's canonical full Session size. The phone's FULL
overview asks tmux for `max(terminal_full_size, current pane size)` instead of
reading the pane back, because the phone's own viewport is what shrinks the shared
window when no desktop client is attached. See `TMUX_CONTRACT.md`.

## Settings

### PUT /settings/terminal-size

Changes the canonical full Session size used by the phone's `FULL` overview, by
new Managed Work launches, and by new terminal attachments.

```json
{
  "terminal_full_size": { "cols": 240, "rows": 60 }
}
```

Requires a paired device and passes the normal Origin/CSRF policy. The value is
validated against the supported range (20–500 columns, 10–200 rows) and persisted
to `config.json`; it takes effect without a daemon restart. See
`CONFIG_SCHEMA.md`.

Errors:

- `INVALID_TERMINAL_SIZE` — the size is outside the supported range
- `CONFIG_WRITE_FAILED` — the daemon could not persist the change, and the
  effective value is left untouched

## Projects

### GET /projects

### POST /projects

`path` is the initial working directory for new Work. The Mac expands `~` to
the user's home directory and stores the resolved absolute path.

### DELETE /projects/:id

Unregisters a Project from future launches. Existing Sessions and Work are not
closed. If the Project still has non-terminal, unarchived Managed Work, the request returns
`409 PROJECT_IN_USE` and nothing is removed.

## Runners

### GET /runners

```json
[
  {"id":"shell","name":"Shell","available":true},
  {"id":"codex","name":"Codex","available":true},
  {"id":"opencode","name":"OpenCode","available":false}
]
```

## Sessions

### GET /sessions

### GET /sessions/:id

Session `active_pane` and `pane_details` include optional `bracketed_paste`, the
pane's actual `bracket_paste_flag` observation. Missing means unknown, not enabled.
This is separate from the tmux client's paste framing. Clients must check current
program support before inserting multiline drafts.

Session `managed_work` may include optional `child_cwd`. It is a read-only,
bounded observation of the registered child of a RUNNING, unarchived Work, only
when the active pane belongs to its Runner. Runner/Attempt identity and child
ownership are verified, and child ownership is checked again after reading. It is omitted on observation
failure and never persisted to Work history. tmux's pane `cwd` remains its own
fact, and may be the Runner's starting directory rather than a Shell's cwd after
`cd`. `/work` does not promise this ephemeral directory field.

### POST /sessions/:id/close

Closes one tmux Session and terminates its windows, panes, and child processes.
This is an explicit destructive action; it does not delete Project or Work
history.

```json
{ "force": false }
```

External Sessions may be closed without `force`. If the Session has active
Managed Work, `force: false` returns `409 WORK_RUNNING`; the caller must
repeat with `force: true` after explicit user confirmation. Repeating the
request after the Session is already gone is treated as an idempotent success.

### GET /sessions/:id/preview?lines=50

```json
{
  "lines": [
    "Running tests...",
    "PASS B01",
    "PASS B02"
  ],
  "captured_at": "..."
}
```

Defaults:

- Home: 20
- Detail: 100

Preview is plain text with terminal escape sequences stripped.
Trailing blank pane rows are removed before applying the requested line limit,
so short output remains visible in a tall pane. Interior blank lines are kept;
an empty pane returns an empty `lines` array.

Desktop continuation fields are optional and read-only:

- `GET /host`: `desktop_start_command` (`mctrl here`, including profile when needed).
- `GET /sessions` and `GET /sessions/:id`: `desktop_resume_command`, quoting the
  daemon's effective socket and observed Session ID/name. Copying it performs no
  execution; the user runs it locally on the Mac, where identity is checked again.

## Work

### GET /work

### GET /work/:id

### POST /work/:id/archive

```json
{ "confirm": true }
```

Archives a lost Work after verifying its Session identifiers and Runner/child
processes are absent. Authentication, Origin and CSRF rules apply. Existing
Sessions, live processes, uncertain observation, fresh launches, or exit evidence
awaiting reconciliation return `409 WORK_ARCHIVE_UNSAFE`.

The response adds `archived_at`. Archival preserves state, launch-stage and exit
facts, history, and the request-id mapping, but removes Project occupancy. It is
idempotent; old Launch requests cannot restart archived Work. Late canonical exit
evidence is still applied.

### POST /work

```json
{
  "request_id": "uuid",
  "project_id": "personal-control",
  "runner_id": "codex",
  "prompt": "补齐测试并运行完整测试。"
}
```

Idempotency:

```text
same authenticated device
+ same request_id
→ same Work
```

A retry must not create another child process. Reusing the same authenticated
device and `request_id` with a different Project, Runner, or Prompt returns
`IDEMPOTENCY_KEY_REUSED`; the original Work is never rewritten.

## Prompt action

If V1 exposes structured prompt sending after launch, use a dedicated idempotent endpoint rather than replaying raw terminal bytes.

Example shape:

```text
POST /sessions/:id/prompt
```

with:

```json
{
  "request_id": "uuid",
  "text": "Continue and run the tests."
}
```

Do not implement this endpoint unless Prompt Mode requires it.

## Devices

- GET /devices — returns active device records only
- GET /devices/history — returns revoked device audit records, newest first
- POST /devices/links — creates a two-minute, one-time link token for the
  current authenticated device
- POST /devices/link — consumes a browser link token and creates a new browser
  session under the same device record
- POST /devices/installation — binds the current authenticated browser to its
  persistent installation identity
- DELETE /devices/:id

Browser-link request:

```json
{
  "token": "one-time-link-token",
  "installation_id": "safari-generated-random-id"
}
```

The raw link token is returned only to the authenticated browser that created
it, is stored on the Mac only as salted hash material, expires after two minutes,
and is removed when consumed. Linking never exposes the device credential.

Installation binding request:

```json
{
  "installation_id": "browser-generated-random-id"
}
```

The daemon stores only a one-way hash of the installation identity. Re-pairing
the same browser installation reuses its device record and rotates credentials.

## Pairing

- POST /pair
- POST /pair/qr — loopback-only same-origin endpoint used by the Mac Pair
  Console; validates the current one-time token and returns PNG QR data plus the
  manual phone Pair URL without consuming the token

The QR response has `data_url`, `pair_url`, `expires_at`, and optional
`fallback_pair_url`. With an unconfigured Trusted-LAN wildcard listener, the QR
encodes the Mac's Bonjour hostname and token. `pair_url` is the token-free manual
address; the optional fallback is the current numeric LAN address and accepts the
same code. Configured public URLs and explicit/loopback listeners do not gain an
alternate origin, and TLS mode never adds a plaintext LAN fallback. All existing
loopback, Origin, expiry and single-use checks remain in force.

The V1 UI sends:

```json
{
  "token": "...",
  "device_name": "My iPhone",
  "installation_id": "browser-generated-random-id"
}
```

The daemon also accepts the equivalent `pair_token` field for API clients. The
pairing token remains single-use and short-lived. The installation ID is not an
authentication credential; successful pairing still requires the valid token.

## Errors

```json
{
  "error": {
    "code": "RUNNER_NOT_FOUND",
    "message": "Codex executable was not found.",
    "details": {}
  }
}
```

Stable V1 codes:

- UNAUTHORIZED
- FORBIDDEN_ORIGIN
- CSRF_FAILED
- PAIRING_CLOSED
- PAIR_TOKEN_EXPIRED
- DEVICE_LINK_EXPIRED
- INVALID_INSTALLATION_ID
- INSTALLATION_ALREADY_BOUND
- PROJECT_NOT_FOUND
- PROJECT_PATH_MISSING
- RUNNER_NOT_FOUND
- SESSION_NOT_FOUND
- TMUX_UNAVAILABLE
- TMUX_SESSION_COLLISION
- WORK_NOT_FOUND
- WORK_LAUNCH_FAILED
- WORK_ATTEMPT_CHANGED
- WORK_ARCHIVE_UNSAFE
- IDEMPOTENCY_KEY_REUSED
- PROMPT_DELIVERY_FAILED
- TERMINAL_ATTACH_FAILED
- INVALID_REQUEST
- TRANSPORT_POLICY_VIOLATION
- INTERNAL_ERROR
