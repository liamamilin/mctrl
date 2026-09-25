# Frozen V1 Decisions

This file records decisions that implementation agents must not silently revisit.

## D1 — tmux is the durable session runtime

Do not replace tmux with a custom PTY session manager in V1.

## D2 — Two binaries

Build:

```text
mctrl
mctrl-runner
```

`mctrl` is the control plane.

`mctrl-runner` supervises one Managed Work.

## D3 — No database

Persistence uses atomic files under:

```text
~/.mctrl/
```

Do not add SQLite unless a concrete implementation blocker is demonstrated.

## D4 — macOS first

Linux may be added later.

Do not widen V1 platform support.

## D5 — LAN first

V1 works without cloud infrastructure.

## D6 — Security honesty

Plain local HTTP is allowed only as **Trusted-LAN mode**.

Do not describe it as end-to-end secure.

Secure transport is a separate explicit deployment mode.

## D7 — No wake guarantee

Remote Ready prevents ordinary idle sleep according to policy.

V1 does not promise to revive an unreachable Mac.

## D8 — No semantic agent state

Do not add WAITING / THINKING / SUCCESS / FAILED_TASK.

## D9 — Existing sessions are source-of-truth data

mctrl must discover sessions not created by mctrl.

## D10 — Raw terminal input is never replayed

Reconnect creates a fresh attachment.

Uncertain keypresses are not retransmitted.

## D11 — Structured Launch is idempotent

`request_id` prevents duplicate Work creation.

## D12 — Projects constrain launch scope

The phone launches only registered Projects.

No generic arbitrary command execution endpoint.

## D13 — Terminal is not the home screen

Home focuses on:

- host reachability,
- Start Work,
- sessions,
- recent output.

## D14 — No plugin framework

Runner adapters are internal V1 code.

## D15 — Product layer is frozen

Implementation discoveries may change implementation details, not product scope, unless the conflict is explicitly documented and approved.
