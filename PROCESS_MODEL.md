# Process Ownership Model

This document is normative.

## Process tree for Managed Work

Conceptually:

```text
tmux server
   │
   └── managed session/pane
           │
           └── mctrl-runner
                   │
                   └── child process
                       (shell/codex/opencode)
```

The daemon is **not** the parent whose lifetime controls Managed Work.

## Ownership

### mctrl daemon owns

- HTTP server
- WebSocket gateway
- pairing window
- Project registry access
- host Remote Ready assertion
- Work launch coordination
- reconciliation
- attach PTYs

### tmux owns

- Session lifetime
- window/pane lifetime
- terminal multiplexing
- persistence across client detach

### mctrl-runner owns

- child process start
- child process wait
- child PID recording
- work-level power assertion
- exit-code recording
- final Work state write

### browser owns

Nothing durable.

## Launch ownership sequence

```text
daemon accepts request
        ↓
daemon persists ACCEPTED
        ↓
daemon creates tmux session/pane
        ↓
tmux launches mctrl-runner
        ↓
mctrl-runner atomically claims the attempt-scoped launch evidence
        ↓
mctrl-runner persists STARTING evidence
        ↓
mctrl-runner acquires work assertion
        ↓
mctrl-runner starts child
        ↓
mctrl-runner records child PID
        ↓
Work becomes RUNNING
        ↓
runner waits for child
        ↓
child exits
        ↓
runner records the canonical attempt-scoped exit result
        ↓
runner records exit code/signal on Work
        ↓
Work becomes EXITED
        ↓
runner releases work assertion
```

## Critical rule

The daemon must not `wait()` on the child as the only source of exit truth.

`mctrl-runner` is the authoritative observer of the child exit.

## Daemon crash during launch

The launch protocol must leave enough persistent evidence to reconcile:

- Work record
- intended session name
- runner PID if known
- child PID if known
- launch stage

## mctrl-runner crash

If runner disappears before recording child exit:

- daemon must not invent an exit code,
- reconciliation may mark factual abnormal termination metadata,
- the tmux Session must be preserved when possible.

## Cleanup policy

V1 favors preserving evidence over aggressive cleanup.

Do not silently delete:

- failed session,
- Work record,
- diagnostic log

until behavior is understood and tested.
