SHELL := /bin/bash
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
DEMO ?= 0
RUN_PORT ?= 8080
RUN_STATE_DIR ?= $(CURDIR)/run-state

.PHONY: all deps ui server build run release demo demo-serve clean typegen e2e

all: build

# --- Dependencies (install once) ---
UI_DIR := ui-next

deps: $(UI_DIR)/node_modules/.stamp ## Install UI dependencies once (idempotent)

$(UI_DIR)/node_modules/.stamp: $(UI_DIR)/package.json $(UI_DIR)/package-lock.json
	@echo "==> Installing UI dependencies (npm ci)"
	cd $(UI_DIR) && npm ci
	@touch $@

# --- Build steps ---
ui: deps ## Build UI to ./web
	@echo "==> Building UI (demo=$(DEMO))"
	cd $(UI_DIR) && VITE_API_DEMO=$(DEMO) npm run build

server: ## Build piccolod with embedded ./web
	@echo "==> Building piccolod (version=$(VERSION))"
	go build -ldflags "-X main.version=$(VERSION)" -o piccolod ./cmd/piccolod

build: ui server
	@echo "==> Build complete: ./piccolod with embedded ./web"

# --- Run targets ---
run: build ## Build (non-demo) and run piccolod locally
	@echo "==> Running piccolod on http://localhost:$(RUN_PORT) (state dir: $(RUN_STATE_DIR))"
	mkdir -p "$(RUN_STATE_DIR)"
	PORT=$(RUN_PORT) PICCOLO_STATE_DIR="$(RUN_STATE_DIR)" ./piccolod

run-fresh: build ## Build and run piccolod with a temporary state dir
	@echo "==> Running piccolod on http://localhost:8080 with a fresh ephemeral state dir"
	@set -euo pipefail; tmpdir="$$(mktemp -d)"; \
	  echo "   state dir $$tmpdir"; \
	  cleanup() { sleep 2; rm -rf "$$tmpdir"; }; \
	  trap cleanup EXIT; \
	  PORT=8080 PICCOLO_STATE_DIR="$$tmpdir" ./piccolod

release: clean deps typegen ## Produce a clean release build (non-demo)
	$(MAKE) build DEMO=0
	@echo "==> Release build available at ./piccolod"

demo: DEMO=1
demo: build ## Build (demo UI) and run server on :8080
	@echo "==> Running piccolod in demo mode on http://localhost:8080"
	PORT=8080 PICCOLO_DEMO=1 ./piccolod

demo-serve: ## Run the server using existing build (no rebuild)
	@echo "==> Serving existing build on http://localhost:8080"
	PORT=8080 PICCOLO_DEMO=1 ./piccolod

# --- Utilities ---
typegen: ## Regenerate API types (not required yet)
	@true

clean:
	rm -rf web/* piccolod $(UI_DIR)/node_modules/.stamp
	rm -rf run-state .e2e-state
	cd $(UI_DIR) && rm -rf .svelte-kit build test-results playwright-report

e2e:
	@$(UI_DIR)/scripts/run-e2e-with-server.sh e2e

# Removed legacy demo and separate real config; unified on single config.
