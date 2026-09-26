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
mctrl project remove <id>                         # unregister; never closes Sessions
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

## project

`project add` accepts `~` as the Mac user's home directory and stores the
resolved absolute path. A Project is a launch target, not an owner of tmux
Sessions.

`project remove` unregisters a Project only. It does not close Sessions or stop
Work. Active Managed Work must finish before the Project can be unregistered.

`project restore` re-inserts a Project under an id you supply. It exists for one
recovery case: a Project was unregistered while a Work still referenced it, so
that Work points at an id the registry no longer holds. `project add` cannot fix
this, because it mints a new id and would orphan the Work's history.

```text
mctrl project restore --id <id> <path> [--name <name>] [--runner <id>]
```

The id must not already exist, and the path must not belong to a different id.
Ids written by older versions are accepted: the check stays permissive on
purpose, because the records that need recovering use that older shape. The path
must exist and be a directory, and `~` expands to the Mac user's home directory.

`mctrl doctor` reports a live Work whose Project id is missing, and prints the
exact restore command to run.


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
