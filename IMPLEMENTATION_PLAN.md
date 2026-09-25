# Implementation Plan

## Phase 0 — Skeleton

- Go binaries
- Preact/TypeScript
- embedded assets
- CLI skeleton
- test harness

## Phase 1 — tmux contract

Implement adapter and contract tests first.

Do not build UI assumptions before tmux behavior is proven.

## Phase 2 — Read-only UI

- Host
- Sessions
- previews
- polling

## Phase 3 — Terminal bridge

- PTY attach
- WebSocket binary stream
- xterm.js
- resize
- reconnect
- multi-client size test

## Phase 4 — Pairing/auth

- pairing window
- device auth
- Origin policy
- CSRF policy
- revocation
- transport-profile behavior

## Phase 5 — Project registry

CLI and API.

## Phase 6 — Managed Work core

- Work persistence
- mctrl-runner
- Shell runner
- idempotency
- launch-stage evidence
- reconciliation

## Phase 7 — Runner probes

Before Codex/OpenCode implementation:

- inspect versions
- inspect help
- execute disposable probes
- document contract
- add integration tests

Then implement adapters.

## Phase 8 — Prompt mode

Implement structured Prompt delivery with explicit uncertainty handling.

Never solve uncertainty by blind replay.

## Phase 9 — Power management

- host assertion
- work assertion
- release lifecycle

## Phase 10 — PWA polish

- manifest
- standalone
- safe area
- keyboard/touch
- transport status

## Phase 11 — Fault injection

Intentionally test:

- daemon kill
- network disconnect
- runner kill
- Session removal
- prompt-delivery failure
- duplicate request
- auth revocation

## Phase 12 — Release candidate

No features.

Only:

- test
- diagnose
- fix
- real iPhone smoke
- docs
