# distributed-key-value-store — local development gate.
#
# CI runs only on `dev` -> `main` PRs, so `make check` is the gate that
# feature branches rely on. Until `go.mod` exists, the Go toolchain steps are
# SKIPPED (a skipped run is not a passed run), keeping scaffolding PRs usable.

GO ?= go
NPM ?= npm
WEB_DIR ?= frontend

.PHONY: help check fmt vet test test-integration build run clean \
        web-install web-build web-dev web-lint web-test web-check \
        cluster-smoke cluster-up cluster-down cluster-logs \
        preflight dev-up dev-down dev-restart dev-log dev-build reset \
        shell opencode opencode-auto dev-shell dev-opencode dev-opencode-auto

# Extra flags for the opencode invocation. `make opencode-auto` /
# `make dev-opencode-auto` set this to --auto, which auto-approves permissions
# that are not explicitly denied; the secret-file deny rules still apply.
OPENCODE_ARGS ?=

help: ## List available targets
	@grep -E '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

## check: gofmt + vet + test + build (the local gate before pushing)
check:
	@if [ ! -f go.mod ]; then \
	  echo "==> no go.mod yet — Go toolchain checks SKIPPED (skipped is not passed)"; \
	else \
	  $(MAKE) fmt vet test build; \
	fi

fmt:
	@echo "==> gofmt"
	@out="$$(gofmt -l .)"; \
	if [ -n "$$out" ]; then echo "unformatted files:"; echo "$$out"; exit 1; fi

vet:
	@echo "==> go vet"
	$(GO) vet ./...

test:
	@echo "==> go test"
	$(GO) test ./...

## test-integration: run opt-in integration tests (leader failure + no quorum)
test-integration:
	@echo "==> go test -tags=integration"
	$(GO) test -tags=integration ./...

build:
	@echo "==> go build"
	$(GO) build ./...

clean:
	@echo "cleaning"
	rm -f ./kvnode

# --- Dashboard (frontend/ -> backend/internal/webui/dist) ----------------------
# The Go binary embeds backend/internal/webui/dist via //go:embed. Until
# `make web-build` runs, that directory only holds placeholder.html (committed).

## web-install: install frontend dependencies from the lockfile
web-install:
	@echo "==> npm ci ($(WEB_DIR))"
	cd $(WEB_DIR) && $(NPM) ci

## web-install-update: resolve deps and refresh package-lock.json
web-install-update:
	@echo "==> npm install ($(WEB_DIR))"
	cd $(WEB_DIR) && $(NPM) install

web-dev: ## Run the Vite dev server (proxies to a locally running node)
	cd $(WEB_DIR) && $(NPM) run dev

web-build: ## Build the dashboard into backend/internal/webui/dist for embedding
	@echo "==> vite build ($(WEB_DIR))"
	cd $(WEB_DIR) && $(NPM) run build

web-lint: ## Lint/format-check the frontend
	cd $(WEB_DIR) && $(NPM) run lint && $(NPM) run format:check

web-test: ## Run frontend unit tests
	cd $(WEB_DIR) && $(NPM) test

## web-check: frontend gate — install, lint, test, build (used by CI)
web-check:
	@$(MAKE) web-install
	cd $(WEB_DIR) && $(NPM) run lint && $(NPM) run format:check \
	  && $(NPM) test && $(NPM) run build

# --- 3-node cluster ------------------------------------------------------------
# cluster-smoke needs only Go + curl (no docker) and runs the Done-when check:
# write, kill the leader, read the committed value from the new leader.
# cluster-* uses deploy/docker-compose.yml.

## cluster-smoke: run the 3-node Done-when check with local processes
cluster-smoke:
	scripts/cluster-smoke.sh

cluster-up: ## Build and start the 3-node cluster (needs docker)
	docker compose -f deploy/docker-compose.yml up -d --build

cluster-down: ## Stop the 3-node cluster
	docker compose -f deploy/docker-compose.yml down

cluster-logs: ## Follow the 3-node cluster logs
	docker compose -f deploy/docker-compose.yml logs -f

run:
	$(GO) run ./backend/cmd/kvnode

# --- Dev sandbox (docker compose) ----------------------------------------------

preflight: ## Check host prerequisites for the dev sandbox
	@test -x "$(HOME)/.opencode/bin/opencode" || { echo "ERROR: opencode not found at ~/.opencode/bin/opencode" >&2; exit 1; }
	@mkdir -p "$(HOME)/.config/opencode" "$(HOME)/.config/gh" && touch "$(HOME)/.gitconfig"
	@scripts/resolve-gh-token.sh --check >/dev/null 2>&1 || true

dev-up: preflight ## Build and start the dev sandbox
	@HOST_UID=$$(id -u) HOST_GID=$$(id -g) GH_TOKEN=$$(scripts/resolve-gh-token.sh 2>/dev/null) GITHUB_TOKEN=$$(scripts/resolve-gh-token.sh 2>/dev/null) docker compose up -d --build

dev-restart: preflight ## Restart the dev container (after Dockerfile/deps changes)
	@docker compose restart dev

dev-down: ## Stop and remove the dev sandbox
	@docker compose down

dev-log: ## Follow dev container logs
	@docker compose logs -f dev

dev-build: preflight ## Rebuild the dev image without starting
	@docker compose build

reset: ## Stop dev sandbox and remove volumes
	@docker compose down -v

shell: dev-shell ## Alias for dev-shell
opencode: dev-opencode ## Alias for dev-opencode (run opencode inside the sandbox)
opencode-auto: dev-opencode-auto ## Alias for dev-opencode-auto (run opencode with --auto)

dev-shell: preflight ## Open a shell in the dev container
	@docker compose exec dev bash || docker compose run --rm dev bash

dev-opencode: preflight ## Run opencode inside the dev sandbox
	@if [ -z "$$(docker compose ps -q dev 2>/dev/null)" ]; then echo "[opencode] starting dev stack..."; HOST_UID=$$(id -u) HOST_GID=$$(id -g) GH_TOKEN=$$(scripts/resolve-gh-token.sh 2>/dev/null) GITHUB_TOKEN=$$(scripts/resolve-gh-token.sh 2>/dev/null) docker compose up -d dev; fi
	@if [ -t 0 ]; then docker compose exec -it dev zsh -lc "cd /workspace && opencode $(OPENCODE_ARGS)" || docker compose exec -it dev bash -lc "opencode $(OPENCODE_ARGS)"; else docker compose exec -T dev zsh -lc "cd /workspace && opencode $(OPENCODE_ARGS)" || docker compose exec -T dev bash -lc "opencode $(OPENCODE_ARGS)"; fi

# Same recipe, with --auto: auto-approve permissions that are not explicitly
# denied (every secret-file deny rule still applies).
dev-opencode-auto: OPENCODE_ARGS = --auto
dev-opencode-auto: dev-opencode ## Run opencode inside the dev sandbox with --auto