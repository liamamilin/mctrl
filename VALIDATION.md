# Validation Record

## Stable LAN phone entry after DHCP changes — 2026-10-09

Reported failure: the phone still opened `192.168.2.105:17681`, while the Mac's
DHCP address was now `192.168.2.226`. The running daemon, page and assets were
healthy; both new external tmux Sessions were discoverable.

Implemented: ordinary Trusted-LAN wildcard listeners now prefer macOS's
Bonjour `LocalHostName.local`. The CLI status separates the preferred browser
Address from the bind Listener and current numeric LAN fallback. Pair QR and
manual address share this preferred origin. The Mac Console offers a copyable
IP backup using the same one-time code, and Refresh updates that backup. Address
fields have a full row; copy targets are at least 44px. Explicit public origins,
explicit binds, loopback and TLS are preserved. No auth or tmux boundary changed.

Validation:

- `.tools/go1.27.1/bin/go test ./internal/config ./internal/api ./cmd/mctrl -run
  'TestLANPairing|TestPairAddress|TestLocalHostname|TestPairQR|TestPairConsole|TestAdvertisedPair|TestPairingConsole'
  -count=1`: passed. A simulated `.105` → `.226` address change retains the
  preferred name; IPv6/loopback/custom public/TLS cases and invalid names pass.
  QR bytes encode the preferred origin, QR reads do not consume the token, and
  manual pairing through the backup consumes that same single-use window.
- `make release-check`: passed on the final build, including Go tests/vet/race,
  real tmux/PTY coverage, frontend typecheck/build, normalizer, CLI smoke,
  Pair.app signature, embedded resources and binary checksums.
- `npm --prefix web run test:browser -- pairing.spec.ts`: **4 passed**, two per
  Chromium/WebKit. Covers stable manual URL, refreshed backup IP, native copy
  fallback, loopback compatibility and 393 × 852 layout. The displayed backup
  fits without internal horizontal scrolling and copy targets measure 44px.
  Both browser screenshots were visually reviewed; this targeted run adds to
  the previously recorded 48-case workflow baseline, not a fresh full-suite run.
- Deployed version: `1.0.0 / 6b9cb2e-dirty / 2026-10-09T15:48:48Z`. The actual
  daemon QR now advertises `http://MiLinsMacBook.local:17681/pair`, with
  `http://192.168.2.226:17681/pair` as backup. Hostname health/page/SW returned
  200; JS/CSS/SW bytes match the final embedded resources. Pair.app binaries
  match `bin/`. A fresh hostname QR Console was opened without logging its token.
- Deployment retained all eight Work identity/state/archive/exit records, all
  three pane identities/PIDs and four live baseline processes. CLI still reports
  three Sessions and RemoteReady `on_ac`.

Frontend cache: `mctrl-shell-06bfe41d180de77e`; JS: `index-B_UY19SZ.js`.
Local deployment/runtime evidence: `bin/stable-pairing-preview/`; browser
screenshots: `web/test-results/` (temporary fixtures, no live pairing token).

Exit criterion met on the Mac: pairing now uses a stable origin across DHCP
address changes with an explicit backup. Physical-iPhone Bonjour resolution
still needs confirmation on this Wi-Fi. The operator must pair once at the new
origin and replace an old numeric-IP Home Screen entry; origins do not share
browser credentials. Local-name changes or a network blocking Bonjour still
require updating/falling back. No router or Wi-Fi settings were changed.

## Desktop → phone continuation polish — 2026-10-09

Deployed build: `1.0.0 / 34ee1f9-dirty / 2026-10-09T15:02:08Z`.
Earlier uncommitted repairs and both mobile-flow rounds are preserved.

Implemented: `mctrl here` creates or resumes a directory-specific external tmux
Shell using the daemon's recorded connection. Different paths with the same
basename stay separate; the same physical directory retains its Session and
process. It registers no Project, creates no Managed Work, and sends no harness
command. The phone provides a short desktop-start instruction and a per-Session
return command. `mctrl resume` checks the observed name and stable ID before
attaching, retaining the socket/profile. Inside another tmux context it asks for
a new terminal tab rather than switching an ambiguous client. The installer
creates a guarded `~/.local/bin/mctrl` link without editing shell/WezTerm config.

Validation:

- `.tools/go1.27.1/bin/go test ./cmd/mctrl ./internal/tmux ./internal/config`:
  passed. Native PTYs exercise here, detach, repeated here and resume, retaining
  the same Session/Shell PID. Stale names and noninteractive input are refused;
  different directories remain separate. Quoted socket/name arguments, native
  TERM preservation, private connection permissions and profile matching pass.
- `.tools/go1.27.1/bin/go test ./internal/api`: passed. The daemon records its
  actual custom socket and emits matching read-only start/return fields through
  the existing authenticated Host/Session endpoints.
- `MCTRL_LIVE_RUNNERS=1 npm --prefix web run test:browser`: **48 passed**, 24 per
  Chromium/WebKit. New desktop cases cover copy without mutations and phone input
  in the original external Shell without creating Work or replacing its PID.
  All prior durability, input, launch and installed interactive-Agent cases pass.
- Mobile screenshot fixture: **1 passed**, 393 × 852, no horizontal overflow,
  return-copy button height 44px. Both new sections were visually inspected.
