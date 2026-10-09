<div align="center">

# mctrl

### Your Dev Environment in Your Pocket

**Connect. Work. Create. Anywhere.**

[![CI](https://github.com/liamamilin/mctrl/actions/workflows/ci.yml/badge.svg)](https://github.com/liamamilin/mctrl/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.27.1-00ADD8?logo=go&logoColor=white)](go.mod)
[![Platform](https://img.shields.io/badge/platform-macOS-111111?logo=apple&logoColor=white)](https://github.com/liamamilin/mctrl)
[![License: MIT](https://img.shields.io/badge/License-MIT-79e6a5.svg)](LICENSE)

mctrl is a mobile control surface for terminal work running on your Mac.
Observe existing tmux sessions, intervene when needed, and launch durable
Managed Work from your iPhone—without moving execution into the cloud.

<p align="center">
  <img src="docs/images/hero.png" width="820" alt="mctrl — Your Dev Environment in Your Pocket" />
</p>

<p align="center">
  <img src="docs/images/workspace.png" width="32%" alt="mctrl professional session and workspace manager" />
  <img src="docs/images/cockpit.png" width="32%" alt="mctrl remote development cockpit" />
</p>

</div>

---

## What is mctrl?

mctrl is **not** an SSH client, cloud IDE, or remote desktop.

It is a focused control plane for a Mac that is already running your development
environment:

- **Resume** an existing tmux session from your phone.
- **Observe** recent terminal output without taking over the desktop.
- **Intervene** through a real terminal when a task needs attention.
- **Launch** registered Projects through Shell, Codex, or OpenCode runners.
- **Continue working** after the browser closes, the phone locks, or the daemon
  restarts.

> The computer does the work. The phone starts, observes, and intervenes.

## Highlights

| | |
|---|---|
| **Observe & Resume** | Discover real tmux Sessions, active commands, working directories, windows, panes, and recent output. |
| **Managed Work** | Launch a registered Project with a Runner and Prompt while `mctrl-runner` owns the real child process. |
| **Durable by design** | Work survives phone disconnects and daemon restarts; tmux remains the source of Session truth. |
| **Terminal escalation** | `LOCAL` provides a readable interactive viewport. `FULL` fits, zooms, and pans the complete desktop-sized pane. |
| **Safe prompting** | Structured Prompt actions are idempotent. Raw terminal input is never replayed after uncertain delivery. |
| **Power protection** | Configurable macOS power assertions protect active Work without promising to wake an unreachable Mac. |
| **Pairing that stays useful** | A browser stays paired for 30 days. Safari and the installed PWA can share one device record through a short-lived browser link. |
| **Mac pairing launcher** | A lightweight `Mctrl Pair.app` opens a QR Pair Console—no terminal required after initial setup. |

## Requirements

- macOS on Apple Silicon or Intel
- macOS logged in and reachable from the phone
- Go **1.27.1**
- Node.js **22+**
- tmux **3.7c** or compatible
- iPhone Safari or installed PWA

## Quick Start

### 1. Build and verify

```sh
git clone git@github.com:liamamilin/mctrl.git
cd mctrl
make release-check
```

`make release-check` rebuilds the PWA, embeds frontend assets, runs Go tests,
`go vet`, the race detector, TypeScript checks, CLI smoke tests, the macOS pairing
launcher check, and binary checksums.

### 2. Initialize mctrl

Loopback-only mode is the default:

```sh
./bin/mctrl setup
```

For a phone on a private trusted LAN:

```sh
./bin/mctrl setup --lan
```

The setup command installs a macOS LaunchAgent. After login, launchd starts and
maintains the daemon.

### 3. Pair the iPhone

Build the lightweight Finder launcher:

```sh
make mac-launcher
open 'dist/Mctrl Pair.app'
```

The launcher opens a Mac-only Pair Console with:

- a QR code;
- a copyable one-time pairing code;
- a manual pairing address for devices that cannot scan.

The equivalent CLI is:

```sh
./bin/mctrl pair --open
```

### 4. Register a Project

On the phone, open **Settings → Projects**, enter a display name and the
absolute path of an existing directory on the Mac (or `~` for the Mac user's
home directory), then tap **Register Project** and **Start in this directory**.
The launch form returns with that Project selected. Starting from **Start →
Register a Project** opens the registration section directly.

A Project is only a future launch target. It can have multiple Sessions, and
unregistering it does not close them. To terminate a Session and its processes,
open **Session detail → End this session → Close Session** explicitly.

The Mac CLI remains available for first-time setup or batch registration:

```sh
./bin/mctrl project add /path/to/project --name 'My Project'
./bin/mctrl project list
```

### 5. Start or continue working

- **Phone interaction:** choose a Project and Shell, Codex or OpenCode. Shell
  opens in the displayed directory; Codex/OpenCode default to **Open & interact**.
  Launch opens Terminal directly. A remembered Project is preselected next time.
- **Submit a task:** choose Codex/OpenCode → **Run a task**, enter the Prompt,
  and launch. Session output opens directly; read output and process exit facts
  when returning.
- **Continue desktop work:** install the short command once after building:

  ```sh
  ./scripts/install-cli.sh
  ```

  Then, in WezTerm or another terminal:

  ```sh
  cd /path/to/project
  mctrl here
  codex
  ```

  The Session appears on the phone automatically. Repeating `mctrl here` from
  the same directory resumes it; Ctrl+B, then D leaves it running. The phone's
  **Continue on your Mac** section provides the exact return command when needed.
  An already running ordinary terminal process cannot be attached this way.
  The helper uses the daemon's recorded connection and selected profile, while
  explicit socket overrides remain available.

Home offers **Continue last session** when its live identity still matches. The
Session page puts recent output before folded process details and termination.

In Terminal, tap **Copy** to select output or copy the current path. **Use
selection in draft** inserts at the draft caret or replaces its selection while
preserving surrounding text; **Prepare cd
command** quotes a selected directory for a supported Shell. Review the draft,
tap **Insert**, then press **⏎** when ready. Insert preserves text and does not
append Return. Multiline insertion requires observed program paste support.
On HTTP, native text selection remains available when clipboard access fails.
Preview refresh preserves selected text and older output until **Show latest
output** is tapped. Returning to the App refreshes session facts immediately.

## Everyday CLI

```sh
mctrl status                 # daemon, transport, Sessions, Projects, devices
mctrl doctor                 # local diagnostics
mctrl pair                   # print a one-time phone Pair URL and QR
mctrl pair --open            # open the Mac Pair Console
mctrl here                   # create/resume a phone-accessible shell here
mctrl project list           # list registered Projects
mctrl devices                # show active and revoked device audit records
mctrl restart                # restart only the daemon
mctrl uninstall              # stop mctrl without deleting arbitrary tmux Sessions
```

## Version Isolation

The stable release and the next development line are isolated at both Git and
runtime levels:

```sh
mctrl profile                         # show the active V1 paths
mctrl --profile v2 profile            # inspect the V2 paths
mctrl --profile v2 setup --lan        # initialize V2 on port 17682
mctrl --profile v2 status
mctrl --profile v2 pair
```

V1 keeps `~/.mctrl`, its existing launchd service, cookies, Pair app, and tmux
behavior. V2 uses `~/.mctrl-v2`, port `17682`, a separate launchd service,
cookies, tmux socket, and `Mctrl V2 Pair.app`. V2 never writes to V1 state by
default. See [`VERSIONING.md`](VERSIONING.md) for cutover, rollback, and
side-by-side development.

## Transport Profiles

### Trusted-LAN HTTP

Zero external dependency. Intended only for a private, trusted network.

> **Warning:** plain HTTP provides no end-to-end confidentiality. A malicious
> or actively monitored LAN may observe or tamper with traffic.

### TLS-Terminated HTTPS/WSS

Use a trusted TLS endpoint such as Tailscale Serve or a user-controlled reverse
proxy:

```sh
mctrl setup \
  --transport tls_terminated \
  --public-url https://mctrl.example.internal
```

The public URL must be the exact browser-facing origin.

## Architecture at a Glance

```text
iPhone Safari / installed PWA
              │
       HTTPS / WSS or Trusted-LAN HTTP
              │
        mctrl daemon (Go)
        ├── pairing + device auth
        ├── REST + terminal WebSocket bridge
        ├── Project + Runner registry
        ├── Work lifecycle + recovery journal
        ├── power assertions
        └── tmux adapter
                  │
          durable tmux Sessions
                  │
             mctrl-runner
                  │
        Shell / Codex / OpenCode
```

The daemon is a control plane. `mctrl-runner` owns the Work child process, and
tmux owns Session persistence.

## Security Model

mctrl grants a paired phone highly privileged terminal access as the current
macOS user. The implementation includes:

- short-lived, one-time pairing tokens;
- salted credential and Session hashes;
- 30-day browser Sessions;
- strict Origin and CSRF validation for browser mutations;
- WebSocket revocation;
- active device records plus collapsed revoked history;
- two-minute, single-use browser-link tokens for Safari/PWA linking;
- no arbitrary `/execute` endpoint;
- no automatic replay of raw terminal input or uncertain Prompts.

Read [`SECURITY.md`](SECURITY.md) and
[`TRANSPORT_SECURITY.md`](TRANSPORT_SECURITY.md) before deployment.

## Development

```sh
make build          # rebuild PWA, embed assets, then build V1 binaries
make build PROFILE=v2  # build isolated V2 binaries under bin/v2
make mac-launcher PROFILE=v2  # build Mctrl V2 Pair.app
make test           # Go tests + vet + strict TypeScript
make test-race      # race detector, including real tmux integration
make test-browser   # Chromium/WebKit regression (install browser runtimes first)
make release-check  # complete deterministic V1 release gate
```

Frontend development:

```sh
cd web
npm ci
npm run dev
```

Documentation:

- [`DEVELOPMENT.md`](DEVELOPMENT.md)
- [`ARCHITECTURE.md`](ARCHITECTURE.md)
- [`PRODUCT.md`](PRODUCT.md)
- [`API.md`](API.md)
- [`WEBSOCKET.md`](WEBSOCKET.md)
- [`TEST_PLAN.md`](TEST_PLAN.md)
- [`TROUBLESHOOTING.md`](TROUBLESHOOTING.md)
- [`VALIDATION.md`](VALIDATION.md)
- [`VERSIONING.md`](VERSIONING.md)

## Repository Layout

```text
cmd/mctrl/             daemon and CLI
cmd/mctrl-runner/      Managed Work process owner
internal/api/          REST, WebSocket, static PWA serving
internal/auth/         pairing, browser Sessions, device linking
internal/config/       validated configuration
internal/project/      Project registry
internal/runner/       Shell/Codex/OpenCode runner registry
internal/tmux/         tmux lifecycle adapter
internal/work/         durable Work records and recovery
web/                   Preact + TypeScript PWA
packaging/macos/       macOS pairing launcher assets
scripts/               build and release scripts
```

## Current Status

`1.0.0` · V1 stable baseline

The real-iPhone Trusted-LAN path has been exercised. Chromium/WebKit regression
tests cover recovery, durability, reconnect and Prompt uncertainty. Empty Agent
Prompts open interactive CLIs. Lost Work can be reviewed and archived in Settings
without inventing an exit result or blocking its Project indefinitely.

Final TLS/WSS deployment, the remaining real-iPhone interaction matrix, and
authenticated Codex/OpenCode provider integration remain release gates. The
2026-10-09 provider probes were blocked by the configured Codex model/account
combination and OpenCode Go subscription requirements; see `VALIDATION.md`.

## License

[MIT](LICENSE) © 2026 liamamilin
