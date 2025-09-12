# Real API Integration Plan (UI ↔ piccolod)

Purpose: Phase the demo UI over to the production backend (`/api/v1`) with strong contract validation, clean fallbacks, and measurable gates. The OpenAPI spec remains the single source of truth.

## Ground Rules
- OpenAPI (`docs/api/openapi.yaml`) is authoritative; every route shipped must be in spec.
- Add operationIds and lint the spec; block breaking changes in CI.
- Server validates requests/responses against OpenAPI (Gin middleware).
- UI keeps `VITE_API_DEMO=1` for routes not yet implemented; flip per‑panel when ready.
- Visual and E2E checks run on demand; contract smokes run on every PR.

## Phase 0 — Foundations
- Spec hygiene: add `operationId`, enable Spectral lint.
- Codegen: use `oapi-codegen` (or kin-openapi) for Go types + validation middleware.
- UI feature flags: runtime check to prefer prod endpoints when server advertises support.
- CI: contract test job (start server; run a minimal Playwright smoke against real `/api/v1`).

Implementation notes
- Server exposes optional request validation against the embedded spec; enable with `PICCOLO_API_VALIDATE=1`.
- The server serves the embedded spec at `GET /api/v1/openapi.yaml` to aid tooling.

Gate: Spec linting + server validation in place; CI green.

## Phase 1 — Auth & Sessions
- Backend: `/auth/session`, `/auth/login`, `/auth/logout`, `/auth/csrf`, `/auth/password`, `/auth/setup` with rate‑limit and secure cookies.
- UI: flip login/setup/session to prod.
- Tests: unit + E2E auth flow.

Gate: UI can sign in/out via real API.

## Phase 2 — Dashboard Data
- Backend: `/health`, `/services`, `/updates/os` (read‑only), `/remote/status`, `/storage/disks` (read‑only).
- UI: dashboard panels per‑panel cutover.
- Tests: unit + Playwright panel rendering and error states.

Gate: Dashboard reads from real API.

## Phase 3 — Apps Management
- Backend: `/apps` list/get, `/apps/{name}/start|stop|update|revert|logs`, `DELETE /apps/{name}?purge`, `/catalog`.
- Container runner: Podman rootless, labels, port allocator, service registry.
- UI: flip Apps list/details/actions and Catalog install.
- Tests: unit with fakes; integration runner; E2E lifecycle.

Gate: Full app lifecycle works via real API.

## Phase 4 — Storage & Encryption
- Backend: `/storage/mounts`, `/storage/default-root`, `/storage/disks/{id}/init|use`, `/storage/recovery-key*`, `/storage/unlock`, encrypt‑in‑place dry‑run/confirm.
- Safety: simulate first; idempotent operations; clear irreversible warnings.
- UI: flip Storage to prod.
- Tests: unit planners; integration temp‑dir; E2E dry‑run/confirm.

Gate: Attach/mount + encryption UX backed by real logic.

## Phase 5 — Updates (OS + Apps)
- Backend: `/updates/os` apply/rollback (single‑flight; 429), `/updates/apps`.
- UI: flip actions to prod.
- Tests: unit controller; E2E simulation.

Gate: Apply/rollback durable, throttled correctly.

## Phase 6 — Remote/Nexus
- Backend: `/remote/status|configure|disable|rotate` with preflights (DNS/80/CAA) and ACME HTTP‑01 over tunnel (device‑terminated TLS).
- UI: flip with clear error guidance.
- Tests: unit preflights; integration with fake Nexus/ACME.

Gate: Configure/disable work end‑to‑end; renewal scheduled.

## Phase 7 — Backup/Restore
- Backend: `/backup/list|export|import`, `/backup/app/{name}`, `/restore/app/{name}`.
- UI: flip Backup screen.
- Tests: unit bundle creation; integration temp‑dir; E2E toasts.

Gate: Config and per‑app bundles round‑trip.

## Phase 8 — Install (Live Mode)
- Backend: `/install/targets|plan|run|fetch-latest`; explicit simulation and confirmation.
- UI: reuse simulation output; strong warnings.
- Tests: integration simulation only in CI.

Gate: Simulation + fetch solid; run gated to live environments.

## Phase 9 — SSO & Gate (optional for UI v1)
- Backend: `/sso/start|consume|keys|logout` ticket flow; gate strips cookies and issues app‑scoped sessions.
- Tests: unit for ticket lifetimes; integration with a fake gate.

Gate: Local SSO verified; staged for remote domains.

## Cross‑Cutting
- Errors: consistent shapes; meaningful 4xx/5xx; `Retry-After` where applicable.
- Security: input validation, context timeouts, rate limiting, least privilege for Podman/system ops.
- Observability: structured logs, `/events`, `/logs/bundle`.
- Feature flags: per‑panel flips; demo endpoints remain until prod is proven.

## Tooling & CI
- OpenAPI lint + breaking‑change detection in PRs.
- `oapi-codegen` for server types/validators; `openapi-typescript` for UI types (already present).
- Playwright smoke targets real server for in‑scope routes; visual tour remains manual/on‑demand.

## Deliverables & Checkpoints
- D0 Foundations → D1 Auth → D2 Dashboard → D3 Apps → D4 Storage → D5 Updates → D6 Remote → D7 Backup → D8 Install (sim) → D9 SSO.

## Risks & Mitigations
- OS‑level operations: simulation paths; guarded by env flags.
- Podman/systemd variability: abstraction + health checks/timeouts.
- ACME/DNS dependencies: staging + fakes; actionable error messages.

## Immediate Next Steps
1) Implement Phase 0 (spec lint + server validation + CI job).
2) Start Phase 1 (Auth endpoints) and flip UI auth to production.
