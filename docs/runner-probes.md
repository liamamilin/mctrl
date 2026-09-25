# Runner Probe Evidence

Probe date: 2026-09-25 (local development host)

The following evidence was collected from the installed CLIs before implementing
adapters. These are observations, not assumptions from product documentation.

## Codex

- Executable: `/opt/homebrew/bin/codex`
- Version: `codex-cli 0.154.0`
- `codex --help` exposes the positional form `codex [OPTIONS] [PROMPT]`.
- `codex exec --help` documents non-interactive execution, JSONL output via
  `--json`, `--skip-git-repo-check`, `-C/--cd`, and an initial prompt as a
  positional argument.
- The V1 adapter therefore uses `codex exec --json --skip-git-repo-check -C
  <project> -- <prompt>` when a prompt is present. No raw terminal replay is
  used for Codex.

## OpenCode

- Executable: `~/.opencode/bin/opencode`
- Version: `opencode v2.0.16`
- `opencode --help` exposes `opencode run [flags] [message...]`.
- `opencode run --help` documents `--format json`, `--standalone`, and the
  message positional argument.
- The V1 adapter uses `opencode run --format json -- <prompt>` when a prompt
  is present. The child is still supervised by `mctrl-runner`; no fallback to
  another runner is performed.

## Shell readiness

The Shell adapter starts a short `/bin/sh` wrapper that creates a private
readiness file and then `exec`s the user's configured interactive shell.
`mctrl-runner` waits for that file before injecting terminal text. A timeout is
recorded as `PROMPT_DELIVERY_FAILED`; it is never retried blindly.

## Follow-up required before release

The commands above establish CLI shape, not authentication, provider,
permission, or model behavior. Release integration tests must use disposable
credentials/projects and verify the observed contract for the exact installed
version. If a future CLI removes or changes the positional prompt form, the
adapter must be updated with new evidence rather than silently changed.
