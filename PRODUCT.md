# Product Definition

## Job to be done

When the user is away from the desk but the Mac remains available:

1. observe existing terminal work,
2. start a new piece of work,
3. intervene when necessary,
4. return to the same work later on the Mac.

## Product model

```text
Availability
    ↓
Launch / Resume
    ↓
Observe
    ↓
Intervene
```

There is no fifth core activity in V1.

## Product primitives

- Host
- Project
- Session
- Managed Work
- Runner
- Paired Device

## Key distinctions

### Session ≠ Work

A Session is a durable terminal environment owned by tmux.

A Managed Work is one specific launch initiated through mctrl.

A Work may exit while its Session remains alive.

### Connection ≠ Session

A browser connection is disposable.

Closing Safari must not terminate a Session or Work.

### Process state ≠ agent meaning

V1 may know:

```text
child process exists
child process exited
exit code = 0
```

V1 must not infer:

```text
agent is thinking
agent is waiting
task succeeded
task failed semantically
```

## Product invariants

1. tmux owns session persistence.
2. mctrl never owns the user's long-running shell session.
3. mobile disconnect never stops durable work.
4. daemon restart never intentionally terminates Managed Work.
5. Managed Work prevents ordinary idle system sleep.
6. display sleep remains allowed.
7. lid-close/manual/forced sleep are outside the guarantee.
8. phone attachment must not degrade the desktop tmux layout.
9. V1 does not infer semantic agent state.
10. V1 does not promise wake-from-sleep.
11. existing tmux sessions remain first-class.
12. terminal is an escalation path, not the home screen.
13. V1 has no arbitrary remote `/execute` API.
14. transport security limitations must be stated truthfully.
