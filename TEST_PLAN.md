# Test Plan

## Unit

- config validation
- transport-profile validation
- Project validation
- pairing token generation
- persistent browser installation identity
- same-installation pairing reuses/revives one device record
- credential hashing/session auth
- Origin policy
- CSRF policy
- Work transitions
- request idempotency
- tmux parser
- error mapping
- recovery decisions
- macOS Pair launcher bundle and ad-hoc code-signature verification

## tmux integration

Use real tmux.

Required:

- discover external Session
- multiple Sessions
- active pane cwd
- registered-child directory follows cd, preserves literal path text, and rejects unverified/archived processes
- pane bracketed-paste mode distinguishes program capability from client framing
- current command
- capture preview
- attach/detach
- Session survives detach
- mobile attach while desktop attached
- desktop layout remains stable
- launchd-managed daemon discovers the same tmux Sessions as the CLI
- attach client is marked UTF-8 and renders CJK text
- large desktop Session can be viewed on the phone without resizing tmux
- Session disappearance behavior
- managed Session naming collision

## Work integration

Required:

- Shell Work launch
- child exit 0
- child exit nonzero
- daemon restart during RUNNING
- daemon restart during STARTING
- runner missing recovery
- duplicate request_id
- prompt delivery failure
- no prompt replay after uncertain recovery
- archival rejects live Session/Runner/child and uncertain observations
- archival preserves unknown exit facts, request-id mapping and history
- archived records no longer block Project unregister
- late canonical exit results still apply to archived history
- old Launch retries and stale launch files cannot revive an archived record
- CLI archival requires confirmation and rejects another profile's state root

## Browser E2E

Playwright:

- Chromium
- WebKit mandatory

Implemented automated regression cases are in `web/tests`. Run
`make test-browser`; set `MCTRL_LIVE_RUNNERS=1` to include installed Agent
interactive-startup probes. The remaining items below are the broader acceptance
matrix, including checks that still require a physical iPhone.

Required:

- first Project registration opens its section and returns to the selected directory
- a selected Project starts the Shell in its displayed directory
- native output/path selection and copy fallback work without Clipboard API
- selected directory text prepares a correctly quoted cd draft without executing
- draft insertion preserves Unicode/quotes, adds no Return, and is never replayed
- a client with paste framing but a program without it keeps multiline drafts blocked
- a supporting program receives one framed multiline payload; Return is separate
- pending startup facts block raw keys and drafts without queuing them
- switching Sessions creates fresh terminal/input state
- desktop tmux Sessions can be continued without registering or launching Work
- native desktop PTY runs here/resume, keeps the same Session and Shell PID, and refuses a changed name
- same directory reuses its Session; different directories with the same basename stay separate
- private desktop connection hint matches the daemon's custom socket and selected profile
- desktop start/return command copy does not execute or create Work from the phone
- recent Session identities are checked against the live list before continuation
- harness launch mode explicitly separates interactive startup from a required task Prompt
- interactive launch opens Terminal; task launch opens Session output
- accepted Work is followed using GET only; rapid taps create one launch request
- leaving the launch page prevents late Session navigation; observed failure never retries automatically
- output transfer replaces the draft selection and restores focus without sending input
- previews follow latest content, preserve the selected snapshot during updates, and omit trailing blank pane rows
- foreground return refreshes Home/Session/Terminal facts without launching or replaying input
- output is visible before folded process/termination details on mobile dimensions
- physical iPhone selection handles, native Copy menu, IME composition, and draft-to-terminal keyboard switching
- pair
- Mac Pair Console renders QR, copyable code and manual phone URL
- LAN pairing keeps the Bonjour origin across DHCP IP changes and refreshes the numeric fallback
- explicit/public/TLS/loopback origins retain their address policy without an unintended LAN fallback
- QR and manual backup share one expiring, single-use code; QR reads do not consume it
- moving a numeric-IP Home Screen entry to the hostname requires pairing at the new browser origin
- Pair Console QR endpoint rejects non-loopback requests
- re-pair same browser installation reuses one active device record
- revoked history is absent from Settings
- create a two-minute one-time Safari link
- link Safari under the same device record and reject token reuse
- auth rejection
- Origin rejection
- Home
- Session preview
- Session detail lists every window and pane, marks the one the phone is showing,
  and offers no window switch
- the Home card shows a Managed Work state chip, and an External tmux Session has
  no chip
- Session detail loads 100 preview lines and grows to at most 500 on request
- Terminal
- stale CONNECTING socket is replaced after resume
- FULL overview fits, zooms and pans with one finger
- the keybar keeps the home-indicator inset in FULL, where the Prompt dock is
  hidden and the keybar is the bottom element
- Start Work states why the launch button is disabled
- Start Work launches with an empty Prompt, matching what the API accepts and what
  every runner guards
- unregistering a Project and revoking a device both confirm inline, never through
  window.confirm
- a Project with active Managed Work says so on its card before the button is
  pressed
