# UI Dev Quickstart

## Prereqs
- Node.js 18+ and npm
- Go toolchain

## One‑liners (Makefile)
- Build everything (UI → `web/`, server with embedded UI):
  - `make build`
- Build and run in demo mode (serves `/api/v1/demo/*`):
  - `make demo`
- Run server without rebuilding (uses last build):
  - `make demo-serve`
- Build UI only (set `DEMO=1` to point at demo API):
  - `make ui DEMO=1`
- Install UI deps once (idempotent):
  - `make deps`
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

## Cadence & Testing
- Local loop: implement → `make ui DEMO=1` → `make server` → `make demo-serve`.
- Full validation: `make e2e` (builds and runs Playwright tests).
- Tests fail on any browser console error (caught via Playwright hooks).
- First time only: `make deps` (UI deps) and `make e2e-deps` (Playwright browsers).

## Must-read Docs for UI Sessions
- `docs/ui/screen-inventory.md` — screens/routes and API mapping.
- `docs/ui/traceability.md` — scenarios → screens → APIs.
- `docs/ui/ui-implementation-plan.md` — architecture, milestones, workflow.
- `docs/ui/demo-fixture-index.md` — demo endpoints (success and error).
- `docs/api/openapi.yaml` — API contract (validated; types generated from here).

### About dependency installs
- `make demo` no longer runs Playwright or OS-level dependency installers.
- UI dependencies are installed only when missing or package-lock changes (`make deps`).
- E2E browsers are installed via `make e2e-deps` (no root). Installing system packages for Playwright is optional and should be run manually if needed.