- Installer checked in a temporary directory: repeated install succeeds; an
  existing different file remains unchanged and installation refuses to replace
  it.
- `make release-check`: passed (Go tests/vet/race, frontend typecheck/build,
  normalizer, embedded resources, isolated CLI smoke, Pair.app signature and
  binary checksums). The first attempt was not executed because automatic review
  temporarily lacked quota; a normal retry after the user's Continue succeeded.
- Installed CLI resolves to the current binary and reports the deployed build.
  V1 daemon update returned healthy status 200, matching JS and service worker.
  Both Pair.app binaries match `bin/`. All eight selected Work facts and the
  existing pane `$533 %566 38658` remain unchanged. Runner 38658, child 38677 and
  caffeinate child 38676 remain alive; five archived records retain exit facts.

Frontend cache: `mctrl-shell-7113156009bbcd72`; JS: `index-4FQGO7Ms.js`.
Evidence is in `bin/desktop-preview/`, including screenshots, layout and runtime
before/after records. Screenshot command:

```sh
node web/node_modules/@playwright/test/cli.js test --config /private/tmp/mctrl-desktop-polish-20261009/playwright.config.mjs
```

Exit criterion met: a desktop directory enters a phone-accessible Session through
one short command; repeated entry/return retains the process; phone continuation
operates the same external Session. This does not migrate an already running
ordinary terminal process. Recent Session still records this phone's visits.
Fresh physical-iPhone copy/keyboard/IME evidence, authenticated model-provider
acceptance and real TLS/WSS remain the previously recorded release gaps. No new
product-boundary or durability conflict was introduced.

## Usage-flow polish, second round — 2026-10-09

Source base `34ee1f9`, retaining the earlier uncommitted repairs and first-round
UX changes. Deployed binaries identify as
`1.0.0 / 34ee1f9-dirty / 2026-10-09T11:42:47Z`.

Implemented:

- Interactive launch opens Terminal directly; task launch opens Session output.
  Accepted Work without a Session is followed through GET observations of that
  same identity, with a bounded observation window and manual status check.
  Rapid taps create one POST. Leaving either a pending launch request or its
  follow-up page prevents late navigation; observed launch failure never retries.
- Output/path transfer replaces the draft selection or inserts at its caret,
  preserving surrounding text and returning focus without sending input. Copy
  actions have 44px touch targets and the sheet path field has its own full row.
- Preview captures remove trailing pane padding before applying the line limit.
  This fixes short output disappearing on Home, while keeping interior blank
  lines. Native preview fields start at the tail and retain their snapshot while
  reading older content or selecting text; Show latest output explicitly updates
  it. The copy sheet displays the capture time.
- Returning to the foreground refreshes Home, Session and Terminal facts through
  reads only. Terminal uses the Project name when available, with Runner and
  actual tmux identity retained as secondary context.

Validation:

- `.tools/go1.27.1/bin/go test ./internal/tmux`: passed, including real 100-row
  pane coverage for short output, line limits, interior blank lines and an empty
  pane.
- `MCTRL_LIVE_RUNNERS=1 npm --prefix web run test:browser`: **44 passed**, 22 in
  each of Chromium and WebKit. New coverage includes draft caret/selection and
  focus, stable selected snapshots, foreground refresh without mutations, rapid
  launch taps, accepted Work observation, both late-response navigation cases,
  observed failure, and task output routing. Existing durability/input tests and
  installed interactive Agent startup probes also passed.
- Mobile screenshot fixture: **1 passed**. At 393 × 852, launch mode buttons end
  at y=673 before the submit bar at y=698; Recent output starts at y=598. All six
  copy/transfer actions measure 44px tall. The 320 × 720 terminal and 393px Home
  have no horizontal overflow. Screenshots were visually inspected.
- `make release-check`: passed, including Go tests/vet/race, frontend typecheck
  and normalizer, final embedded build, isolated CLI smoke, Pair app signature,
  and both binary checksums. Cache is `mctrl-shell-1bbb5f6c596b3a82`, JS
  `index-DsUgnTy_.js`.
- Local V1 daemon updated and `/healthz` returned 200. Served JS bytes and
  service worker match the final embedded files; Pair.app binaries match `bin/`.
  All eight selected Work identity/state/archive/exit records remained unchanged.
  The live Work retained Session `$533`, Runner 38658, child 38677 and its
  caffeinate child 38676. Five archived records retain their original exit facts.

Local evidence: `bin/ux-preview-round2/`, including `layout.json`, screenshots,
`runtime-before.json` and `runtime-after.json`. Screenshot command:

```sh
node web/node_modules/@playwright/test/cli.js test --config /private/tmp/mctrl-ux-polish-20261009/playwright.config.mjs
```

Exit criterion met: all three supported routes remain continuous, late responses
respect the user's current page, copy/reading workflows preserve context, and
existing durability/no-replay checks pass. Physical iPhone native selection,
Copy menu, keyboard activation and IME still need fresh device evidence.
Authenticated provider/model acceptance and real TLS/WSS remain external release
gaps. No new product-scope conflict was introduced.

## Usage-flow polish — 2026-10-09

Source base `34ee1f9`, uncommitted repairs and UX changes. Final binaries:
`1.0.0 / 34ee1f9-dirty / 2026-10-09T11:12:32Z`.

