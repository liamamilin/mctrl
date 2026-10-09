# Release Checklist

This checklist records repeatable local and real-iPhone evidence. Items that require
Playwright browsers, a real TLS endpoint, or authenticated Agent CLIs remain
unchecked until that external environment is available.

## Product

- [x] no scope expansion beyond V1
- [x] Resume works
- [x] Launch works
- [x] Observe works
- [x] Intervene works
- [x] terminal Work can be relaunched with Project preselection
- [x] Mac Pair Console supports QR, copyable code and manual phone URL
- [x] Settings supports registering and removing Projects without the CLI
- [x] Project paths expand `~` and do not alter existing Session cwd
- [x] Project unregister never closes Sessions and blocks active Managed Work
- [x] Close Session is explicit, confirmed, and independent of Project removal

## Reliability

- [x] browser close preserves Work — Chromium/WebKit browser processes closed in isolated regression tests
- [x] phone lock preserves Work
- [x] daemon restart preserves Work
- [x] Session persists after Work exit
- [x] duplicate Launch does not duplicate process
- [x] raw terminal input not replayed — Chromium/WebKit offline/reconnect tests count the actual tmux output
- [x] uncertain Prompt not replayed
- [x] Work recovery tested
- [x] lost Work can be archived after fresh Session/process verification without inventing exit facts
- [x] archived Work retains history/idempotency and no longer blocks Project unregister
- [x] stale launch evidence and old Launch retries cannot revive archived Work

## tmux

- [x] external Sessions discovered
- [x] preview capture correct
- [x] attach/detach correct
- [x] launchd daemon enumerates the same tmux Sessions as the CLI
- [x] mobile attachment is UTF-8 and renders CJK
- [x] desktop layout unaffected by mobile attach
- [x] FULL overview pans/zooms without changing tmux dimensions
- [x] FULL overview remains editable through the keyboard control
- [x] Session disappearance handled
- [x] raw PTY prevents control sequences from being echoed as `^[` / `^G`
- [x] OpenCode-style terminal capability replies do not corrupt the mobile input path

## Security

- [x] pairing closed by default
- [x] pairing token short-lived
- [x] same browser installation reuses one device record
- [x] revoked device history excluded from Settings
- [x] one-time cross-browser link maps Safari/PWA to one device record
- [x] browser link token is short-lived, hashed and single-use
- [x] device revocation works
- [x] Origin validation works
- [x] CSRF policy works
- [x] secrets absent from API DTOs/log paths
- [x] transport profile shown honestly
- [x] public port-forwarding not documented

## Power

- [x] Remote Ready policy works
- [x] Work assertion independent of browser
- [x] Work assertion independent of daemon
- [x] assertion released after child exit
- [x] display sleep allowed

## Browser

- [x] Chromium E2E — recovery, durability, reconnect, uncertain Prompt and interactive CLI startup
- [x] WebKit E2E — recovery, durability, reconnect, uncertain Prompt and interactive CLI startup
- [x] real iPhone Safari
- [x] Add to Home Screen / standalone PWA
- [x] virtual keyboard usable
- [x] lock/unlock reconnect UX usable
- [x] PWA and Safari storage isolation understood and verified
- [x] active device list and revoked pairing history verified on iPhone

## Usage-flow polish — 2026-10-09

- [x] first directory registration returns to the selected Project
- [x] direct interactive/task launch choices and starting-directory preview
- [x] native output/path selection with clipboard fallback
- [x] selected directory prepares a quoted cd draft; insertion does not append Return
- [x] actual program mode guards multiline insertion, including client-only framing
- [x] startup input barrier drops early keys without queuing
- [x] desktop continuation and last-session identity/ended-session checks
- [x] output before folded process/termination details at mobile dimensions
- [x] interactive launch opens Terminal; task launch opens output
- [x] accepted Work follows GET observations, with one POST for rapid taps and no late navigation after leaving
- [x] copy transfer preserves draft cursor/selection; live refresh preserves reading and selection
- [x] short tmux output remains visible; foreground return refreshes without sending input
- [x] native desktop here/resume keeps Session/child identity and separates directories
- [x] copied Mac return command uses effective connection and refuses changed identity
- [x] final Chromium/WebKit regression — 48 passed
- [x] local update retains original Work, Session and process identities
- [ ] new copy/draft interactions exercised on a physical iPhone

## Runner

- [x] Shell contract test
- [x] Codex behavior probed and documented
- [ ] Codex authenticated integration test
- [x] OpenCode behavior probed and documented
- [ ] OpenCode authenticated integration test
- [x] empty Agent Prompt opens the installed interactive CLI, rather than a batch command reading stdin

2026-10-09 provider probes reached the CLIs but did not pass: Codex's configured
`gpt-6.1-sol` was rejected for this ChatGPT login; OpenCode's configured Go model
requires an active Go subscription. These gates remain open. The browser suite
is targeted regression coverage; the broader real-iPhone interaction matrix and
the final TLS/WSS deployment still require their own evidence.

## Documentation

- [x] setup instructions
- [x] security limitations
- [x] trusted-LAN mode explained
- [x] secure transport mode explained
- [x] troubleshooting
- [x] `mctrl doctor`
- [x] lightweight macOS Pair launcher and multi-resolution app icon release-verified

## Version isolation

- [x] V1 defaults remain backward compatible
- [x] V2 state, port, LaunchAgent, cookies, tmux socket, and Pair app are distinct
- [x] V2 refuses a V1-marked state root
- [x] V1/V2 service-worker and browser storage identities are separated
- [x] V2 Pair launcher invokes `--profile v2`
- [ ] V1 and V2 have been exercised simultaneously on a real iPhone

## Required local gate

```sh
make release-check
```

The gate must pass on macOS with Go 1.27.1, CGO, Node.js 22+, npm and tmux. It
rebuilds and embeds the frontend, runs tests/vet/race, executes CLI smoke checks
and writes binary checksums.
