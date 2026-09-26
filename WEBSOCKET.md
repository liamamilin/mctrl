# Terminal WebSocket Protocol

Endpoint:

```text
WS /api/v1/sessions/:id/terminal
```

## Attachment

Each WebSocket connection owns one ephemeral attach process:

```text
browser
   ↕
WebSocket
   ↕
PTY
   ↕
tmux attach-session
```

The attach process is not the Session owner.

### First frames on a successful attach

In order:

1. optional text control frames, such as `INPUT_MODE_REPAIRED`
2. one binary frame carrying the pane's history replay, if any — plain lines
   separated by CRLF, at most 500 lines
3. the live output stream

The replay comes first so the client's scrollback is populated before the live
redraw. It is not a distinct frame type: it is ordinary terminal output, so a
client needs no special handling and older clients are unaffected. See
TMUX_CONTRACT.md for why this is needed and why the redraw does not erase it.

Because every attachment replays, a client that keeps its terminal across a
reconnect must reset it on re-attach, or the scrollback accumulates one copy of
the history per connection. mctrl's client does this.

## Binary frames

Client → server:

```text
raw terminal input bytes
```

Server → client:

```text
raw terminal output bytes, history replay first when present
```

Do not JSON-wrap terminal byte streams.

## Text control frames

Resize:

```json
{
  "type": "resize",
  "cols": 44,
  "rows": 22
}
```

Error:

```json
{
  "type": "error",
  "code": "SESSION_GONE"
}
```

Notice:

```json
{
  "type": "notice",
  "code": "INPUT_MODE_REPAIRED",
  "message": "This Session's keyboard transport was line-buffered on the Mac. mctrl reset it to raw, so keys now reach the program immediately and Return arrives as Return."
}
```

A notice reports something mctrl changed on the Mac while the attachment was
live. It never blocks input and never ends the attachment.

`INPUT_MODE_REPAIRED` means the Session's pane keyboard transport was not raw and
mctrl reset it. See `TMUX_CONTRACT.md`.

A client must treat an unknown `type` as a protocol error, as before.

## Raw input semantics

Raw input is:

```text
at-most-once / no application retry
```

If delivery is uncertain, do not resend.

## Reconnection

On connection loss:

1. disable input,
2. show reconnect state,
3. destroy dead attach resources,
4. leave tmux untouched,
5. reconnect with backoff,
6. create new PTY attachment.

Recommended backoff:

```text
0.5s → 1s → 2s → 5s → 10s
```

## Origin and authentication

WebSocket upgrade must enforce:

- authenticated paired device/session,
- allowed Origin,
- configured transport policy.

## Size policy

The mobile client must not become the authority that shrinks a larger desktop tmux layout.

See `TMUX_CONTRACT.md`.
