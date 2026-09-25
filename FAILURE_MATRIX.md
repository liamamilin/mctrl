# Failure Matrix

This file defines expected V1 behavior for important failure paths.

| Failure | Durable Work | Session | UI / State |
|---|---|---|---|
| Safari closed | continues | remains | phone disconnected |
| iPhone locks | continues | remains | reconnect on return |
| Wi-Fi drops | continues | remains | reconnecting |
| WebSocket fails | continues | remains | fresh attach PTY |
| attach PTY crashes | continues | remains | reconnect |
| daemon crashes | continues if already launched | remains | offline until daemon returns |
| daemon restarts | continues | remains | reconcile |
| tmux server dies | cannot guarantee | lost/affected | explicit TMUX_UNAVAILABLE |
| runner child exits 0 | stops normally | remains | EXITED + exit_code 0 |
| runner child exits nonzero | stops | remains | EXITED + actual code |
| mctrl-runner dies before recording exit | unknown | preserve | factual recovery reason |
| Project path disappears | not launched | unchanged | PROJECT_PATH_MISSING |
| Runner executable missing | not launched | unchanged | RUNNER_NOT_FOUND |
| tmux session create fails | not launched | none/new session absent | LAUNCH_FAILED |
| runner start fails after session creation | none | preserve for diagnostics | LAUNCH_FAILED |
| prompt delivery fails after child starts | child may continue | remains | explicit prompt delivery error |
| duplicate POST /work | no duplicate | unchanged | return existing Work |
| raw key sent near disconnect | unknown delivery | remains | never replay |
| paired device revoked | work continues | remains | new control requests rejected |
| Mac display sleeps | continues | remains | no issue |
| ordinary idle system sleep requested during Managed Work | prevented | remains | no issue |
| lid closes | outside guarantee | may suspend | no false promise |
| Mac shuts down | stops/suspends by OS | unavailable | Offline |

## Design rule

When uncertain, preserve the Session and evidence.

Do not convert an infrastructure failure into an invented semantic Agent result.
