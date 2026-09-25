# Security Model

## Security significance

A paired phone can interact with terminal sessions as the current macOS user.

Treat paired-device access as highly privileged.

## V1 threat boundary

V1 has two explicit transport profiles.

### Trusted-LAN mode

- zero external dependency
- local HTTP + WebSocket allowed
- intended only for a private trusted network
- does not claim confidentiality against malicious LAN peers
- UI and README must show this limitation

### Secure transport mode

- HTTPS + WSS
- supplied by a configured trusted TLS endpoint
- may use Tailscale Serve or another user-controlled trusted reverse proxy
- transport termination must preserve authentication/origin expectations

## Pairing

Pairing is closed by default.

`mctrl pair` opens a short-lived pairing window.

Requirements:

- cryptographically random token
- one-time
- short TTL
- invalidated after successful use
- no token logging

## Device authentication

Long-lived secrets must never appear in URLs.

Server stores only non-reversible credential material where applicable.

Browser session credentials expire after 30 days; the device credential remains
until explicit revocation. In secure transport mode, use secure browser
cookie/session semantics.

Each browser installation creates a random, non-hardware installation identity
in local storage. The daemon stores only its one-way hash. Re-pairing that browser
context reuses the same device record and rotates its credentials; successful
pairing still requires the short-lived one-time token. This is not a hardware
fingerprint and must not be used to merge unrelated devices.

An authenticated PWA may create a two-minute, one-time browser link for an
explicitly named Safari/browser context. The raw token is returned only in the
authenticated create response, stored on the Mac as salted hash material, never
placed in a request URL, and consumed atomically when the new browser context
establishes its independent 30-day session. The link grants the same privileged
terminal capability as pairing and must be treated as a bearer secret until it
expires. Revoking the shared device invalidates all linked browser sessions.

In Trusted-LAN HTTP mode, the implementation must not pretend a cookie is protected from network interception.

The Mac Pair Console's QR endpoint is available only to a loopback remote
address with a valid same-origin browser request and the current one-time token.
It renders the existing phone Pair URL and never consumes or bypasses pairing.
The console may display the token for manual entry, so the Mac screen and any
screen sharing remain sensitive during the pairing window.

## Origin validation

Browser pairing, cookie-authenticated mutations, and WebSocket upgrades must
carry a valid `Origin` matching the configured transport profile. Authenticated
non-browser API clients may omit Origin when using a bearer credential; they
remain subject to the configured allowed-origin policy when one is supplied.

Mandatory for:

- pairing
- POST /work
- structured prompt actions
- DELETE/revoke actions
- WebSocket upgrade

## CSRF

State-changing browser APIs require:

- strict Origin validation, and
- a defensible SameSite/CSRF design.

## Project boundary

Mobile launch targets only registered Project IDs.

No arbitrary `/execute` endpoint.

## Revocation

A paired device can be revoked from:

- CLI
- Settings

Revocation blocks future control requests but does not terminate already-running Work unless explicitly designed later.

## Logs

Never log:

- pairing token
- long-lived credential
- cookie/session secret
- full Authorization header

`store_prompts=false` prevents mctrl from retaining Prompt text in its structured
Prompt idempotency records. A one-way launch request fingerprint is retained to
reject `request_id` payload reuse. It cannot erase command text already visible in terminal
scrollback, echoed by an interactive Shell, emitted by an Agent, or copied into
runner logs. Treat `~/.mctrl/logs/runner/` and tmux scrollback as sensitive command
history.

## Security claims rule

Documentation and UI must not use words such as:

```text
secure
encrypted
private
```

for Trusted-LAN HTTP transport without qualifying the exact limitation.
