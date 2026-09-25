# Transport Security Profiles

## Why this document exists

mctrl controls a shell. Transport security cannot be left implicit.

## Profile A — Trusted-LAN HTTP

Purpose:

- zero-dependency local use
- development
- private home LAN where the user accepts the trust model

Characteristics:

```text
HTTP
WS
pairing/auth at application layer
no end-to-end transport encryption
```

Security limitation:

> A malicious or actively monitored LAN may observe or tamper with traffic.

The product must display this honestly.

## Profile B — HTTPS/WSS

Purpose:

- stronger deployment
- untrusted LAN
- remote access through an overlay/private network

Characteristics:

```text
HTTPS
WSS
authenticated application session
```

Possible transport providers:

- Tailscale Serve
- user-managed reverse proxy with a trusted certificate
- another explicitly supported TLS endpoint

## Browser-facing URL

A TLS terminator normally has a different browser-facing hostname from the
loopback listener. Configure it explicitly:

```sh
mctrl setup \
  --transport tls_terminated \
  --public-url https://mctrl.example.internal
```

`public_url` is an origin only, must use HTTPS in this profile, and is added to the
allowed Origin set. Do not derive a phone pairing URL from `127.0.0.1` when TLS is
terminated upstream.

## V1 implementation rule

Core mctrl must remain transport-agnostic enough to run behind a TLS terminator.

Do not hard-code assumptions that all requests originate as plaintext HTTP.

## Cookie rule

If transport is HTTPS:

- prefer HttpOnly
- prefer Secure
- prefer restrictive SameSite compatible with the UI flow

If transport is plain HTTP:

- `Secure` cannot be the basis of the design
- documentation must state the interception risk

## Binding

The daemon may listen on a configurable address.

Default must be conservative and documented.

The implementation should support:

- localhost-only for reverse-proxy/Tailscale deployments
- LAN listening for Trusted-LAN mode

Avoid silently exposing a shell-control service on unintended interfaces.

## Public Internet

Direct port forwarding to the public Internet is explicitly unsupported in V1.
