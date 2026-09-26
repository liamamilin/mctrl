# Troubleshooting

Start with:

```sh
mctrl status
mctrl doctor
mctrl logs -n 120
```

These commands do not stop tmux Sessions or Managed Work.

## The phone cannot reach the daemon

- Confirm `mctrl status` reports the daemon running.
- Confirm the Mac and phone are on the same reachable network.
- Remember that the default listener is `127.0.0.1`; use `mctrl setup --lan` only
  when Trusted-LAN HTTP is intentionally acceptable.
- On macOS, check the firewall if another local process can connect but the phone
  cannot.

## The Mac pairing launcher does not open the Pair Console

Run `make mac-launcher`, then double-click `dist/Mctrl Pair.app`. For V2, run
`make mac-launcher PROFILE=v2` and use `dist/Mctrl V2 Pair.app`; it invokes
`mctrl --profile v2 pair --open`. The launcher requires an initialized, running
profile-specific daemon:

```sh
./bin/mctrl setup --lan
./bin/mctrl status
```

It intentionally does not enable LAN mode automatically. The equivalent command
is `./bin/mctrl pair --open`. The bundle is a non-resident launcher, not a
menu-bar daemon or native management UI. If the phone cannot scan, use the
visible Pairing code and manual Pair address in the Console.

## The status API belongs to another process

If the port is already occupied, `setup` can report that mctrl started while a
different daemon answers locally. Confirm with:

```sh
lsof -nP -iTCP:<port> -sTCP:LISTEN
curl -i http://127.0.0.1:<port>/healthz
```

Choose a different port without stopping the existing service, for example:

```sh
mctrl setup --lan --port 17681
mctrl status
```

A real mctrl `/healthz` response contains JSON with `"status":"ok"` and a
`"profile"` identity. `mctrl status` rejects a reachable daemon from another
profile. For V2, inspect `mctrl --profile v2 status` and use port `17682` unless
an explicit port override was configured.

V2 uses a separate tmux socket, so it does not list V1/default desktop Sessions
during development. This is intentional. At cutover, stop V1 before explicitly
pointing V2 at the V1 socket; never run both writers against one socket.

## Pairing succeeds but the phone is immediately unauthorized

- Run `mctrl devices` and confirm the device is active.
- If it was revoked, pair again with a new short-lived token.
- Pairing tokens are one-time and expire; run `mctrl pair` for a new window.
- In TLS mode, confirm `public_url` is the exact HTTPS origin used by the phone.
- iOS standalone PWAs and Safari may use separate cookie/storage containers. A
  PWA can be paired while Safari still shows Pair; pair each browser context once.

## Settings shows a revoked pairing history or repeated login is required

Settings intentionally lists active device records only. Revoked records remain
available in the collapsed **Pairing history** section, in
`~/.mctrl/devices.json`, and in `mctrl devices`; they are not counted as active
paired devices.

A browser normally stays paired for 30 days. Re-pairing the same browser
installation reuses its device ID and rotates its credentials. If browser storage
was cleared, the old active cookie may be gone and the new installation cannot be
proved to be the same context; revoke the old Settings record after pairing the
new one. Safari and an installed PWA are separate contexts by default.

To use one device record in both contexts, open **Settings → Link another
browser** in the already paired PWA. Copy the generated two-minute link, paste
it into Safari, and confirm **Link this browser**. The link is one-time and
carries the same terminal privilege as pairing, so only create it immediately
before use.

## TLS mode advertises the wrong pairing URL

Run setup with the browser-facing origin:

```sh
mctrl setup --transport tls_terminated --public-url https://mctrl.example.internal
mctrl pair
```

`public_url` must be an origin such as `https://host[:port]`, without `/pair`.

## `RUNNER_NOT_FOUND` or the Work will not launch

- Run `mctrl doctor` and check that `mctrl-runner` is beside `mctrl`.
- Build both binaries with `make build`.
- Codex/OpenCode Work also requires the selected executable to be installed and
  authenticated.
- A registered Project path must still exist and be a directory.

## Work reports recovery uncertainty

An uncertain state means mctrl lacks one authoritative exit or identity fact. It
does not mean the task semantically failed. Inspect the Session and the Work's
`recovery_status`, `termination_reason`, and `launch_stage`. Do not blindly resend
a Prompt.

## A Work exited; how do I run it again?

