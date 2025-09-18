SHELL := /bin/bash
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
DEMO ?= 0
RUN_PORT ?= 8080
RUN_STATE_DIR ?= $(CURDIR)/run-state

.PHONY: all ui server build run release demo demo-serve clean typegen deps e2e e2e-real e2e-demo e2e-deps e2e-one e2e-mobile e2e-visual e2e-visual-one

all: build

# --- Dependencies (install once) ---
deps: web-src/node_modules/.stamp ## Install UI dependencies once (idempotent)

web-src/node_modules/.stamp: web-src/package.json web-src/package-lock.json
	@echo "==> Installing UI dependencies (npm ci)"
	cd web-src && npm ci
	@touch $@

web-src/src/api/types.ts: docs/api/openapi.yaml | deps
	@echo "==> Generating API types from OpenAPI"
	cd web-src && npm run typegen

# --- Build steps ---
ui: web-src/src/api/types.ts ## Build UI to ./web
	@echo "==> Building UI (demo=$(DEMO))"
	cd web-src && VITE_API_DEMO=$(DEMO) npm run build

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
typegen: web-src/src/api/types.ts ## Regenerate API types

clean:
	rm -rf web/* piccolod web-src/node_modules/.stamp
	rm -rf run-state .e2e-state
	cd web-src && rm -rf dist test-results playwright-report

# --- E2E testing ---
e2e-deps: ## Install Playwright browser binaries (no root deps)
	cd web-src && npm ci && npx playwright install chromium

e2e: e2e-real ## Run Playwright tests (default: real API)

# Run only a subset of tests by passing ARGS, e.g.:
#   make e2e-one ARGS="tests/mobile.spec.ts --project=mobile-chromium --grep 'menu button'"
e2e-one: ## Run Playwright with custom args (ARGS="...") against real API
	$(MAKE) build DEMO=0
	rm -rf .e2e-state && mkdir -p .e2e-state
	cd web-src && npm ci && npx playwright install chromium && npx playwright test --config=playwright.real.config.ts $(ARGS)

e2e-mobile: ## Run only mobile project tests
	$(MAKE) build DEMO=0
	rm -rf .e2e-state && mkdir -p .e2e-state
	cd web-src && npm ci && npx playwright install chromium && npx playwright test --config=playwright.real.config.ts --project=mobile-chromium

# Visual tour: capture screenshots across key routes (no reinstall)
e2e-visual: ## Generate visual screenshots (desktop+mobile) for real API
	$(MAKE) build DEMO=0
	rm -rf .e2e-state && mkdir -p .e2e-state
	cd web-src && npm ci && npx playwright install chromium && npx playwright test tests/visual.spec.ts --config=playwright.real.config.ts --project=chromium --project=mobile-chromium || true

# Visual tour for a single project or with custom args
e2e-visual-one: ## Run visual tour with ARGS (e.g., --project=chromium)
	$(MAKE) build DEMO=0
	rm -rf .e2e-state && mkdir -p .e2e-state
	cd web-src && npm ci && npx playwright install chromium && npx playwright test tests/visual.spec.ts --config=playwright.real.config.ts $(ARGS)

# Real-auth E2E (default)
.PHONY: e2e-real
e2e-real: ## Run Playwright against real server (auth/setup enforced)
	$(MAKE) build DEMO=0
	rm -rf .e2e-state && mkdir -p .e2e-state
	cd web-src && npm ci && npx playwright install chromium && npx playwright test --config=playwright.real.config.ts || true
	node web-src/tools/summarize-playwright.js || true

e2e-demo: ## Run legacy demo-mode Playwright suite (stubs enabled)
	$(MAKE) build DEMO=1
	cd web-src && npm ci && npx playwright install chromium && npx playwright test || true
	node web-src/tools/summarize-playwright.js || true