Implemented: directory registration returns to Start with its Project selected;
remembered launch directory; explicit interactive/task modes; native output/path
copy and draft transfer; quoted cd preparation; literal draft insertion with a
separate Return; actual program paste-mode observation; startup input barrier;
recent Session continuation/review; output before folded process/end controls.
Directory observations follow the verified registered child after cd.

Validation:

- `make release-check`: passed, including new tmux paste-mode and child-directory
  tests, Go vet/race, frontend checks, CLI smoke, Pair app signature and checksums.
- `make build`: passed again after the final compact launch layout. Embedded
  frontend cache is `mctrl-shell-2bd2d810dedaa398`, JS `index-C-uo_hzK.js`.
- `MCTRL_LIVE_RUNNERS=1 npm --prefix web run test:browser`: **28 passed**, 14 each
  in Chromium and WebKit. Covers real tmux output/bytes, copy fallback, directory
  changes with Chinese/spaces/quotes, no implicit Return/replay, pending startup,
  unsafe multiline blocking, literal framed paste, desktop continuation, ended
  Session review, recovery/durability and installed Agent interactive startup.
- Mobile screenshot walk-through: passed at 393 × 852 and a 320 × 720 terminal.
  Launch mode buttons end at y=673, before the fixed submit bar at y=698; Recent
  output begins at y=598 (baseline y=1269); the 320px terminal has no horizontal
  overflow. Home card bounds also remain within the viewport.
- `make smoke-launcher`: passed after the final build. Pair app binaries equal
  `bin/` byte for byte; `shasum -a 256 -c SHA256SUMS`: both OK.
- Local V1 daemon restarted onto this build and serves the matching frontend;
  `/healthz` returned 200. All recorded Work identity/state/archive/exit facts
  remain unchanged: live Session `$533`, Runner 38658, child 38677; five archived
  records still retain their original unknown exit facts.

Local visual evidence is in `bin/ux-preview/` (ignored runtime artifacts), with
`layout.json` and `runtime-after.json`. The temporary screenshot fixture is
`/private/tmp/mctrl-ux-polish-20261009/preview.spec.ts`.

Exit criterion: the three supported routes can be followed continuously in the
isolated browser fixtures while existing Work durability and input rules pass.
This does not close the broader V1 release matrix. Physical iPhone selection
handles/native Copy menu/IME/keyboard switching need fresh device evidence.
Provider account/model acceptance and real TLS/WSS remain the previously recorded
external gaps. No new product-scope conflict was introduced.

## Repair validation — 2026-10-09

Current source base: `34ee1f9`, with uncommitted repairs. Binaries identify this
as `34ee1f9-dirty` instead of claiming the unchanged commit.

- `make release-check`: passed (Go tests/vet/race, frontend checks/build, CLI
  smoke, Pair app signature and binary checksums).
- Final frontend build/typecheck and embedded asset verification: passed after
  the state badge polish.
- `MCTRL_LIVE_RUNNERS=1 npm --prefix web run test:browser`: 12 passed, six each
  on Chromium and WebKit at mobile dimensions. Real browser processes are closed
  for the persistence case, and real tmux output is counted for input replay.
- Agent startup probes opened `codex-cli 0.154.0` and `opencode v2.0.24` in
  interactive mode with empty Prompts. Codex may display its native directory
  trust screen; the probe does not approve it or send a model task.
- `make smoke-launcher`: passed after the final rebuild. Both binaries bundled
  in `dist/Mctrl Pair.app` compare byte-for-byte equal to `bin/`; checksums match.
- The normalizer test now compiles and imports the same source function used by
  TerminalPage; 18 conversion cases and 23 untouched cases passed.

The authenticated provider probe did **not** pass. Both CLIs emitted real errors:

- Codex: configured `gpt-6.1-sol` is not supported with this ChatGPT account.
- OpenCode: an active OpenCode Go subscription is required for its configured
  model.

Model/provider settings and credentials were left unchanged. These are external
acceptance gaps, distinct from the passing interactive-startup contract.

The installed V1 control plane was restarted onto the repaired binaries. The
existing Work retained Session `$533`, Runner PID `38658` and child PID `38677`;
its Work-owned power assertion remained active. Five records whose Session,
Runner and child were absent were backed up, freshly rechecked and archived.
Their Work state and unknown exit facts were retained. The registered `mctrl`
Project now has zero unarchived Work references; `ai-research` has one live Work.
No registered Project was removed.

Backup: `bin/recovery-backup-2026-10-09T07-05-08-104Z/`, private permissions,
including original Work records and a summary. The archival UI keeps a collapsed
history. Final TLS/WSS, remaining physical-iPhone checks and V1/V2 simultaneous
phone use remain unverified.

The older dated evidence below is retained as historical context.

Validation date: 2026-09-25

Environment used for the final local gate:

- macOS on Darwin/arm64
- Go 1.27.1
- Node.js 22.21.1
- npm 10.9.4
- tmux 3.7c

## Automated release gate

Final command:

```sh
make release-check
```

Result: passed.

The gate verifies tool/dependency availability, Go modules, production frontend
build and asset version, embedded asset references, both Go binaries, Go tests,
`go vet`, the race detector, isolated CLI smoke commands, and binary checksums.

