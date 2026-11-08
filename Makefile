.PHONY: deps dev-api dev-web test test-go test-web build install-convert-vacs-to-lumi build-linux desktop-build desktop-app desktop-check tag tag-patch tag-minor tag-major migrate-new migrate-up migrate-down migrate-version

TAG_BUMP ?= patch
TAG_INITIAL_VERSION ?= 0.1.0
TAG_REMOTE ?= origin
CODEX_VERSION ?= 0.1.0
CODEX_OUT ?= build/codex
CODEX_WINDOWS_OUT ?= build/codex-windows
CODEX_DEV_OUT ?= build/codex-dev
CODEX_DEV_DATA_DIR ?= $(HOME)/.lumi_dev
PYTHON ?= python3
CINDY_DEV_DATA_DIR ?= $(HOME)/.lumi_dev
CINDY_APP ?= Cindy
GOBIN ?= $(HOME)/.local/bin

deps:
	go mod download
	pnpm --dir web install --frozen-lockfile
	pnpm --dir rel/app install --frozen-lockfile

dev-api:
	go tool air

dev-web:
	pnpm --dir web run dev

test: test-go test-web

test-go:
	go test ./...

test-web:
	pnpm --dir web test

build:
	pnpm --dir web run build
	mkdir -p build
	go build -trimpath -tags embed_frontend -ldflags="-s -w" -o build/lumi_web ./cmd/lumi_web
	go build -trimpath -tags embed_frontend -ldflags="-s -w" -o build/lumi ./cmd/lumi
	go build -trimpath -ldflags="-s -w" -o build/lumi_ctl ./cmd/lumi_ctl
	node scripts/package-codex.mjs "$(CODEX_VERSION)" "$(CODEX_OUT)"

install-convert-vacs-to-lumi:
	mkdir -p "$(GOBIN)"
	GOBIN="$(GOBIN)" go install -trimpath ./cmd/convert-vacs-to-lumi

# CLI/plugin release assets; Go + web only, no Cargo or Tauri invocation.
.PHONY: codex-build codex-package codex-release codex-smoke codex-dev codex-dev-build codex-dev-refresh codex-dev-stop
codex-dev: codex-dev-build
	+$(MAKE) codex-dev-refresh

codex-dev-build:
	@if command -v pnpm >/dev/null 2>&1; then pnpm --dir web run build; else corepack pnpm --dir web run build; fi
	mkdir -p "$(CODEX_DEV_OUT)"
	go build -trimpath -tags embed_frontend -ldflags="-X main.defaultEnvironment=development" -o "$(CODEX_DEV_OUT)/lumi_dev" ./cmd/lumi
	node scripts/dev-codex.mjs --out "$(CODEX_DEV_OUT)" --data-dir "$(CODEX_DEV_DATA_DIR)"

codex-dev-refresh:
	node scripts/dev-codex.mjs --out "$(CODEX_DEV_OUT)" --data-dir "$(CODEX_DEV_DATA_DIR)" --install

codex-dev-stop:
	"$(CODEX_DEV_OUT)/lumi_dev" stop --environment development --data-dir "$(CODEX_DEV_DATA_DIR)"

codex-build:
	pnpm --dir web run build
	mkdir -p "$(CODEX_OUT)"
	go build -trimpath -tags embed_frontend -ldflags="-s -w" -o "$(CODEX_OUT)/lumi" ./cmd/lumi

codex-package:
	node scripts/package-codex.mjs "$(CODEX_VERSION)" "$(CODEX_OUT)"

.PHONY: codex-macos-binaries codex-windows-build
codex-release: codex-package codex-macos-binaries

codex-macos-binaries:
	pnpm --dir web run build
	mkdir -p "$(CODEX_OUT)"
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -tags embed_frontend -ldflags="-s -w" -o "$(CODEX_OUT)/lumi-darwin-arm64" ./cmd/lumi
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -tags embed_frontend -ldflags="-s -w" -o "$(CODEX_OUT)/lumi-darwin-amd64" ./cmd/lumi
	cd "$(CODEX_OUT)" && shasum -a 256 lumi-darwin-arm64 > lumi-darwin-arm64.sha256
	cd "$(CODEX_OUT)" && shasum -a 256 lumi-darwin-amd64 > lumi-darwin-amd64.sha256
	@echo "CI signs macOS binaries and regenerates their SHA256 files (see install.md)."

# Cross-build from macOS/Linux; Windows CI uses build-codex-windows.ps1.
codex-windows-build:
	pnpm --dir web run build
	mkdir -p "$(CODEX_WINDOWS_OUT)"
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -tags embed_frontend -ldflags="-s -w" -o "$(CODEX_WINDOWS_OUT)/lumi-windows-amd64.exe" ./cmd/lumi
	cd "$(CODEX_WINDOWS_OUT)" && shasum -a 256 lumi-windows-amd64.exe > lumi-windows-amd64.exe.sha256

codex-smoke:
	PYTHONDONTWRITEBYTECODE=1 $(PYTHON) scripts/test-install-codex.py --assets "$(CODEX_OUT)"

# Build author directories; development installation uses Cindy's local .cindy import.
.PHONY: cindy-build cindy-dev cindy-dev-install cindy-dev-stop
cindy-build:
	node cindy-plugins/scripts/build.mjs

