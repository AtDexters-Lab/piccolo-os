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
- Visual tour (screenshots only): `make e2e-visual` (captures full-page screenshots for key routes to `web-src/test-results/...`).
- Tests fail on any browser console error (caught via Playwright hooks).
- First time only: `make deps` (UI deps) and `make e2e-deps` (Playwright browsers).
- Mobile‑first: Playwright runs on Desktop and Pixel 5; mobile tests assert no horizontal scroll and working nav menu.
  - Mobile screenshots are also captured under `web-src/test-results/mobile-*/m*.png`.

### Running a single test (focused loop)
- By file: `cd web-src && npx playwright test tests/mobile.spec.ts --project=mobile-chromium`
- By name: `cd web-src && npx playwright test --grep "menu button is clearly tappable" --project=mobile-chromium`
- Via Makefile (custom args):
  - `make e2e-one ARGS="tests/mobile.spec.ts --project=mobile-chromium"`
  - `make e2e-one ARGS="--grep 'menu button' --project=mobile-chromium"`
- Mobile project only: `make e2e-mobile`

## Branding & Static Assets
- Place public assets under `web-src/public/...`. Vite serves these at the root in dev and copies them into `web/` for go:embed.
- Branding lives at `web-src/public/branding/` and is reachable at `/branding/...` at runtime.
- Server mapping:
  - Dev override (`PICCOLO_UI_DIR`): serves `/assets` and `/branding` from that directory.
  - Embedded: `/assets` and `/branding` are served from the embedded `web/` directory.
- Tests: navigation/home specs assert the header logo is visible and `/branding/piccolo.svg` returns 200.

## Demo Error Responses (Normalization)
- Demo fixtures may return HTTP 200 with an error-shaped JSON body (e.g., `{ error: "Too Many Requests", code: 429, message: "Try again later" }`).
- The UI client normalizes these as failures even when status is 200, so UI flows show appropriate error toasts and states.
- Production endpoints should return proper 4xx/5xx codes; the same client logic handles both.

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
