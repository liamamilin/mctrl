# Lifecycle Specification

## Independent state machines

### Host reachability

```text
ONLINE
RECONNECTING
OFFLINE
```

Do not expose SLEEPING unless separately verifiable in a future version.

### tmux Session

```text
EXISTS
GONE
```

### Managed Work

```text
ACCEPTED
    ↓
STARTING
   ↙   ↘
RUNNING  LAUNCH_FAILED
   ↓
EXITED
```

### Browser connection

```text
DISCONNECTED
   ↓
CONNECTING
   ↓
CONNECTED
   ↓
RECONNECTING
   ↓
CONNECTED / DISCONNECTED
```

## Independence

### Browser closes

```text
connection → DISCONNECTED
session    → unchanged
work       → unchanged
```

### WebSocket drops

```text
attach PTY → destroy
tmux       → unchanged
work       → unchanged
```

### daemon restarts

```text
daemon       → restart
tmux         → unchanged
mctrl-runner → unchanged
child        → unchanged
```

### child exits

```text
Work → EXITED
Session → EXISTS
```

## Launch stage evidence

During STARTING, record the most recent confirmed stage, for example:

```text
accepted
session_created
runner_started
child_started
prompt_delivery_started
prompt_delivery_confirmed
```

Do not move stages based on assumptions.

## Prompt delivery failure

Prompt delivery failure is not automatically equivalent to child failure.

Persist the facts separately:

```text
child_running = true
prompt_delivery = failed
```

Then surface a clear launch/intervention error instead of killing the Session blindly.
