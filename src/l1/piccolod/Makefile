SHELL := /bin/bash
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: all ui server build demo demo-serve clean typegen deps e2e e2e-deps

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
	cd web-src && rm -rf dist

# --- E2E testing ---
e2e-deps: ## Install Playwright browser binaries (no root deps)
	cd web-src && npm ci && npx playwright install chromium

e2e: ## Run Playwright tests against demo server with demo UI
	$(MAKE) build DEMO=1
	cd web-src && npm ci && npx playwright install chromium && npm run e2e || true
	node web-src/tools/summarize-playwright.js || true