It includes repeatable coverage for:

- atomic/cross-process state and owner-aware file locks
- attempt-scoped, atomically claimed launch evidence
- conditional Work transitions and terminal-state non-regression
- canonical exit journal ordering and attempt validation
- Shell Runner exit 0/nonzero, stale launch rejection, power loss and log rotation
- request/prompt fingerprint idempotency and payload-reuse rejection
- auth, CSRF, Origin, secure cookies and 30-day browser-session expiry
- same-installation pairing upsert, credential rotation and revoked-device revival
- two-minute hashed one-time browser-link tokens and per-installation sessions
- loopback-only Pair Console QR rendering with manual Pair URL/code
- active-device list and revoked pairing-history separation
- real tmux lifecycle on isolated per-test sockets, including an initially absent
  socket, terminal shutdown after the last Session, server-state text that
  cannot be forged by Session names, active-pane selection, printable record
  parsing and 120×40 desktop/44×22 phone sizing
- forced UTF-8 locale for terminal attachments
- WebSocket attach, CLI revocation closure and `SESSION_GONE` for both a live
  server with another Session and a server that stops with its last Session
- structured Prompt delivery and no replay
- Power assertion liveness and exact owner PID matching
- PWA CSP, cache policy, service-worker build ID and iOS/maskable icons
- Project registry validation and persistence

Additional dependency check:

```sh
cd web
npm audit --omit=dev --audit-level=moderate --registry=https://registry.npmjs.org
```

Result: 0 vulnerabilities.

## Final build evidence

- Version: `0.1.0-dev`
- Build time: `2026-09-25T16:53:26Z`
- Source: Project/Session lifecycle polish layered on commit `edbed8d`
- JS asset: `assets/index-D6NIcu60.js`
- CSS asset: `assets/index-DPNUUREP.css`
- Service Worker cache: `mctrl-shell-d9fa0ca16d0db2fc`
- `mctrl` SHA-256: `8b13b9586a7938e2473cb55d35261cd54505f42a563760ea89642c0182c44136`
- `mctrl-runner` SHA-256: `9b917c0560958dd091cddc02c3459dc58f18bb4b0c14d8536caf09ec6eb7fdfb`
- `dist/Mctrl Pair.app` built and ad-hoc code-signature verified
- `bin/SHA256SUMS` generated by the final `make release-check`

## Terminal keybar layout check

- verified the two-row control layout at 393 px and the 320 px minimum width
- confirmed `ESC`, `TAB`, `CTRL`, `LOCAL`, and `编辑` remain fully visible
- confirmed `↑`, `↓`, `←`, `→`, and `⏎` fit the arrow row without horizontal
  scrolling, with `编辑` staying between `LOCAL` and the arrows
- confirmed the direction controls send the standard terminal sequences for up,
  down, left, and right
- confirmed `⏎` sends Return unchanged

## Mobile input and viewport changes — pending device verification

These landed in this pass and are **not** yet confirmed on the iPhone. They are
listed as expectations to check, not as results. Static checks only: the web
build type-checks and `make release-check` passes for both profiles. The web
package has no test runner, so nothing here is covered by an automated test yet.

- a Chinese keyboard's `：` reaches the program as `:`, and setting
  `mctrl-terminal-half-width` to `off` in `localStorage` restores the raw
  full-width bytes, so the conversion is a visible client-side choice and not a
  silent rewrite
- CJK text typed into the terminal is unchanged by the conversion
- a Latin keyboard's letters, digits and punctuation are each sent exactly once
  on the phone and in a desktop browser
- an armed CTRL still modifies a character typed on the software keyboard
- the digits of a Chinese keyboard select candidates and never reach the
  terminal, while the same digits on a Latin keyboard arrive as `1`–`9`
- scrolling back shows `↓ N lines back`; one tap returns to the tail and the
  control disappears
- attaching to a Session that produced output before the phone connected shows
  that output in the phone's scrollback, including lines well above the visible
  screen. Verified on the Mac against a real tmux attach, and not yet on the
  phone.
- PgUp and PgDn page a full-screen program's own history from the phone. The
  sequences and the reason they were missing are verified; that OpenCode answers
  to them is read out of its keybind table, not observed on the phone.
- the software keyboard opens without covering the terminal and the keybar stays
  above it
- the FULL hint occupies one line
- FULL shows no Structured Prompt dock, and the dock returns in LOCAL
- a Session name longer than the header wraps to two lines and then ellipsises
- the Home card shows a Managed Work state chip and an External tmux Session shows
  none. Not verified on the phone: the payload already carried `state` and the
  frontend never read it, so the data path is certain, but the rendering is not.
- Session detail loads 100 preview lines and grows to at most 500 on request
- the keybar keeps its home-indicator inset in FULL. The inset lived on the
  Prompt dock, which FULL hides, so hiding the dock removed the inset from the
  bottom of the screen. The inset now belongs to the workspace, which is true
  whichever child is last. Introduced by the FULL dock change and found by
  reading the padding rules, not by a test. Not verified on the phone.
- the Prompt dock's permanent at-most-once footnote is gone. It described a
  request ID, which is implementation detail, and the guarantee it referred to is
  enforced rather than actionable. It is still stated in the uncertainty notice,
  in UX.md and in TROUBLESHOOTING.md.
