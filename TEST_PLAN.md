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

## Browser E2E

Playwright:

- Chromium
- WebKit mandatory

Required:

- pair
- Mac Pair Console renders QR, copyable code and manual phone URL
- Pair Console QR endpoint rejects non-loopback requests
- re-pair same browser installation reuses one active device record
- revoked history is absent from Settings
- create a two-minute one-time Safari link
- link Safari under the same device record and reject token reuse
- auth rejection
- Origin rejection
- Home
- Session preview
- Terminal
- stale CONNECTING socket is replaced after resume
- FULL overview fits, zooms and pans with one finger
- FULL mode can edit through the explicit 编辑 control immediately after LOCAL
- the terminal keybar exposes working up, down, left and right arrow controls
- the terminal keybar's ⏎ control sends Return unchanged
- FULL asks the Mac for max(canonical full size, current pane size) and shows the
  whole Session scaled to fit
- FULL still works when the phone is the only attached client and the shared
  window has already shrunk to phone size
- the overview label reports the size being shown, and the Session title shows the
  session name rather than a bare tmux id
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
