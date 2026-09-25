GO ?= $(shell if [ -x "$(CURDIR)/.tools/go1.27.1/bin/go" ]; then printf '%s' "$(CURDIR)/.tools/go1.27.1/bin/go"; else command -v go; fi)
NPM ?= npm
NODE ?= node
VERSION ?= 0.1.0-dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || printf none)
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
GO_LDFLAGS := -X mctrl/internal/version.Version=$(VERSION) -X mctrl/internal/version.Commit=$(COMMIT) -X mctrl/internal/version.BuildTime=$(BUILD_TIME)

.PHONY: build build-go build-web check-release-deps mac-launcher test test-go test-race test-web release-check smoke-cli smoke-launcher verify-assets web-deps

web-deps: web/node_modules/.mctrl-deps

web/node_modules/.mctrl-deps: web/package.json web/package-lock.json
	cd web && $(NPM) ci
	touch $@

build: verify-assets

# Go embeds internal/api/assets. Never compile before the current web build has
# been copied into that directory.
build-go: build-web
	mkdir -p bin
	$(GO) build -trimpath -ldflags "$(GO_LDFLAGS)" -o bin/mctrl ./cmd/mctrl
	$(GO) build -trimpath -ldflags "$(GO_LDFLAGS)" -o bin/mctrl-runner ./cmd/mctrl-runner

build-web: web-deps
	cd web && $(NPM) run build
	find internal/api/assets -mindepth 1 -maxdepth 1 -type f -delete
	find internal/api/assets/assets -type f -delete
	cp web/dist/index.html internal/api/assets/index.html
	cp web/dist/assets/* internal/api/assets/assets/
	cp web/dist/manifest.webmanifest internal/api/assets/manifest.webmanifest
	cp web/dist/sw.js internal/api/assets/sw.js
	cp web/dist/icon.svg internal/api/assets/icon.svg
	cp web/dist/apple-touch-icon.png internal/api/assets/apple-touch-icon.png
	cp web/dist/icon-192.png internal/api/assets/icon-192.png
	cp web/dist/icon-512.png internal/api/assets/icon-512.png
	cp web/dist/icon-maskable-512.png internal/api/assets/icon-maskable-512.png

verify-assets: build-go
	cd web && $(NODE) scripts/finalize-dist.mjs --verify-only ../internal/api/assets

test: test-go test-web

test-go:
	$(GO) test ./...
	$(GO) vet ./...

test-race:
	$(GO) test -race -count=1 -timeout=3m ./...

test-web: web-deps
	cd web && $(NPM) run typecheck

check-release-deps:
	@test "$$($(GO) env GOVERSION)" = "go1.27.1" || { echo "release-check requires Go 1.27.1"; exit 1; }
	@test "$$($(GO) env CGO_ENABLED)" = "1" || { echo "release-check requires CGO for the race detector"; exit 1; }
	@test "$$(uname -s)" = "Darwin" || { echo "release-check requires macOS"; exit 1; }
	@command -v tmux >/dev/null || { echo "release-check requires tmux; otherwise integration tests would skip"; exit 1; }
	@command -v $(NODE) >/dev/null || { echo "release-check requires Node.js"; exit 1; }
	@$(NODE) -e 'if (Number(process.versions.node.split(".")[0]) < 22) process.exit(1)' || { echo "release-check requires Node.js 22+"; exit 1; }
	@command -v $(NPM) >/dev/null || { echo "release-check requires npm"; exit 1; }
	@$(GO) version
	@$(NODE) --version
	@$(NPM) --version
	@tmux -V

smoke-cli:
	@set -eu; \
		root=$$(mktemp -d "$${TMPDIR:-/private/tmp}/mctrl-release-smoke.XXXXXX"); \
		trap 'rm -rf "$$root"' EXIT; \
		mkdir -p "$$root/project"; \
		MCTRL_HOME="$$root" ./bin/mctrl version >/dev/null; \
		./bin/mctrl-runner --version >/dev/null; \
		MCTRL_HOME="$$root" ./bin/mctrl setup --no-start >/dev/null; \
		MCTRL_HOME="$$root" ./bin/mctrl status >/dev/null; \
		MCTRL_HOME="$$root" ./bin/mctrl doctor >/dev/null; \
		MCTRL_HOME="$$root" ./bin/mctrl pair >/dev/null; \
		MCTRL_HOME="$$root" ./bin/mctrl project add "$$root/project" --name smoke >/dev/null; \
		MCTRL_HOME="$$root" ./bin/mctrl project list >/dev/null

mac-launcher: build
	@scripts/build-mac-pair-launcher.sh

smoke-launcher:
	@scripts/build-mac-pair-launcher.sh >/dev/null
	@test -x 'dist/Mctrl Pair.app/Contents/MacOS/mctrl-pair-launcher'
	@test -x 'dist/Mctrl Pair.app/Contents/Resources/mctrl'
	@test -x 'dist/Mctrl Pair.app/Contents/Resources/mctrl-runner'
	@test -s 'dist/Mctrl Pair.app/Contents/Resources/AppIcon.icns'
	@test "$$(plutil -extract CFBundleIconFile raw 'dist/Mctrl Pair.app/Contents/Info.plist')" = 'AppIcon'
	@codesign --verify --deep --strict 'dist/Mctrl Pair.app'

release-check: check-release-deps
	GOTOOLCHAIN=local $(GO) mod download
	GOTOOLCHAIN=local $(GO) mod verify
	$(MAKE) build
	$(MAKE) test
	$(MAKE) test-race
	$(MAKE) smoke-cli
	$(MAKE) smoke-launcher
	cd bin && shasum -a 256 mctrl mctrl-runner > SHA256SUMS
