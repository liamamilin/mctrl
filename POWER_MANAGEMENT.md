# Power Management

## Objective

Managed Work must not be interrupted by ordinary idle sleep.

The screen may turn off.

## Host assertion

Owner:

```text
mctrl daemon
```

Purpose:

Remote availability according to policy.

Modes:

- on_ac
- work_only
- always

Recommended:

```text
on_ac
```

## Work assertion

Owner:

```text
mctrl-runner
```

Purpose:

Keep a running Managed Work alive independently of browser and daemon lifetime.

## Liveness

Remote Ready must reflect the actual `caffeinate` process, not only an in-memory
assertion object. Unexpected Work assertion exit is surfaced as
`POWER_ASSERTION_LOST`; mctrl does not silently claim protection after that fact.

## Separation matters

Daemon crash may remove host-level Remote Ready behavior.

It must not remove the work-level assertion held by a still-running `mctrl-runner`.

## V1 guarantee

Protect against ordinary user-idle system sleep when policy requires.

Do not promise protection from:

- lid close
- explicit Sleep command
- shutdown
- thermal emergency
- low-battery forced behavior
- OS/hardware failure

## Release

Work assertion is released after the child exit is observed and persisted.

Do not tie release to browser disconnect.
