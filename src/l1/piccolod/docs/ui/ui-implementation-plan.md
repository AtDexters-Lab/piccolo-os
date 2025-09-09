# UI Implementation Plan

Purpose: Build a small, fast SPA that fulfills all acceptance scenarios using the frozen API and demo fixtures first, then switch to production API with minimal changes.

## Goals
- Outcome‑driven: implement every scenario with visible UI states (loading/empty/error/success).
- Contract‑first: generate types from `docs/api/openapi.yaml`; no ad‑hoc shapes.
- Mock‑first: develop against `/api/v1/demo/*`; flip to `/api/v1` for prod.
- Security: cookie‑based auth (HttpOnly, SameSite=Lax) + CSRF; portal cookie never reaches apps; SSO handshake via gate.
- Mobile‑first: primary target is mobile; design and tests prioritize small viewports.
- Performance: <100KB gzipped initial JS; route code‑splitting.

## Architecture
- SPA served statically by `piccolod` (go:embed baseline; dev override via `PICCOLO_UI_DIR`).
- Framework: Svelte + Vite; Tailwind for styling.
- Data: TanStack Query (svelte-query) for fetching, cache, retries, polling.
- API types: generated from OpenAPI; centralized fetch wrapper with CSRF and error normalization.
- Routing: file/folder routes with code‑split chunks per screen.
- Mobile foundation: responsive header with menu toggle; card‑first layouts; overflow handled locally (see `docs/ui/mobile-first.md`).

Related docs
- Screen inventory: `docs/ui/screen-inventory.md`
- Traceability map: `docs/ui/traceability.md`
- Demo endpoints: `docs/ui/demo-fixture-index.md`
- API contract: `docs/api/openapi.yaml`

## Project Structure (web/)
- `routes/`: `/`, `/apps`, `/apps/[name]`, `/storage`, `/updates`, `/remote`, `/install`, `/backup`, `/settings`, `/login`, `/setup`
- `components/`: Button, Card, Table, Dialog, Alert, LogsViewer, Wizard, K/VList
- `api/`: `types.ts` (generated), `client.ts` (fetch+CSRF), `endpoints.ts` (helpers)
- `stores/`: `session.ts`, `ui.ts`
- `lib/`: `queryClient.ts`, `formatters.ts`, `links.ts` (build local_url), `featureFlags.ts`
- `styles/`: Tailwind config and tokens

## Data & API Client
- Base URL: `/api/v1` (prod), `/api/v1/demo` (demo) via build flag `VITE_API_DEMO=1`.
- CSRF: fetch `/auth/csrf` once and attach `X-CSRF-Token` to mutating requests.
- Error normalization: map server errors to `{ code, message, next_step? }`.
- Auth handling: central 401/403/429 interceptors (login redirect, unlock gate, Retry‑After UI).

## Auth & SSO Model
- Portal cookie: HttpOnly, SameSite=Lax; Secure on HTTPS, not Secure on HTTP (local).
- App cookie (per‑app): `app_<name>_session`, HttpOnly, origin‑scoped at the gate; never shared with portal.
- SSO (ticket): gate 302 → `/sso/start?app=<id>&return=<url>` → portal issues one‑time code → gate POST `/sso/consume` → sets app cookie → redirect.
- Gate always strips Cookie/Authorization when proxying to apps.

## Screens & Routes (state matrix on each)
- Auth: `/setup`, `/login` (401/429 states), logout; idle timeout.
- Dashboard: health, services, storage, updates, remote status; independent error panels.
- Apps: list; details (listeners, URLs, storage, env); start/stop/update/revert/uninstall; logs (tail + bundle).
- Discovery: “Open locally” using `local_url` or `host_port`.
- Updates: OS (status/apply/rollback), app updates.
- Remote: configure wizard (DNS/80/CAA errors), status/renewal, rotate/disable.
- Storage: disks (init/use), mounts, default root; unlock/recovery key; encrypt‑in‑place.
- Backup/Restore: config export/import; per‑app backup/restore.
- Install: targets preview, plan simulate, run, fetch‑latest with verify error.

## Testing
- E2E (Playwright): one test per acceptance scenario using demo endpoints; include critical error paths.
- Unit (Vitest): component logic (LogsViewer, Wizard, Tables).
- Contract: kin‑openapi validation (added) + openapi‑typescript type gen check in CI.
- Performance: bundle size check and optional Lighthouse CI.
- Mobile‑first: run suites on Desktop Chrome and Pixel 5; fail tests on any console error; assert no page‑level horizontal scroll on key routes.

## Milestones
- M0 Scaffold (Day 1): Vite+Svelte, Tailwind, routing, query client, session store, API client, demo toggle.
- M1 Core (Days 2–3): Dashboard (read), Apps list/details (read + start/stop), Logs (read); Updates/Remote status (read); initial E2E.
- M2 Management (Days 4–5): App update/revert/uninstall, Logs bundle; Remote configure wizard; Storage flows; full E2E coverage.
- M3 Install/Backup (Day 6): Install wizard; config + per‑app backup/restore; A11y/responsive/copy.
- M4 Cutover (Day 7): Build to `web/` for go:embed; flip demo → prod; sanity against real endpoints.

## Dev Workflow
- Server: `PICCOLO_DEMO=1 ./piccolod` for demo data; `PICCOLO_UI_DIR=/abs/path/to/web/dist` during UI dev.
- UI: `VITE_API_DEMO=1` for demo mode; otherwise prod (`/api/v1`).
- Type regen (CI or local): openapi‑typescript `docs/api/openapi.yaml` → `web/api/types.ts`.

## Build & Deploy
- Build UI → `web/` (embedded by go:embed).
- Cache: long‑cache hashed assets; `index.html` no‑cache.
- Optional later: /var override bundle with signature + compat check and rollback.

## Risks & Mitigations
- API/UI drift → type gen + spec validation in CI.
- Long‑running ops complexity → unified progress/polling helpers + clear cancel/retry.
- Security regressions → HttpOnly cookies, CSRF on all mutations, CSP, strict CORS, gate stripping.
