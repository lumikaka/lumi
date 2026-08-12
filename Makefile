.PHONY: deps dev-api dev-web test test-go test-web build build-linux desktop-build desktop-app desktop-check tag tag-patch tag-minor tag-major migrate-new migrate-up migrate-down migrate-version

TAG_BUMP ?= patch
TAG_INITIAL_VERSION ?= 0.1.0
TAG_REMOTE ?= origin

deps:
	go mod download
	pnpm --dir web install --frozen-lockfile

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
	go build -trimpath -ldflags="-s -w" -o build/lumi_ctl ./cmd/lumi_ctl

build-linux:
	pnpm --dir web run build
	mkdir -p build/linux-amd64
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -tags embed_frontend -ldflags="-s -w" -o build/linux-amd64/lumi_web ./cmd/lumi_web
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