cindy-dev:
	node cindy-plugins/scripts/build.mjs --channel development --data-dir "$(CINDY_DEV_DATA_DIR)"

# Submit the new package to Cindy; installation confirmation stays in Cindy.
cindy-dev-install:
	@$(MAKE) cindy-dev || { printf '%s\n' '错误：Cindy 开发版构建失败，未执行安装。请修复上方构建错误后重试。' >&2; exit 1; }
	node cindy-plugins/scripts/install-dev.mjs --app "$(CINDY_APP)"

# Reads the generated configuration so it always stops the same development instance.
# Run only after coordinating every client sharing that data directory.
cindy-dev-stop:
	node cindy-plugins/scripts/stop-dev.mjs

build-linux:
	pnpm --dir web run build
	mkdir -p build/linux-amd64
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -tags embed_frontend -ldflags="-s -w" -o build/linux-amd64/lumi_web ./cmd/lumi_web
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -tags embed_frontend -ldflags="-s -w" -o build/linux-amd64/lumi ./cmd/lumi
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o build/linux-amd64/lumi_ctl ./cmd/lumi_ctl

desktop-build:
	./rel/app/tauri.sh build

desktop-app:
	./rel/app/tauri.sh app

desktop-check:
	./rel/app/tauri.sh check

tag:
	@set -eu; \
	bump="$(TAG_BUMP)"; \
	case "$$bump" in patch|minor|major) ;; *) echo "TAG_BUMP must be patch, minor, or major." >&2; exit 1 ;; esac; \
	git rev-parse --verify HEAD >/dev/null; \
	if [ -n "$$(git status --porcelain)" ]; then \
		echo "Working tree must be clean before creating a release tag." >&2; \
		exit 1; \
	fi; \
	if [ -n "$(TAG_REMOTE)" ]; then \
		git remote get-url "$(TAG_REMOTE)" >/dev/null 2>&1 || { echo "Git remote '$(TAG_REMOTE)' does not exist." >&2; exit 1; }; \
		git fetch --quiet "$(TAG_REMOTE)" 'refs/tags/*:refs/tags/*'; \
	fi; \
	current="$$(git tag --list 'v*' --sort=-v:refname | sed -nE '/^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$$/ { p; q; }')"; \
	if [ -z "$$current" ]; then \
		next="$(TAG_INITIAL_VERSION)"; \
	else \
		version="$${current#v}"; \
		major="$${version%%.*}"; \
		remainder="$${version#*.}"; \
		minor="$${remainder%%.*}"; \
		patch="$${remainder#*.}"; \
		case "$$bump" in \
			major) major=$$((major + 1)); minor=0; patch=0 ;; \
			minor) minor=$$((minor + 1)); patch=0 ;; \
			patch) patch=$$((patch + 1)) ;; \
		esac; \
		next="$$major.$$minor.$$patch"; \
	fi; \
	if ! printf '%s\n' "$$next" | grep -Eq '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$$'; then \
		echo "Invalid semantic version: $$next" >&2; \
		exit 1; \
	fi; \
	tag="v$$next"; \
	if git rev-parse --quiet --verify "refs/tags/$$tag" >/dev/null; then \
		echo "Git tag already exists: $$tag" >&2; \
		exit 1; \
	fi; \
	git tag --annotate "$$tag" --message "Release $$tag"; \
	if [ -n "$(TAG_REMOTE)" ]; then \
		echo "Created $$tag. Push it with: git push $(TAG_REMOTE) $$tag"; \
	else \
		echo "Created $$tag."; \
	fi

tag-patch:
	+@$(MAKE) --no-print-directory tag TAG_BUMP=patch

tag-minor:
	+@$(MAKE) --no-print-directory tag TAG_BUMP=minor

tag-major:
	+@$(MAKE) --no-print-directory tag TAG_BUMP=major

migrate-new:
	@test -n "$(name)" || (echo "usage: make migrate-new scope=app|project name=create_example" >&2; exit 1)
	@test "$(scope)" = "app" -o "$(scope)" = "project" || (echo "scope must be app or project" >&2; exit 1)
	go run ./cmd/lumi_ctl migrate create "$(scope)" "$(name)"

migrate-up:
	go run ./cmd/lumi_ctl migrate app up

migrate-down:
	go run ./cmd/lumi_ctl migrate app down "$(if $(steps),$(steps),1)"

migrate-version:
	go run ./cmd/lumi_ctl migrate app version

# Real-model evaluations are opt-in and are never part of make test.
.PHONY: eval-agent eval-agent-list eval-agent-report eval-agent-compare test-evals
EVAL_ARGS ?=
eval-agent:
	go run ./cmd/lumi_evals $(EVAL_ARGS)

eval-agent-list:
	go run ./cmd/lumi_evals --list $(EVAL_ARGS)

eval-agent-compare:
	@test -n "$(run)" || (echo "usage: make eval-agent-compare run=evals/results/<run>" >&2; exit 1)
	go run ./cmd/lumi_evals --compare "$(run)" $(EVAL_ARGS)

eval-agent-report:
	@test -n "$(run)" || (echo "usage: make eval-agent-report run=evals/results/<run>" >&2; exit 1)
	go run ./cmd/lumi_evals --report "$(run)"

test-evals:
	go test ./internal/agentevals ./cmd/lumi_evals
