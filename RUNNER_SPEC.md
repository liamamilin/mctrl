# Runner Contract

## Purpose

Runner adapters define how a Managed Work process is started and how the initial Prompt is delivered.

Built-ins:

- shell
- codex
- opencode

## Not a plugin platform

No dynamic loading, marketplace, RPC SDK, or external manifest system in V1.

## Required runner contract

Conceptually:

```go
type Runner interface {
    ID() string
    Name() string
    Detect(ctx context.Context) Detection
    BuildLaunch(spec WorkSpec) (LaunchPlan, error)
}
```

`LaunchPlan` must explicitly describe:

- executable
- arguments
- environment additions
- cwd
- prompt delivery strategy
- any readiness condition required before prompt injection

## Prompt delivery strategies

May include:

### argv

Only if the installed CLI reliably supports it.

### stdin

Only if stdin semantics are verified.

### terminal_injection

Start interactive program, wait for a verified readiness condition, then inject the prompt.

## Hard rule

Do not invent Codex/OpenCode CLI behavior from memory.

Before implementing each adapter:

1. inspect installed CLI help/version,
2. run a disposable probe,
3. document the observed behavior,
4. add a contract/integration test,
5. implement the smallest reliable strategy.

## Prompt delivery facts

Track separately:

```text
child_started
prompt_delivery_started
prompt_delivery_confirmed
prompt_delivery_failed
```

A prompt delivery failure does not prove the child process failed.

## Shell runner

Shell Runner starts an interactive shell in the selected Project.

Prompt text sent to Shell is terminal text, not semantically interpreted AI input.

## Adapter failure

Runner detection/launch errors must map to stable API errors.

No silent fallback from Codex to another runner.