- Start Work explains a disabled launch button instead of leaving it mute
- Start Work launches with an empty Prompt. The page required one, and nothing
  else did: the API rejects only a prompt over 200000 characters or containing
  NUL, and the codex and opencode runners both guard their prompt argument with
  `if strings.TrimSpace(spec.Prompt) != ""`. So a plain shell Work, or an agent
  Work the user meant to type into, could not be started at all. Found because the
  user could not press the button.

Two destructive controls in Settings were dead in the user's hands:

- **Unregister Project and Revoke device were built on `window.confirm`.** iOS
  disables `window.confirm` in a standalone PWA: it displays nothing and returns
  false, so the handler returned early and the page did nothing at all, with no
  error anywhere. `SessionPage` already confirms inline for exactly this reason,
  so the app had two conventions and Settings was the one that did not work. Both
  now use the inline pattern.
- **A Project with active Managed Work could not be unregistered, and the page
  never said so.** The API refuses with `PROJECT_IN_USE` — a guard added
  deliberately — but the page only learned that after a failed attempt. The card
  now counts active Work per Project and states it on the card and in the
  confirmation, so the button is not a trap. The API remains the authority, and a
  count that cannot be read does not disable anything: a stale count must never be
  the reason a legitimate action is blocked.
- **The refusal was then invisible anyway.** The error rendered at the top of the
  Projects section while the button sits at the bottom of the project list, and the
  inline confirmation was closed *before* the request was made, so the only piece
  of feedback near the button had already been taken away. A correct 409 carrying
  an accurate message read as "nothing happened". The confirmation now stays open
  through the request, closes only on success, and states a refusal inside itself.
  Revoke device had the identical structure and the identical flaw.

The active-Work count was verified rather than assumed: replaying the page's filter
over the live store yields `project-E_zCrQ -> 3` and `milin-ZAVuXg -> 1`, matching
the four RUNNING Works. A defensive `Array.isArray` branch was removed once
`getWork` was confirmed to return `ManagedWork[]`, and `normalizeWork` was checked
to preserve `project_id` and `state` — otherwise the count would have been
silently zero and the feedback would have looked absent again.

## Feedback audit

The unregister incident was generalised into four questions, asked of every action
in the app, and the answers are now in UX.md as standing rules:

1. Is the in-progress state visible on the control itself?
2. Is success confirmed where the change happened?
3. Is a refusal stated next to the control, in a confirmation that stays open?
4. If the action cannot succeed now, is that said before the press?

Auditing against those found three more, all in Settings and all the same class:

- **A successful unregister was reported at the top of the Projects section.** The
  card it removed was at the bottom of the list, so on a phone the confirmation
  was off-screen. Success now renders where the card was.
- **Revoke had no success confirmation at all** — the device simply vanished from
  the list. It now confirms next to the list.
- **One `actionError` served two distant actions.** A revoke failure therefore
  also rendered in the browser-link panel, above the fold, generically labelled
  "Device action failed". Browser-link actions and revocation now report
  separately. Register and unregister likewise stopped sharing `projectError`,
  which had the same shape of problem: correct above the register form, invisible
  below the project list.

Audited and found already correct, so left alone: Session Detail's close flow
(which re-opens its confirmation on failure, the pattern Settings was missing),
Start Work's submit error (adjacent to the sticky button), the Terminal banners
(stacked between canvas and keybar, never scrolled), and the keybar's disabled
state (covered by the connection overlay directly above it).

One more fixed while auditing: an expired browser link disabled its Copy button
and said "Expires in 0:00", which describes a countdown rather than a state. It
now says the link has expired and to create a new one.

The preview bound is the API's; the paging is in the page.

What is evidence and what is not:

- **Evidence.** The cause of the dropped punctuation was read out of the installed
  xterm 5.5 source, not guessed: `Keyboard.ts` gates printable keys on
  `keyCode >= 48`, the same switch carries a `case 0` for iOS `UIKeyInput*` arrows,
  and `Terminal.ts` discards an `input` event when `composed === true` and a
  keydown was seen. iOS is documented not to populate `keyCode` for its
  on-screen keyboard. Letters and committed phrases use the two paths that are
  not gated, which is why they worked and punctuation did not.
- **Not evidence.** That iOS fires `beforeinput` for this case, and that
  `input.composed` is true for it. Both were plausible and both are now known to
  be wrong: the interception built on them changed nothing on the phone, which
  means the character never reaches a `beforeinput` at all.
- **Not evidence.** That a Chinese keyboard's number row consumes digits as
  candidate selection on this phone. That is the operating system's design and it
  matches what was observed, but it was observed as "nothing happened", not as an
  event trace.
- **Not evidence.** The software-keyboard inset. No desktop browser can raise an
  iOS keyboard, so the layout consuming `--keyboard-inset` is the only thing that
  has been checked.

The Chinese-keyboard question is settled, by measurement rather than inference,
and the answer is that it is not a defect mctrl can fix.

Measured at the daemon's input boundary, which sees every byte the phone sends:

| Key | Chinese keyboard | English keyboard |
| --- | --- | --- |
| `a` | `0x61` arrives | `0x61` arrives |
| Return | `0x0d` arrives | `0x0d` arrives |
| `1` | nothing | `0x31` arrives |
| `，` | nothing | `0x2c` arrives |

