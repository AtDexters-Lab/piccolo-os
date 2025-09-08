# UI Dev Quickstart

## Prereqs
- Node.js 18+ and npm
- Go toolchain

## One‑liners (Makefile)
- Build everything (UI → `web/`, server with embedded UI):
  - `make build`
- Build and run in demo mode (serves `/api/v1/demo/*`):
  - `make demo`
- Build UI only (set `DEMO=1` to point at demo API):
  - `make ui DEMO=1`
- Build server only (injects version from git):
  - `make server`
- Regenerate API types from OpenAPI:
  - `make typegen`
- Clean artifacts:
  - `make clean`

## Dev server (optional)
- Build UI and serve from disk via `PICCOLO_UI_DIR`:
  - UI: `cd web-src && npm install && VITE_API_DEMO=1 npm run build`
  - Server: `PICCOLO_UI_DIR=$(pwd)/web-src/dist PICCOLO_DEMO=1 ./piccolod`

Notes
- Demo endpoints and fixtures: see `docs/ui/demo-fixture-index.md`.
- API contract: `docs/api/openapi.yaml` (validated in tests).
- OpenAPI types: generated into `web-src/src/api/types.ts` via `make typegen`.
