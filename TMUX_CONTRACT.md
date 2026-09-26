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
- close a Session only through an explicit, confirmed destructive action

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

`window-size largest` only protects the shared window while a desktop client is
attached: tmux sizes the window to the largest *attached* client. When the phone
is the only client, its own viewport becomes that largest client and the window
legitimately shrinks to phone size. mctrl must not build any feature on the
assumption that the pane keeps a desktop size.

## Full Session overview size

The phone's FULL overview must not be defined as "the pane's current size",
because that size is phone-driven whenever no desktop client is attached and
FULL would collapse into LOCAL.

FULL asks tmux for:

```text
max(configured full size, current pane size)
```

The configured size is `terminal_full_size` (default `240×60`) and is the same
value a Managed Work pane is launched at and the initial size of a disposable
attach PTY, so the three cannot drift. A pane larger than the configured size
stays authoritative because a desktop client asked for it.

The default is deliberately close to a real desktop terminal: programs lay
themselves out by width and drop panels below their own thresholds, so a narrow
"full" view shows strictly less than the desktop. It is configurable from
Settings because mctrl cannot know a program's threshold.

`GET /api/v1/host` advertises the canonical size as `terminal_full_size`.

Every client-requested resize is clamped to 20–500 columns and 10–200 rows before
it reaches a PTY, so a phone cannot grow or collapse the shared window with an
unbounded value.

## Raw keyboard transport

Every terminal transport mctrl controls must be a raw terminal. That includes
the disposable attach PTY and the tmux pane PTY of a Managed Work Session.

A cooked line discipline silently rewrites the user's typing:

- `ICANON` holds keystrokes in the kernel until Return, so the program sees
  nothing while the user types.
- `ICRNL` rewrites Return into a line feed, so a program that binds Return to
  "submit" inserts a line break instead.
- `ECHO`/`ECHOCTL` prints a second, caret-annotated copy of the input over the
  program's own output.

tmux creates a pane with whatever line discipline it was born with and does not
restore raw mode when a client attaches, detaches, or resizes. Therefore:

- a Managed Work runner must put its pane into raw mode at startup and re-assert
  it while the Work runs;
- the terminal attachment must verify the pane's mode on attach, repair it, and
  report `INPUT_MODE_REPAIRED` when it changed something.

mctrl reconfigures a terminal only when it owns the pane: the Session hosts
non-terminal Managed Work, or the pane's process is `mctrl-runner`. A terminal
that belongs to the user's own desktop client keeps that client's settings.

## Detach semantics

Closing the phone attachment must detach only that client.

It must not:

- kill pane,
- kill window,
- kill session,
- send unintended EOF to the user's durable shell.

## Close semantics

Leaving the phone Terminal page detaches only the phone client. It must never
kill a pane, window, or Session.

`Close Session` is a separate explicit action. It may terminate the Session and
its child processes, including external tmux Sessions, but it must not be
triggered by Project unregistration or daemon uninstall. Active Managed Work
requires an additional force/confirmation step.

## Session disappearance

If the target Session is gone:

- stop attachment,
- emit SESSION_GONE,
- return UI to a safe page.

## Contract tests

All adapter behavior must be verified against the tmux version(s) supported by V1.

Do not rely on guessed command formatting.
