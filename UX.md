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