A dead pane is not restarted in place. V1 creates a new Work/attempt/Session for a
new launch and retains the old Session as evidence. Use **Start another Work** on
the terminal Work card; the original Project is preselected when it still exists.

## A Work references a Project that no longer exists

`mctrl doctor` reports this as a dangling reference:

```text
Dangling:      Work work_xxx references unknown Project project-yyy
  restore with: mctrl project restore --id project-yyy <registered path>
```

A Project was unregistered while that Work still pointed at it. Do **not** re-add
it with `mctrl project add`: that mints a new id, so the Work's history would
reference an id that will never exist again. Restore the original id instead, with
the same path the Work recorded:

```sh
mctrl project restore --id project-yyy <path>
```

The path must still exist. Restore is refused if the id is already taken or the
path belongs to a different Project. Finished Work is history and is not
reported; only a live Work needs this.

## Terminal attachment fails

- Confirm the tmux Session still exists in `mctrl status`.
- Close other mobile attachments if needed and reconnect from the Session page.
- `SESSION_GONE` is factual: the target Session no longer exists.
- Other attach errors leave the tmux Session intact; mctrl never kills it merely to
  close a phone attachment.
- If iOS resume leaves a socket in `CONNECTING`, wait for the five-second timeout
  or tap Reconnect. Returning to the page replaces the stale socket rather than
  queueing input.
- If the API reports zero Sessions only under launchd, inspect `mctrl logs`. The
  macOS adapter uses `/private/tmp/tmux-<uid>/default` by default;
  `MCTRL_TMUX_SOCKET` can override it for a deliberately custom tmux endpoint.
- If CJK output is replaced by placeholders, confirm the tmux client flags include
  `UTF-8`. mctrl forces UTF-8 for phone attachments and for its LaunchAgent.

## Typed keys only appear after Return, and Return inserts a line break

The Session's keyboard transport is line-buffered on the Mac. A cooked terminal
holds keystrokes until Return, rewrites Return into a line feed, and echoes a
caret-annotated copy of the input over the program's own output.

mctrl repairs this when it attaches to a Session that hosts Managed Work and
reports `INPUT_MODE_REPAIRED` in the Terminal page. Reconnect the Terminal page
once after upgrading, or start new Managed Work: a runner started before the fix
cannot repair its own pane.

To confirm the pane's mode directly:

```sh
tmux -S /private/tmp/tmux-$(id -u)/default display-message -p -t <session> '#{pane_tty}'
stty -a -f /dev/ttysNNN
```

A raw pane shows `-icanon`, `-echo`, and `-icrnl`. mctrl never changes the
terminal of a Session that does not host Managed Work, because that terminal
belongs to the user's own desktop client.

## A large desktop Session shows only one corner on the phone

`LOCAL` intentionally uses the phone viewport, while tmux keeps the larger desktop
window authoritative. Use **FULL** to fit the complete pane, pinch to zoom, and use
one-finger drag to pan the two-dimensional viewport. FULL remains editable through
the explicit **编辑** control after LOCAL; Structured Prompt stays in LOCAL. Exiting FULL
restores the readable interactive viewport without changing tmux's desktop size.

## The phone shrinks or changes the desktop tmux layout

Disconnect the mobile attachment. mctrl sets tmux's `window-size` policy to
`largest`, but the final behavior depends on the installed tmux version and the
desktop client's own settings. Verify with both clients attached before release.

Note what `largest` actually means: tmux sizes the window to the largest
**attached** client. While a desktop client is attached, the phone cannot shrink
the window. When the phone is the only client, the phone's own viewport becomes
that largest client and the window follows it down to phone size. That is
expected tmux behavior, not a bug, and it is why the phone's `FULL` overview asks
for the Mac's canonical `240×60` (configurable) instead of reading the pane's
current size.

## The Chinese keyboard cannot type numbers or symbols into a program

**This is not fixable in the terminal, and the cause is measured rather than
guessed.** With the daemon recording every input frame the phone sends:

| Key | Chinese keyboard | English keyboard |
| --- | --- | --- |
| `a` | `hex=61` arrives | `hex=61` arrives |
| Return | `hex=0d` arrives | `hex=0d` arrives |
| `1` | **nothing** | `hex=31` arrives |
| `，` | **nothing** | `hex=2c` arrives |

The transport is sound: every character the browser produces reaches the Mac. A
Chinese keyboard simply produces no event at all for the digit row or for
punctuation, so there is nothing for the page to intercept, forward, or convert.
On a Chinese keyboard the number row is the candidate selector, which is the
operating system's design.

