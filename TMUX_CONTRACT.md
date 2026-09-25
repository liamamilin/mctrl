# tmux Contract

This is a normative adapter contract.

## tmux is source of truth for Sessions

Existing sessions created outside mctrl must be discoverable.

## Session identity

Prefer tmux stable session identifiers where possible.

Do not rely solely on display names if names can collide or change.

API may expose both:

- stable/internal id
- human name

## Required adapter operations

- list sessions
- inspect session
- list windows/panes
- get active pane
- get pane current command
- get pane cwd
- capture pane
- create managed session
- attach session
- verify session existence
- apply/verify size policy

## Session creation naming

Managed sessions use a collision-resistant generated name, for example:

```text
mctrl-<project-slug>-<short-random>
```

Do not assume one Project has only one Session.

## capture-pane

Preview capture must:

- target a specific pane,
- bound line count,
- strip or normalize escape sequences for preview,
- never mutate terminal state.

## Active pane

V1 Home/Detail may summarize only the active pane.

Do not pretend this is the whole Session.

## Phone attach size policy

Desktop experience has priority.

If a desktop client is 120×40 and phone is 44×22, the phone must not force the shared window down to 44×22.

Use a largest-client-oriented tmux policy or equivalent verified mechanism.

This behavior requires integration tests with two clients.

## Detach semantics

Closing the phone attachment must detach only that client.

It must not:

- kill pane,
- kill window,
- kill session,
- send unintended EOF to the user's durable shell.

## Session disappearance

If the target Session is gone:

- stop attachment,
- emit SESSION_GONE,
- return UI to a safe page.

## Contract tests

All adapter behavior must be verified against the tmux version(s) supported by V1.

Do not rely on guessed command formatting.
