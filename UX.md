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
│ xterm.js        [↓ 120 back] │
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

The keybar order is `LOCAL` → **编辑** → `↑ ↓ ← →`, and nothing else is added:
the phone's own keyboard already carries digits and symbols.

### What a Chinese keyboard can and cannot send

Two different failures look like one from the phone:

- **Digits do nothing.** On a Chinese keyboard the number row is the candidate
  selector, so those keys select a word instead of typing a digit, and the
  keystroke never reaches the page. This is the operating system's design; no
  amount of terminal-side handling recovers it. Switching to a Latin keyboard is
  the only native way to type a digit.
- **Punctuation arrives, but full-width.** `：`, `，`, `／` are committed text, so
  they do reach the terminal, and a program that binds `:`, `,` or `/` never sees
  them. mctrl converts the full-width ASCII block (U+FF01–U+FF5E) and the
  ideographic space to half-width before sending, so real CJK input is untouched.
  The conversion applies to terminal keystrokes only, never to the Structured
  Prompt, where a program reads text rather than keys.

The conversion is unconditional. To send the raw bytes back, set
`mctrl-terminal-half-width` to `off` in the page's `localStorage`.

### Reaching earlier output

The terminal keeps 5000 lines of scrollback, so shell output is reachable by
scrolling. When the viewport leaves the live tail, a **↓ N lines back** button
appears over the canvas; one tap returns to the tail.

`FULL` does not need it: it shows the whole Session at once.

### Software keyboard

iOS keeps the layout viewport at full height when the software keyboard opens, so
a terminal sized to the layout viewport ends up behind the keyboard. The page
measures the gap between the two viewports and hands it to the layout as
`--keyboard-inset`, which lifts the keybar above the keyboard. Values that look
like rounding noise or a pinch-zoom are ignored.

The Session title shows the tmux session **name**, never the bare id (`$0`, `$1`),
and wraps to two lines instead of truncating without a hint.

## LOCAL and FULL

`LOCAL` sizes the terminal to the phone viewport, so the program re-renders for
the phone and stays readable. `FULL` shows the whole Session at once, scaled
down to fit, and is labelled with the size it is showing (`240×60`).

`FULL` asks the Mac for `max(terminal_full_size, current pane size)` instead of
reading the pane back, because the phone's own `LOCAL` size is what shrinks the
shared window when no desktop client is attached. Without that rule `FULL` and
`LOCAL` would be identical and `FULL` would have nothing to show. See
`TMUX_CONTRACT.md`.

The size is editable in **Settings → Full Session size**. Widen it if a program
still drops panels, because programs lay themselves out by width and mctrl cannot
know their thresholds. See `CONFIG_SCHEMA.md`.

`FULL` hides the Structured Prompt dock, because a whole-Session view does not
need it and the dock costs about 90px. The one exception is an unconfirmed
Prompt: while a send is uncertain, the dock stays visible, because "Clear
prompt" is the only way out of that state.

The `FULL` hint is a single line (`Full 240×60 · Editable · pinch · pan`).
A hint that wraps into three lines costs more terminal than the instruction is
worth, so "editable" stays in the shortened form: `FULL` is still a working
terminal, not a read-only view.

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