So the transport is sound and the browser produces no event for the digit row or
for punctuation under a Chinese keyboard. Nothing in the page can intercept a
character that is never dispatched. Two earlier conclusions were wrong, and the
reason is worth keeping: xterm's source was read as showing the character being
delivered and then discarded, and an interception built on that reading changed
nothing on the phone. Reading a library's control flow tells you what a browser
*would* do with an event, not what iOS *delivers*.

The measurement itself is gone. It was a temporary recorder in the terminal
bridge, gated behind a flag file so nothing was written during normal use, and it
has been removed along with its tests. What survives is the finding.

The measurement did find a real, separate defect, which is fixed here: the
full-width conversion covered U+FF01–U+FF5E and U+3000, so `。` (U+3002) and `、`
(U+3001) — what a Chinese keyboard actually sends for the period and comma keys —
passed through unchanged and a program binding `.` never saw one. The conversion
now also covers those two and the curly quotes, and
`web/scripts/check-normalizer.mjs` checks 18 conversions and 23 characters that
must not change, wired into `make test-web` so it cannot rot.

## History replay validation

The phone could only scroll to output it caused itself, while the Mac could
scroll freely over the same tmux pane. Cause: a fresh attach carries the pane's
visible grid alone, so the client's scrollback starts empty and fills only from
output that arrives after the attach.

Two claims were measured rather than assumed, because the fix depends on both:

- a real `tmux attach-session` on this machine sends `CSI 2 J` (clear screen) in
  its first bytes and does **not** send `CSI 3 J` (clear scrollback). The replay
  is therefore written before the live stream and survives the redraw.
- the panes hold real history, not repainted frames: the two mctrl Sessions carry
  57 and 202 lines of `history_size` against a 2000 limit, and the content is
  genuine shell output.

`TestTerminalWebSocketReplaysPaneHistoryOnAttach` covers it end to end. The first
version of that test passed with the replay disabled, because 60 lines was close
enough to the pane height that the attach — which creates its PTY at the canonical
full size and so *grows* the pane — revealed the marker anyway. It now prints 400
lines, asserts up front that the first marker is not on the visible screen, and
fails without the fix. The failure output shows the live stream carrying only
`hist-line-343` onward, which is exactly the bug.

Known limit, not a defect: a full-screen TUI repaints the same rows, so its tmux
history is many near-identical frames. The replay is faithful to what tmux holds
and is not a conversation log.

The replay also introduced a defect, found by review rather than by a test: every
attachment replays, and the xterm instance survives a dropped socket, so a
reconnect appended a second copy of the history into the middle of the scrollback
and a third on the next blip. The client now resets the terminal when it
re-attaches, so the scrollback is exactly one replay plus what follows. Not
verified on the phone; the mechanism is certain from the code — `replayHistory` is
unconditional per WebSocket, and `connect()` builds a new socket without
recreating the Terminal.

That reset must stay synchronous. Deferring it behind an xterm write callback
would let the *new* attachment's replay be parsed first, so the reset would wipe
the fresh history instead of the stale one, and replay would break on every
re-attach.

## Replayed history had to be captured at the client's width

The replay handed the client lines it had to wrap a second time. Captured
frames, off a real attach to a 240-column pane with a 46-column client, replayed
as:

```text
2 3 4 5; do printf "LONGLINE%02d-%s\n" $i "$(p
rintf "X%.0s" {1..200})"; done; printf "CJK-新
 建游戏项目-END\n"
```

Cause: the attach PTY is created at the canonical full size, so the pane is still
240 columns wide when the capture runs. The client then re-wraps 240-column lines
into 46 columns itself. tmux re-wraps its own history when the window narrows, so
the capture has to happen after the resize. `ServeHTTP` now waits for the client's
first resize message before capturing, bounded by `historySizeWait` so a client
that never resizes still gets a working terminal.

`TestTerminalWebSocketReplaysHistoryAtTheClientWidth` covers it. It sends the
resize late on purpose: the web client sends it immediately, which hides the race
this test exists for. Without the fix it fails with `replayed line is 157 columns
wide`.

**This is not the cause of the chopped lines reported from the phone.** See
"What the phone screenshots actually were".

## What the phone screenshots actually were

Two reports of "the terminal shows chopped lines and `997;1m` garbage" were first
misdiagnosed as client-side rendering defects. Both symptoms were then checked
against the source of truth, and neither was an mctrl defect:

- `capture-pane -e` on the affected Session showed `✗ 997;1n997;1n…` in the pane
  itself, wrapped in the pane program's own bold-red highlighter. The text was
  never corrupted in transit; the program had written it.
- The `cd "…/god` / `ot-ai-smoke-test/新建游戏项目"` break was also the pane's own
  content: the program rendered that line at the narrow width the phone's
  attachment gives the pane, and the frame froze when the program exited. The
  `capture-pane` output reproduced the phone screenshot line for line.
- `TestBridgeDeliversOnlyClientBytesToThePane` and
  `TestBridgePreservesEscapeSequenceBytes` record a real pane's stdin through the
  real bridge and assert that only the client's own keystrokes arrive, byte for
  byte, including a sequence split across frames. Both pass.
