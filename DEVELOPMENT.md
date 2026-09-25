# Development

## Requirements

- macOS with `tmux` installed
- Go 1.27.1 or newer (`go version` must not report the legacy Go 1.13 toolchain)
- CGO-capable compiler for `go test -race`
- Node.js 22+ and npm
- Optional: `codex` and `opencode` on `PATH`

## Build

```sh
make build
```

This is the only supported local binary build path. It performs the operations in
this order:

```text
npm ci
→ TypeScript check + Vite production build
→ asset version verification
→ copy current assets into internal/api/assets/
→ build bin/mctrl
→ build bin/mctrl-runner
```

Go embeds `internal/api/assets`; compiling before the asset sync would ship stale
frontend files. Release builds inject version, commit (when available), and UTC
build time; both binaries expose `--version`.

For a development server:

```sh
cd web
npm run dev
```

The Vite development server can proxy `/api` to the local daemon with
`VITE_DEV_API_TARGET`.

## Test

```sh
make test
make test-race
```

The full deterministic release gate is:

```sh
make release-check
```

It verifies the pinned Go toolchain, CGO, macOS, Node.js 22+, npm and tmux; verifies
Go modules; runs tests/vet/race; rebuilds the frontend; embeds and verifies the
assets; builds both binaries; runs stateless CLI smoke checks; and builds/verifies
the lightweight macOS pairing launcher bundle.

Real tmux integration tests skip when the `tmux` executable is absent during ordinary
`go test` runs, but `make release-check` refuses to continue without tmux so a
release cannot silently lose that coverage.

## Local first run

```sh
./bin/mctrl setup
./bin/mctrl pair --open
```

`--open` displays a Mac-only Pair Console with a QR code; the phone scans it
and uses the normal Pair page. Alternatively build and double-click the
non-resident Finder launcher:

```sh
make mac-launcher
open 'dist/Mctrl Pair.app'
```

`setup` binds to `127.0.0.1` by default. Use `mctrl setup --lan` only when the
operator intentionally chooses Trusted-LAN HTTP. The UI shows that this transport
has no end-to-end encryption.

For a loopback TLS deployment behind a trusted terminator, configure the external
browser origin:

```sh
./bin/mctrl setup \
  --transport tls_terminated \
  --public-url https://mctrl.example.internal
```

When running from a nonstandard build location, set `MCTRL_RUNNER_BINARY` to the
absolute path of `mctrl-runner` for launch tests. Launch evidence is removed after
the runner consumes it when `store_prompts` is false; set that option only when
retaining prompts is intentional. Runner logs and terminal scrollback can still
contain commands or output even when mctrl does not retain structured Prompt text.
