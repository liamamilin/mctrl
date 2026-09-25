# CLI Specification

## Commands

```text
mctrl setup
mctrl version
mctrl status
mctrl pair
mctrl doctor
mctrl logs

mctrl project add <path> [--name <name>] [--runner <id>]
mctrl project remove <id>
mctrl project list

mctrl devices
mctrl revoke <device-id>

mctrl restart
mctrl uninstall
mctrl profile [--json]
```

All commands accept the global selector:

```sh
mctrl --profile v2 status
MCTRL_PROFILE=v2 mctrl status
```

## setup

- verify macOS
- verify tmux
- create state directory
- initialize config
- install/start LaunchAgent
- start daemon
- explain transport profile
- require `--public-url` for `tls_terminated`
- honor `start_after_login` / login LaunchAgent policy
- optionally begin pairing

## status

Example:

```text
Server       running
Address      http://milin-macbook.local:7681
Transport    Trusted LAN HTTP
Sessions     4
Projects     3
Devices      1
RemoteReady  on_ac
```

## profile

`mctrl profile` reports the selected profile, state directory, config path,
LaunchAgent label, tmux socket, Pair app name, and bundle identifier. V1 is the
backward-compatible default; V2 uses port `17682` and isolated state by default.

## pair

- open pairing for short TTL
- print QR code
- print expiration
- close after success/expiry

## doctor

Check:

- macOS version
- tmux presence/version
- daemon
- listener
- selected transport profile
- config
- Project paths
- Codex presence/version
- OpenCode presence/version
- power assertion state
- local hostname
- basic API reachability

## restart

Restart daemon only. When the per-user LaunchAgent is loaded, use
`launchctl kickstart -k` and wait for a new daemon PID; otherwise perform the
background-process restart path.

Must not intentionally terminate:

- tmux sessions
- mctrl-runner
- child processes

## uninstall

Do not destroy arbitrary user tmux sessions.

Deleting persistent mctrl state requires an explicit destructive flag.