- xterm.js's device-query replies (`CSI 6n` → `ESC[1;1R`, `CSI c` → `ESC[?1;2c`,
  `CSI >c` → `ESC[>0;276;0c`, `CSI 5n` → `ESC[0n`) were captured at 46x20, 46x33
  and 240x60. None produces the reported text.

Two wrong fixes were nearly shipped on the strength of that first diagnosis, and
both are recorded here so the next person does not repeat them:

- deferring `terminal.reset()` until xterm's write queue drained, which would
  wipe the fresh replay as described above
- a floor on FULL's fit scale, described in the next section

## FULL's fit scale is unreadable, and that is the feature

Measured in the browser at 393x852, FULL applies `scale(0.209265)` to the
1878x1080 content: a 13pt font renders at 2.72pt, and the tmux status line — the
only row with ink across all 240 columns — becomes a solid bar across the screen.
That was treated as a defect, and `MIN_OVERVIEW_SCALE` was added to keep the text
legible.

It was rejected on the phone. At a floor of 0.5 the stage is 939pt wide, so FULL
shows 45 of the 240 columns and the user has to pan sideways for the rest. "The
whole Session at once" is the feature: an unreadable overview is still an
overview, and a partial one is just LOCAL with extra steps. Reading the output is
what LOCAL and the pinch gesture are for. The measurement stands; the conclusion
drawn from it did not.

## Window and pane inventory

The phone shows only the active window's active pane, which read as a limitation
without explanation. The Session page now lists every window and pane, grouped by
window, marking which one the phone is showing.

The switch that was proposed with it is deliberately absent, because measurement
showed it would break the desktop. On tmux 3.7c, with a desktop client attached:

- `attach-session -t session:window` attaches to that window, moves the Session's
  active window, and the desktop client follows
- `attach-session -t session:window.pane` attaches to the whole window
- `switch-client -c <client> -t <target>` returns rc 0 and moves every client

A client cannot hold an independent current window, so any switch from the phone
changes what the desktop user sees. The inventory reports the facts instead.
`TestInspectSessionReportsEveryWindowAndPane` covers the data: two windows, a
split pane, two distinct window indexes, both window names, and exactly one pane
that is active within the active window — the one the phone attaches to.

The first measurement was wrong and was discarded: it read client positions out
of `list-panes`, which reports pane ttys, not client ttys. The corrected probe
reads each client's window with `display-message -c <client>`.

## Raw keyboard transport validation

- a single keystroke reaches a Managed Work program without any Return, proving
  the pane is not line-buffered;
