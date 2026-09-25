# Coding Agent Guide

## Mission

Implement **mctrl V1 Final** according to the frozen specification.

Deliver runnable software, not snippets.

## Do not redesign the product

The following are frozen:

- product boundary
- core domain model
- tmux durability boundary
- daemon/runner separation
- V1 scope
- no semantic Agent state
- no cloud requirement
- no wake guarantee
- no database
- no generic execute API

## Prohibited silent additions

Do not add:

- cloud backend
- multi-host
- SQLite/Postgres
- native iOS app
- file browser
- Git GUI
- notifications
- wake-from-sleep promise
- arbitrary command endpoint
- plugin marketplace
- semantic AI state inference
- custom replacement for tmux persistence

## Implementation-discovery rule

If reality conflicts with the spec:

1. reproduce the conflict,
2. collect exact evidence,
3. state which spec clause is affected,
4. propose the smallest change,
5. stop product-level redesign until approved.

## Runner rule

Never invent CLI behavior.

For Codex/OpenCode:

1. inspect actual installed version/help,
2. probe behavior,
3. document evidence,
4. add contract test,
5. implement.

## Reliability rule

A browser connection is disposable.

A daemon is recoverable.

A tmux Session and running Managed Work are durable.

Keep these boundaries in code.

## Input rule

Never replay uncertain raw terminal input.

Never replay an uncertain initial/structured Prompt unless the protocol proves idempotent delivery.

## Security rule

Do not describe Trusted-LAN HTTP as encrypted or secure against malicious LAN peers.

## Code completeness

Each implementation phase must leave the repository runnable.

No critical-path placeholder implementation.

## Testing

Any change touching:

- tmux
- process ownership
- Work lifecycle
- recovery
- auth
- WebSocket
- prompt delivery
- power

requires tests.

## Milestone report

For every phase report:

- implemented
- tests run
- exact commands
- exit criterion
- known limitations
- unresolved spec conflicts

## Definition of Done

V1 is not done until `TEST_PLAN.md` release gates pass.
