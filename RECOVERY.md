# Recovery and Reconciliation

## Objective

Control-plane failure must not be confused with Work failure.

## Daemon startup

1. load and validate config
2. load Project registry
3. load device registry
4. enumerate persisted Work
5. inspect tmux
6. inspect process evidence
7. reconcile records
8. begin serving

## Reconciliation rules

For each Work believed RUNNING or STARTING, check independently:

- tmux Session exists?
- mctrl-runner exists?
- child PID exists?
- runner result file contains exit data?
- latest launch stage?
- current attempt id?
- canonical exit result belonging to that attempt?

## Never invent facts

Do not turn:

```text
runner missing
```

into:

```text
exit_code = 1
```

unless that code was actually observed.

Use explicit recovery metadata.

## STARTING recovery examples

### session exists, runner absent

Persist factual launch failure/recovery state with stage information.

Preserve Session for diagnostics unless policy explicitly says otherwise.

### runner exists, daemon lost launch response

Reconstruct Work from persisted evidence.

Do not create a second runner.

### child exists, prompt delivery status unknown

Do not automatically inject the Prompt again.

This is essential: prompt replay may duplicate user intent.

Surface a recovery state requiring observation/intervention.

## Idempotency after restart

`request_id` mapping must survive daemon restart.

A repeated client request after restart must resolve to the same Work.

## Completed Work

The Runner writes an attempt-scoped exit result before updating Work to `EXITED`.
The result contains the observed exit code or signal and cannot be reused by
another attempt. If the process stops between those writes, reconciliation
rebuilds Work from the result. Once final exit facts are persisted, the daemon
trusts them unless validation detects corruption or a mismatch.
