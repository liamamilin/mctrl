# Configuration and Persistence

Root:

```text
~/.mctrl/
```

Layout:

```text
~/.mctrl/
├── config.json
├── projects.json
├── devices.json
├── pairing.json                 # only while pairing is open
├── prompts/                     # structured Prompt idempotency evidence
├── work/
│   ├── work_xxx.json
│   ├── work_xxx.launch.json     # retained only when store_prompts=true or recovery needs it
│   └── work_xxx.result.json
└── logs/
    ├── mctrl.log
    └── runner/
```

## config.json

Example:

```json
{
  "schema_version": 1,
  "device_name": "Milin's MacBook",
  "listen_address": "0.0.0.0",
  "port": 7681,
  "transport_profile": "trusted_lan_http",
  "remote_availability": "on_ac",
  "start_after_login": true,
  "store_prompts": false,
  "public_url": "http://192.168.1.20:7681",
  "allowed_origins": []
}
```

The example shows an explicitly selected LAN listener. The V1 implementation default
is `127.0.0.1`; `0.0.0.0` must be opted into through setup or configuration.

`transport_profile` values:

- trusted_lan_http
- tls_terminated

`remote_availability` values:

- on_ac
- work_only
- always

## public_url

`public_url` is the browser-facing origin, without a trailing path. It is used for
QR pairing and Origin validation. It is optional for Trusted-LAN HTTP and required
for `tls_terminated`, where it must use `https://`.

The daemon still listens on loopback in `tls_terminated`; a trusted reverse proxy or
overlay endpoint terminates TLS on the same Mac.

## Work files

One Work per file.

Persist atomically:

```text
write temporary file
→ flush/fsync as appropriate
→ atomic rename
```

## Persist launch stage

A STARTING Work must record its latest confirmed launch stage for reconciliation.

## Prompt retention

`store_prompts=false` avoids retaining launch and structured Prompt text in mctrl's
own idempotency records. A one-way request fingerprint is retained so reuse of a
`request_id` with a different payload can be rejected. Terminal output and Agent JSON output may still contain the
Prompt or command output in tmux scrollback and runner logs; protect the state
directory accordingly.

## No SQLite

Do not introduce SQLite in V1 without an explicit spec change.