Switch to a Latin keyboard to type digits and punctuation. If the character
appears on screen but the program ignores it, that is the program's input
handling, not mctrl: use a shell rather than a full-screen TUI to tell them
apart.

Two wrong conclusions came out of this before it was measured. Reading xterm's
source showed the character being delivered and then discarded, and an
interception built on that reading changed nothing on the phone. The lesson is
recorded because it is the trap: reading a library's control flow tells you what
a browser *would* do with an event, not what iOS *delivers*. Measure at the
boundary the data actually crosses.

To confirm the full-width conversion is what you are seeing, set
`mctrl-terminal-half-width` to `off` in the page's `localStorage` and reload; the
raw full-width characters then reach the program unchanged.

CJK text itself is never altered by the conversion. What it does cover, beyond
the full-width block, is the ideographic punctuation a Chinese keyboard sends
when an English layout is active or when text is pasted: `。` (U+3002) and `、`
(U+3001) become `.` and `,`, which the previous version missed because those
codes sit below U+FF01. Curly quotes are straightened too. `web/scripts/check-normalizer.mjs`
covers all of this and runs in `make test-web`.

## The software keyboard covers the terminal

The page measures the gap between the layout viewport and the visual viewport and
lifts the keybar above the keyboard. If the keyboard still covers the terminal:

- reload the page once; the versioned service worker swaps the cached shell on
  the next load, and `index.html` is fetched network-first
- rotate the phone once, which forces a fresh visual-viewport measurement
- a value that looks like rounding noise or a pinch-zoom is deliberately ignored,
  so a half-open keyboard that reports an implausible gap is left alone

## Only one window or pane is visible on the phone

Expected. mctrl attaches to the Session's active window and its active pane, and
the Session detail page lists every window and pane as a read-only inventory so
you can see what else exists.

There is no switch, deliberately. Measured on tmux 3.7c:

- `attach-session -t session:window` reaches that window but moves the Session's
  active window, and the desktop client follows it
- `attach-session -t session:window.pane` attaches to the whole window, not one
  pane
- `switch-client -c <client> -t <target>` succeeds but moves every client

A tmux client cannot hold a window of its own, so a switch from the phone would
change the desktop's view. Since the desktop has priority, mctrl shows the facts
and leaves the switching to the Mac. See TMUX_CONTRACT.md.

## Earlier output cannot be found

Three different things get called "history", and each needs its own answer.

**Shell output** — reachable. The terminal keeps 5000 lines and the Mac replays
the pane's tmux history on attach, up to 500 lines. `↓ N lines back` returns to
the live tail. To go further back, raise tmux's limit and re-attach:

```sh
tmux set-option -g history-limit 5000
```

**A TUI's conversation** — not in the terminal at all. A full-screen program
repaints the same rows, so its history never enters the terminal's scrollback.
Use the program's own keys: **PgUp** and **PgDn** on mctrl's keybar send
`ESC [ 5 ~` and `ESC [ 6 ~`, which is what OpenCode binds to
`messages_page_up` / `messages_page_down`. If the program binds scrolling to
something else, it is that program's keybind configuration, not mctrl.

**The complete transcript** — in the agent's store, not the terminal:

```sh
cd <project> && opencode session list
cd <project> && opencode session export <session-id> > /tmp/session.json
```

This is the only way to read a conversation end to end, and the only way past
what the program's own scrollback holds.

If scrolling a shell shows nothing after a reload, check the Session is not a TUI
before suspecting mctrl.

## Remote Ready is false

- `mctrl status` shows the configured availability policy.
- `on_ac` requires AC power and a live daemon host assertion.
- `work_only` requires a live Work assertion.
- `always` uses a daemon-owned assertion.
- mctrl prevents ordinary idle sleep only; lid close, explicit Sleep, shutdown,
  thermal and low-power behavior remain outside the guarantee.

## Inspect Work without stopping it

- Session preview: Home or Session detail.
- Full terminal: Session detail → Open terminal.
- Persisted facts: `GET /api/v1/work` through the authenticated PWA/API.
- Runner output: `~/.mctrl/logs/runner/`.
- Daemon output: `mctrl logs`.

Do not kill `mctrl-runner` merely to make a stuck state disappear. Its disappearance
before an exit result is recorded is treated as uncertain recovery.
