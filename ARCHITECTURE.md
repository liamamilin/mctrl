# Architecture

## Topology

```text
┌──────────────────────────────────────┐
│ iPhone Safari / installed PWA        │
│ Preact + TypeScript + xterm.js       │
└──────────────────┬───────────────────┘
                   │
            HTTP(S) / WS(S)
                   │
┌──────────────────▼───────────────────┐
│ mctrl daemon                         │
│ Go                                   │
│                                      │
│ API / Auth / Pairing                 │
│ Project Registry                     │
│ Session Discovery                    │
│ Preview Capture                      │
│ Work Coordinator                     │
│ Terminal Gateway                     │
│ Host Power Policy                    │
└──────────────────┬───────────────────┘
                   │
                  tmux
         ┌─────────┴──────────┐
         │                    │
existing session       managed session
                              │
                        mctrl-runner
                              │
                    shell/codex/opencode
```

## Durability layers

### Layer A — durable execution

```text
tmux
mctrl-runner
child process
```

Must survive:

- Safari close,
- phone lock,
- Wi-Fi interruption,
- WebSocket loss,
- mctrl daemon restart.

### Layer B — recoverable control plane

```text
mctrl daemon
```

May restart and reconcile.

### Layer C — disposable presentation

```text
browser
WebSocket
attach PTY
```

May be destroyed freely.

## Repository layout

```text
mctrl/
├── cmd/
│   ├── mctrl/
│   └── mctrl-runner/
├── internal/
│   ├── api/
│   ├── auth/
│   ├── config/
│   ├── host/
│   ├── power/
│   ├── project/
│   ├── runner/
│   ├── session/
│   ├── terminal/
│   ├── tmux/
│   └── work/
├── web/
├── tests/
├── docs/
└── go.mod
```

## tmux adapter boundary

Only `internal/tmux` may encode tmux command syntax.

Upper layers consume typed methods.

Examples:

```text
ListSessions
InspectSession
CapturePane
CreateManagedSession
AttachSession
SessionExists
ConfigureSizingPolicy
```

## No universal command endpoint

Do not implement:

```text
POST /execute
POST /shell
```

The terminal stream and registered Project + Runner launch path are the only V1 active surfaces.
