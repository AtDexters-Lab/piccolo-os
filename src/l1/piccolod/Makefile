SHELL := /bin/bash
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: all ui server build demo clean typegen

all: build

ui:
	@echo "==> Building UI (demo=$(DEMO))"
	cd web-src && \
	  npm install && \
	  npm run typegen && \
	  VITE_API_DEMO=$(DEMO) npm run build

server:
	@echo "==> Building piccolod (version=$(VERSION))"
	go build -ldflags "-X main.version=$(VERSION)" -o piccolod ./cmd/piccolod

build: ui server
	@echo "==> Build complete: ./piccolod with embedded ./web"

demo: DEMO=1
demo: build
	@echo "==> Running piccolod in demo mode on http://localhost:8080"
	PORT=8080 PICCOLO_DEMO=1 ./piccolod

typegen:
	cd web-src && npm run typegen

clean:
	rm -rf web/* piccolod
	cd web-src && rm -rf node_modules dist

.PHONY: e2e
e2e: build
	cd web-src && npm install && npx playwright install chromium --with-deps && npm run e2e