- Return reaches the program as Return (`^M` at the program's own echo), not as a
  line feed;
- the delivered line is not echoed a second time over the program's output;
- the attachment reports `INPUT_MODE_REPAIRED` only when it actually changed a
  pane;
- a Session without Managed Work is never reconfigured, and its terminal keeps
  the desktop client's settings;
- a runner re-asserts raw mode for its own pane while the Work runs, so client
  attach, detach, or resize cannot silently reintroduce line buffering

## Project and Session lifecycle validation

- `~` expands to the Mac home directory when registering a Project;
- Project paths remain independent from existing Session cwd values;
- Project unregister does not close Sessions;
- unregister is rejected while non-terminal Managed Work references the Project;
- external Sessions can be closed through the explicit Close Session action;
- active Managed Work requires a second force-confirmed close step;
- closing an already absent Session is idempotent;
- Home and Session detail distinguish Managed and external tmux Sessions;
- `make release-check` and `make release-check PROFILE=v2`: passed

## Terminal fidelity validation

- attachment PTY is put into raw mode before the tmux client starts;
- managed runner sets the tmux pane PTY to raw mode before starting the child;
- control bytes such as `ESC [ 10 ; rgb:... BEL` are covered by a regression test and are not rendered as `^[` / `^G`;
- terminal resize failures still send a bounded size, and duplicate resize frames are suppressed;
- `COLORTERM=truecolor` is passed to the tmux attachment client;
- DCS/PM/APC/SOS payloads are stripped from previews instead of leaking as text;
- `make release-check` and `make release-check PROFILE=v2`: passed

## Full Session overview validation

- `GET /api/v1/host` advertises the configured full size, and it equals the size a
  Managed Work pane is launched at
- FULL asks for `max(configured, pane)`: a phone-shrunk pane is raised back to the
  configured size, while a larger desktop pane stays authoritative
- the default is desktop-like (`240×60`), so a program that drops panels below a
  narrow threshold still shows them in FULL
- Settings can change the size, and the change persists to `config.json` and takes
  effect without a daemon restart
- an out-of-range size is rejected with a stable code and leaves the effective
  value untouched
- the endpoint requires a paired device and a valid CSRF token
- a config file written before `terminal_full_size` existed still loads
- the fit scale never exceeds 1, so a Session already smaller than the phone is
  never magnified
- the overview label reports the size actually being shown
- the Session title shows the session name rather than the tmux id
- an absurd client resize is clamped: a 4000×4000 request does not grow the shared
  window past 500×200
- switching between FULL and LOCAL re-requests the matching size in both directions

## Profile isolation validation

- `make release-check PROFILE=v2`: passed
- V2 build version: `2.0.0-dev`
- V2 `mctrl` SHA-256: `f94a1cf87f162691125b68e9711c4136b6ee80f089d15c80efa906d17b1842e0`
- V2 `mctrl-runner` SHA-256: `fe0504616fbb29eb003e4774d13be3f2c5e1e32b1bfa92ed4efdb54afd7c6519`
- V2 binaries built under `bin/v2/` without replacing V1 `bin/`
- V2 Pair app built as `Mctrl V2 Pair.app`
- V1 bundle identifier remained `com.mctrl.pair-launcher`
- V2 bundle identifier is `com.mctrl.v2.pair-launcher`
- V1 and V2 launchers invoke their explicit profile
- a temporary V2 daemon answered on port `17682` with `profile: v2`
- V1 health on port `17681` remained independently reachable
- V1 Settings can register and remove Projects from the PWA
- Start Work links directly to Project registration when the registry is empty

## Isolated end-to-end check

Performed with a temporary `MCTRL_HOME` and final binaries:

1. Initialized loopback configuration without touching the user's existing setup.
2. Paired through HTTP and used Cookie + CSRF authentication.
3. Registered a disposable Project.
4. Launched real Shell Managed Work through real `mctrl-runner` and tmux.
5. Confirmed `RUNNING`, Runner/child PID, prompt confirmation and preview marker.
6. Restarted the daemon while the Work was running.
7. Confirmed the same Work remained `RUNNING` and its Session remained attached
   to the same tmux identity.
8. Sent structured `exit` Prompt.
9. Confirmed canonical terminal facts: `EXITED`, `exit_code=0`,
   `launch_stage=child_exited`, `prompt_delivery=confirmed`.
10. Confirmed the tmux Session remained after Work exit.
11. Confirmed no test `mctrl-runner` or `caffeinate` process remained.
12. Exercised `launchctl bootstrap`, `print`, `kickstart -k` and `bootout` with an
    isolated temporary LaunchAgent label; the label was removed afterward.

The user's existing `mobile` tmux Session was not modified.

## Real iPhone Safari/PWA validation

Performed on an iPhone against Trusted-LAN HTTP at the Mac's private address:

1. Paired from a QR code and confirmed the warning that LAN HTTP is not encrypted.
2. Confirmed iOS standalone PWA and Safari can maintain separate pairing storage.
3. Fixed launchd-only empty Session enumeration by using an explicit macOS tmux
   socket and a printable per-call record delimiter instead of control-character
   table fields.
4. Confirmed the authenticated API and UI both listed the disposable and existing
   Sessions.
5. Opened LOCAL Terminal, entered a command and confirmed the virtual keyboard.
6. Corrected the tmux attach locale and confirmed the phone client was marked
   `UTF-8` and rendered the existing desktop Session's CJK output correctly.
7. Added FULL pane overview with fit, pinch zoom, native one-finger two-dimensional
   panning and explicit-keyboard editing. The desktop pane remained 272×60.
8. Locked and unlocked the phone; the stale `CONNECTING` socket was replaced and
   Terminal automatically returned to Live.
9. Launched real Shell Managed Work from the iPhone using a disposable Project.
   Confirmed `RUNNING`, confirmed Prompt delivery, Runner/child identity and a
   live Work-owned `caffeinate` assertion.
10. Confirmed phone lock/backgrounding did not stop the Work; after `sleep` ended,
    the interactive Shell correctly remained `RUNNING`.
11. Sent Structured Prompt `exit`; confirmed `EXITED`, `exit_code=0`,
    `child_exit_observed`, retained dead Session evidence and complete Runner/
    `caffeinate` release.
12. Launched another Work from the terminal Work card and verified the original
    Project was preselected.
13. Restarted the real LaunchAgent daemon while the second Work was `RUNNING`.
    The same Work, Session `$317`, Runner PID `6160` and child PID `6169`
    survived; the iPhone automatically reconnected.
14. Exited the second Work with the same canonical result and released its Runner
    and `caffeinate`.
15. Removed the disposable Project, directory, Managed Sessions and terminal smoke
    Session. The user's `mobile` Session remained attached and unchanged.
16. Verified Settings showed one active iPhone installation and a separate
    collapsed history of nine revoked test records. After loading the new PWA,
    the active device automatically gained a hashed installation identity, so a
    future re-pair in that same browser context will reuse its device record.
17. Created a two-minute one-time link from the paired PWA, opened it in Safari,
    and explicitly linked Safari. The registry retained one active device with
    two installation identities and two independent 30-day browser sessions; the
    pending link was consumed. Closing and reopening Safari's base URL returned
    directly to Home without Pair or Link.
18. Built and ad-hoc code-signed `dist/Mctrl Pair.app`, including a generated
    multi-resolution `AppIcon.icns`. Double-clicking it opened a Mac-only Pair
    Console rather than pairing the Mac browser. Verified the QR
    code, prominent one-time code, Copy code, manual phone Pair URL, Copy address
    and refresh behavior. The QR API is restricted to loopback plus same-origin
    and a valid one-time token.

## External validation still required

- Playwright Chromium/WebKit run in an environment with browsers installed.
- Final TLS-terminated deployment check with the chosen reverse proxy/CA.
- Codex/OpenCode authenticated integration using disposable projects.

Chromium/WebKit, final TLS/WSS and authenticated Agent Runner validation remain
unchecked in `RELEASE_CHECKLIST.md`.
