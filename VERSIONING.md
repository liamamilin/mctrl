# Version Isolation

mctrl uses Git releases and runtime profiles as two separate layers:

- **Git** freezes and reviews source changes.
- **Runtime profile** isolates the state, listener, launchd service, tmux server, cookies, and Pair app of an installed version.

A V2 development profile can run beside the V1 installation without changing V1's data or service.

## Branch and tag policy

```text
main       current stable release
v2-dev     V2 integration branch
v1.0.0     immutable V1 release tag
v2.0.0-rc.1, v2.0.0-rc.2, v2.0.0
```

V1 bug fixes may continue on `main` while `v2-dev` is developed. Before a V2 release candidate, merge the required V1 fixes into `v2-dev`, then merge the verified release candidate back into `main`.

The V1 state directory is not a V2 development directory. No automatic data migration is performed. There is not yet a `migrate` or `snapshot` command: take a copy only after active V1 Work has drained, and do not copy `pairing.json`, PID files, lock directories, or claimed launch evidence as active Work. A V2 cutover normally requires a fresh V2 pairing because browser cookies and installation identities are intentionally profile-scoped.

## Runtime profiles

| Resource | V1 | V2 development |
|---|---|---|
| State | `~/.mctrl` | `~/.mctrl-v2` |
| Default port | `7681` | `17682` |
| LaunchAgent label | `com.mctrl.daemon` | `com.mctrl.v2.daemon` |
| LaunchAgent plist | `~/Library/LaunchAgents/com.mctrl.daemon.plist` | `~/Library/LaunchAgents/com.mctrl.v2.daemon.plist` |
| tmux socket | macOS default user socket | `/private/tmp/tmux-<uid>/mctrl-v2` |
| Session cookie | `mctrl_session` | `mctrl_v2_session` |
| CSRF cookie | `mctrl_csrf` | `mctrl_v2_csrf` |
| Pair app | `Mctrl Pair.app` | `Mctrl V2 Pair.app` |
| Bundle identifier | `com.mctrl.pair-launcher` | `com.mctrl.v2.pair-launcher` |
| V2 binary directory | `bin/` | `bin/v2/` |
| Build version | `1.0.0` | `2.0.0-dev` |

V1 values are frozen for compatibility. An already-running V1 daemon with a pre-profile health response is treated as V1 for compatibility; V2 requires an explicit V2 health identity.

`MCTRL_HOME` remains an explicit state-directory override. `MCTRL_TMUX_SOCKET` remains an advanced explicit tmux override. The profile only supplies defaults when those variables are unset.

## Commands

Show the resolved profile and paths:

```sh
mctrl profile
mctrl --profile v2 profile
mctrl --profile v2 profile --json
```

Set up V2 for a private LAN:

```sh
mctrl --profile v2 setup --lan
mctrl --profile v2 pair
```

V2 can be kept out of the login startup set during development:

```sh
mctrl --profile v2 setup --lan --no-start-after-login
mctrl --profile v2 restart
```

Operational commands are profile-scoped:

```sh
mctrl --profile v2 status
mctrl --profile v2 doctor
mctrl --profile v2 logs
mctrl --profile v2 restart
mctrl --profile v2 uninstall
```

The equivalent environment form is useful for scripts and LaunchAgents:

```sh
export MCTRL_PROFILE=v2
mctrl setup --lan
```

## Building isolated Pair apps

V1 remains the default build:

```sh
make mac-launcher
```

Build the V2 app without replacing the V1 app or binaries:

```sh
make mac-launcher PROFILE=v2
open 'dist/Mctrl V2 Pair.app'
```

V2 uses `bin/v2/mctrl` and `bin/v2/mctrl-runner`. The V1 LaunchAgent continues to point at the V1 `bin/mctrl` path.

## tmux visibility

V2 uses a separate tmux socket during development, so its tests and Managed Work cannot modify V1 sessions. It intentionally does not enumerate the V1/default socket.

At final cutover, if V2 must adopt the existing desktop sessions, stop V1 first and explicitly point V2 at the V1 socket:

```sh
MCTRL_TMUX_SOCKET=/private/tmp/tmux-$(id -u)/default \
  mctrl --profile v2 setup --lan
```

Do not run V1 and V2 as concurrent writers against the same tmux socket.

## Cutover and rollback

1. Complete V2 release-candidate testing on port `17682`.
2. Back up `~/.mctrl` and record the V1 state directory.
3. Stop new V1 Work and wait for active Work to finish or migrate explicitly.
4. Stop the V1 daemon, but leave tmux running.
5. Configure V2 with the desired final state and, if required, the V1 tmux socket.
6. Pair the V2 PWA and verify Sessions, Projects, terminal input, and Work.
7. Move the stable pointer to V2 and create the V2 release tag.
8. Keep the V1 state directory and V1 binary until the rollback window closes.

Rollback is:

```text
stop V2
start V1
```

V1 keeps its original state, cookies, port, LaunchAgent, and binary unless an explicit cutover migration changed those resources. Uninstalling or purging V2 must never target V1 state.
