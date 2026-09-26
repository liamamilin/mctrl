# UX Specification

## Pages

1. Home
2. Start Work
3. Session Detail
4. Terminal
5. Settings
6. Pair

## Home

```text
┌──────────────────────────────┐
│ My MacBook              ●    │
│ Remote Ready                 │
│                              │
│ ＋ Start Work                │
│                              │
│ Sessions                     │
│                              │
│ personal-control             │
│ opencode                     │
│ ~/Projects/personal-control  │
│                              │
│ Running browser tests...     │
│ PASS B03                     │
│                         >    │
└──────────────────────────────┘
```

## Transport indicator

Trusted-LAN HTTP mode must be visually identifiable in Settings and setup.

Do not use a misleading "Secure" badge.

## Start Work

Fields:

- Project
- Runner
- multiline Prompt

The user does not see:

- tmux arguments
- PTY configuration
- command-line internals

## Session Detail

Show facts:

- Session name
- active command
- cwd
- windows/panes
- recent output
- Managed Work process facts if associated
- Open Terminal

## Terminal

```text
┌─────────────────────────────┐
│ ← Session                   │
├─────────────────────────────┤
│ xterm.js                    │
├─────────────────────────────┤
│ ESC  TAB  CTRL  LOCAL  编辑 │
│ ↑   ↓   ←   →    ⏎         │
├─────────────────────────────┤
│ [Prompt textarea]    Send   │
└─────────────────────────────┘
```

**编辑** raises the software keyboard on the terminal itself. **⏎** sends the
same Return the keyboard sends, so submitting never depends on which Return the
phone shows.

The Session title shows the tmux session **name**, never the bare id (`$0`, `$1`).

## LOCAL and FULL

`LOCAL` sizes the terminal to the phone viewport, so the program re-renders for
the phone and stays readable. `FULL` shows the whole Session at once, scaled
down to fit, and is labelled with the size it is showing (`120×40`).

`FULL` asks the Mac for `max(120×40, current pane size)` instead of reading the
pane back, because the phone's own `LOCAL` size is what shrinks the shared window
when no desktop client is attached. Without that rule `FULL` and `LOCAL` would be
identical and `FULL` would have nothing to show. See `TMUX_CONTRACT.md`.

## Reconnect

```text
Connection lost.
Your session is still running on the Mac.

Reconnecting...
```

Disable terminal input while disconnected.

## Prompt uncertainty

If a structured Prompt send cannot determine whether it was delivered, do not silently resend.

Show a factual warning and let the user inspect output.

## Offline

Use:

```text
Mac is currently unreachable.
```

Do not claim:

```text
Mac is sleeping.
```

unless future infrastructure proves it.
