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
│ ↑   ↓   ←   →   ⏎  PgUp PgDn│
├─────────────────────────────┤
│ [Prompt textarea]    Send   │
└─────────────────────────────┘
```

**编辑** raises the software keyboard on the terminal itself. **⏎** sends the
same Return the keyboard sends, so submitting never depends on which Return the
phone shows.

The keybar order is `LOCAL` → **编辑** → `↑ ↓ ← →`, and nothing is inserted into
it. **PgUp** and **PgDn** follow the arrows because they are not cursor movement:
they page whatever view the running program keeps for itself.

### What a Chinese keyboard can and cannot send

The phone keyboard has digits and symbols. The failures are narrower than they
look, and they have different causes.

**Digits never reach the page.** On a Chinese keyboard the number row is the
candidate selector, so `1`–`5` choose a word instead of typing a digit. That
decision is made by the operating system, outside the page, and no terminal-side
handling recovers it. Switching to a Latin keyboard is the only native way to
type a digit.

**Digits and punctuation cannot be sent from a Chinese keyboard, and mctrl
cannot fix that.** Measured on the phone by recording every frame the browser
sends to the Mac: a Chinese keyboard delivers letters (`a` → `0x61`) and Return
(`0x0d`), and delivers *nothing at all* for the digit row or for punctuation. No
DOM event is produced, so there is nothing for the page to intercept. The same
keys on an English keyboard arrive normally (`1` → `0x31`, `,` → `0x2C`), which is
why switching keyboards works. The digit row of a Chinese keyboard is the
candidate selector; that is the operating system's design.

Two wrong conclusions came out of this before it was measured, and are recorded
because they are the trap: reading xterm's source suggested the character was
delivered and then discarded, and an interception built on that changed nothing
on the phone. Reading a library's control flow tells you what a browser *would*
do with an event, not what iOS *delivers*.

**Width is still corrected for text that does arrive.** A Chinese keyboard with
an English layout, a paste, or a hardware keyboard produce real characters, and
they arrive full-width or as CJK punctuation. A program that binds `.` never sees
`。`. Converted, and only where there is one unambiguous ASCII equivalent:

| From | To |
| --- | --- |
| U+FF01–U+FF5E full-width forms | ASCII `0x21`–`0x7E` |
| U+3000 ideographic space | space |
| U+3001 `、` | `,` |
| U+3002 `。` | `.` |
| U+2018–U+201D curly quotes | `'` and `"` |

Left exactly as typed: CJK text, CJK brackets such as `「」【】《》`, `—`, `…`, `〃`,
and the circled digits a long press produces. Those are characters, not width
errors. Checked by `web/scripts/check-normalizer.mjs`, which runs in `make
test-web`.

The conversion applies to terminal keystrokes only, never to the Structured
Prompt. It is unconditional; to send the raw bytes, set
`mctrl-terminal-half-width` to `off` in the page's `localStorage`.

### Reaching earlier output

There are three different kinds of "earlier output", and they need three
different mechanisms. Confusing them is why this felt broken.

**1. Terminal scrollback — for shell output.** The terminal keeps 5000 lines, and
on attach the Mac replays the pane's own tmux history into it, up to 500 lines.
Without that replay the phone could only ever scroll to output the phone itself
caused, because a fresh attach carries the visible grid alone — which is why the
Mac could scroll and the phone could not, even though looking at the same pane.
When the viewport leaves the live tail, a **↓ N lines back** button appears; one
tap returns to the tail. `FULL` does not need it.

**2. The program's own view — for a TUI's conversation.** A full-screen program
repaints the same rows, so its own history never reaches the terminal's
scrollback at all. No amount of replay or scrollback will show it. The program
keeps that history itself and pages through it with **PgUp** / **PgDn** — which is
why those keys are on the keybar. OpenCode, for example, binds
`messages_page_up` to `pageup` and `messages_page_down` to `pagedown`; without
them the phone simply could not ask for a TUI's history, however much the terminal
was holding.

**3. The application's store — for everything, in full.** An agent's complete
transcript is not in the terminal at all. It is in the agent's own session store
and can be read directly:

```sh
cd <project> && opencode session list
cd <project> && opencode session export <session-id> > /tmp/session.json
```

This is the only way to read a conversation end to end, and the only way to reach
anything older than the program's own scrollback.

tmux keeps 2000 lines per pane, so 500 is a deliberate cap for what a phone can
usefully scroll, not a limit of the Session. See TMUX_CONTRACT.md.

A dropped connection re-attaches, and every attachment is a fresh
`tmux attach-session` that replays the history again. The client therefore resets
its terminal when it re-attaches, so the scrollback is always exactly one replay
plus what has happened since, never two copies of the same output stacked up.

### Software keyboard

iOS keeps the layout viewport at full height when the software keyboard opens, so
a terminal sized to the layout viewport ends up behind the keyboard. The page
measures the gap between the two viewports and hands it to the layout as
`--keyboard-inset`, which lifts the keybar above the keyboard. Values that look
like rounding noise or a pinch-zoom are ignored.

The Session title shows the tmux session **name**, never the bare id (`$0`, `$1`),
and wraps to two lines instead of truncating without a hint.

## Windows and panes

A Session with more than one window or pane shows only the active window's active
pane in the terminal, and the Session detail page lists the rest as a read-only
inventory: every window, every pane, and which one you are looking at.

The inventory has no switch, on purpose. A tmux client cannot hold a window of its
own, so switching from the phone would drag the Mac's view along with it. See
TMUX_CONTRACT.md for the measurements.

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