- a refused unregister keeps the confirmation open and states the reason inside it
- a successful unregister and revoke are confirmed next to the list, not at the
  top of the section
- an expired browser link says it has expired rather than counting down to 0:00
- the Prompt dock carries no permanent mechanism footnote
- FULL mode can edit through the explicit 编辑 control immediately after LOCAL
- the terminal keybar exposes working up, down, left and right arrow controls
- the terminal keybar's ⏎ control sends Return unchanged
- the terminal keybar's PgUp and PgDn send `ESC [ 5 ~` and `ESC [ 6 ~`, and a
  full-screen program's own history pages with them from the phone
- 编辑 focuses the visible native text editor in the same gesture in LOCAL/FULL
- native composition keeps candidates local and blocks Insert/Return until confirmed
- confirmed Chinese, digits and punctuation are inserted once, literally, without implicit Return
- closing/reopening the FULL editor preserves its unsent draft and canonical terminal size
- reconnect retains the draft without queueing or replaying it
- physical iPhone Chinese keyboard: candidate commit, 123/symbol layout, delete,
  keyboard switching, and native editor focus; daemon bytes alone cannot establish DOM events
- `。` and `、` are converted to `.` and `,` when text does arrive in that form,
  and CJK brackets, `—`, `…` and circled digits are left alone
- `web/scripts/check-normalizer.mjs` passes in `make test-web`
- a Latin keyboard's letters, digits and punctuation are each sent exactly once
  on the phone and in a desktop browser
- an armed CTRL still modifies a character typed on the software keyboard
- scrolling back shows the ↓ lines-back control, and one tap returns to the tail
- attaching to a Session with existing pane history shows that history in the
  phone's scrollback, including lines that had already scrolled off the pane's
  visible screen before the phone connected
- after a dropped connection and re-attach, the scrollback holds one copy of the
  history rather than one per connection
- the software keyboard does not cover the terminal, and the keybar stays above it
- the FULL hint stays on one line
- FULL hides the draft dock by default; 编辑 reveals native editing, and uncertain Prompt notices remain reachable; structured Shell send stays LOCAL
- a long Session name wraps to two lines instead of being silently truncated
- FULL asks the Mac for max(canonical full size, current pane size) and shows the
  whole Session scaled to fit
- FULL still works when the phone is the only attached client and the shared
  window has already shrunk to phone size
- the overview label reports the size being shown, and the Session title shows the
  session name rather than a bare tmux id
- Settings changes the full Session size, it persists, and FULL uses it on the
  next toggle without a daemon restart
- keys reach a Managed Work program one at a time, without pressing Return first
- Return submits in the program instead of inserting a line break
- the input is not echoed twice over the program's own output
- LOCAL mode restores the readable interactive viewport
- reconnect banner
- Start Work
- terminal Work card offers Start another Work with Project preselection
- duplicate POST
- device revoke
- V1 and V2 profiles keep state, ports, LaunchAgents, cookies, Pair apps, and tmux sockets separate
- V2 refuses to open a V1-marked state root
- V1/V2 Pair apps coexist and V2 invokes `--profile v2`

## Transport tests

Trusted-LAN mode:

- warning/config state visible
- secure claims absent
- auth still required

TLS-terminated mode:

- secure cookie/session behavior works
- WSS terminal works

## Real iPhone smoke test

Release candidate must test:

```text
Pair
→ add to Home Screen
→ confirm standalone PWA and Safari may require separate one-time pairing
→ observe existing large desktop Session
→ confirm UTF-8 CJK output
→ open LOCAL Terminal
→ type command
→ enter FULL overview
→ pinch zoom and one-finger pan
→ edit from FULL with the 编辑 control after LOCAL
→ exercise ↑, ↓, ← and → without leaving the terminal
→ return to LOCAL
→ lock iPhone
→ unlock/reopen
→ replace stale CONNECTING socket and reconnect automatically
→ Session still alive
→ launch Shell Managed Work
→ lock iPhone
→ display sleeps
→ Work and caffeinate continue
→ reopen
→ observe output
→ restart launchd daemon
→ Work survives with the same Session/Runner identity
→ send structured exit
→ confirm EXITED/exit_code=0 and Session retention
→ use Start another Work and confirm Project preselection
→ open Settings and verify one active device plus collapsed revoked history
→ create a one-time Safari link and confirm both browser contexts share the device
```

## Failure-path release gates

Must pass:

1. browser disconnect never kills Work
2. daemon restart never intentionally kills Work
3. raw terminal input never replays
4. Launch request idempotency survives restart
5. unknown Prompt delivery is never silently repeated
6. mobile attach does not shrink desktop layout
7. power assertion releases after actual child exit
8. revocation blocks subsequent control
9. Session persists after Work exit
10. security profile displayed accurately
11. launchd daemon and foreground daemon enumerate the same tmux Sessions
12. phone FULL navigation does not shrink the desktop tmux window
13. exiting FULL restores LOCAL input and a phone-sized viewport
