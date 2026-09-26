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

## Projects

### GET /projects

### POST /projects

`path` is the initial working directory for new Work. The Mac expands `~` to
the user's home directory and stores the resolved absolute path.

### DELETE /projects/:id

Unregisters a Project from future launches. Existing Sessions and Work are not
closed. If the Project still has non-terminal Managed Work, the request returns
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

## Work

### GET /work

### GET /work/:id

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
- IDEMPOTENCY_KEY_REUSED
- PROMPT_DELIVERY_FAILED
- TERMINAL_ATTACH_FAILED
- INVALID_REQUEST
- TRANSPORT_POLICY_VIOLATION
- INTERNAL_ERROR
