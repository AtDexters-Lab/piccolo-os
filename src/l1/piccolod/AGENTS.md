# Repository Guidelines

## Project Structure & Module Organization
- `cmd/piccolod/`: Daemon entrypoint (`main.go`).
- `internal/`: Packages for server, app management, mdns, storage, etc.; tests live next to code as `*_test.go`.
- `web/`: Static assets for the minimal UI.
- `docs/`: App platform docs and examples.
- `testdata/`: Fixtures for unit/integration tests.
- `example-apps/`: Example app specs.

## Architecture Overview
- Three-layer networking: container → 127.0.0.1 bind → public proxy.
- Service-oriented model (planned): named `listeners` with auto port allocation and middleware.
- Managers: server, app (FS-backed), container (Podman), mdns, ecosystem; future service/proxy managers.
- UI: static SPA served by Gin; API on port 80; systemd `SdNotify` readiness.

## Build, Test, and Development Commands
- Build daemon: `go build ./cmd/piccolod`.
- Inject version: `go build -ldflags "-X main.version=1.0.0" ./cmd/piccolod`.
- Run tests (all packages): `go test ./...`.
- Coverage quick check: `go test -cover ./...`.
- Optional script build: `./build.sh 1.0.0` (writes binary with version).

## Coding Style & Naming Conventions
- Go formatting: run `go fmt ./...` before committing.
- Package layout: keep code under `internal/<domain>` (e.g., `internal/app`, `internal/mdns`).
- Tests: table‑driven, `*_test.go`, colocated with sources.
- Versioning: pass via `-ldflags -X main.version` at build time.

## Testing Guidelines
- Framework: standard Go `testing` with subtests and table patterns.
- Unit tests: prefer small, isolated tests with fakes/mocks under `internal/*`.
- Integration tests: marked in packages like `internal/app` and `internal/mdns`; may use fixtures in `testdata/`.
- Run: `go test ./...`; add coverage for new logic.

## Commit & Pull Request Guidelines
- Commits: follow Conventional Commits, e.g., `feat(server): add API`, `fix(container): handle errors`, `docs:`.
- PRs include: goal/summary, scope (affected packages), linked issues, test plan with `go test` output, and any UI snapshots under `web/` when relevant.

## Security & Configuration Tips
- Do not commit secrets or signing keys.
- Favor least privilege; avoid expanding container/host capabilities without review.
- Validate inputs at API boundaries; prefer context timeouts on I/O and network calls.
- Uninstall semantics: `DELETE /api/v1/apps/:name` keeps data by default; add `?purge=true` to also remove app data under `/var/piccolo/storage/<app>/...` and `/tmp/piccolo/apps/<app>/...` (including explicit host paths in `app.yaml`). Upserts are non-destructive and never purge.

## App Spec Alignment
- Source of truth: `docs/app-platform/specification.yaml`.
- Supported now: `name`, `image|build` (xor), `listeners` (v1), `storage`, `resources`, `permissions` (validated in `internal/app/parser.go`).
 - Not yet implemented: protocol middleware processing, full service discovery features, build pipeline (multipart/Git), per‑app healthchecks, `depends_on`, filesystem persistence/RO root enforcement, detailed network policy.
- Security defaults: container ports bind to `127.0.0.1`; `permissions.network.internet: deny` maps to `--network none`.
- Install API: `POST /api/v1/apps` with YAML body; fixtures in `testdata/apps`.

## Local Dev Notes
- Run daemon: `go run -ldflags "-X main.version=dev" ./cmd/piccolod`.
- Lint/format quickly: `go vet ./... && go fmt ./...`.

## UI Development Cadence (Piccolo Web)

Must-read docs when starting a UI session:

- `docs/ui/screen-inventory.md` — screens and routes.
- `docs/ui/traceability.md` — acceptance scenarios mapped to screens/APIs.
- `docs/ui/ui-implementation-plan.md` — architecture, milestones, workflow.
- `docs/ui/bfs-tree.md` — breadth-first plan (levels, status, mapping).
- `docs/ui/demo-fixture-index.md` — demo endpoints for happy/error paths.
- `docs/api/openapi.yaml` — API contract and source of generated types.
- Quickstart commands: see `docs/ui/dev.md` (one‑liners, workflow, notes).
- Mobile-first guidelines: see `docs/ui/mobile-first.md` (layout, overflow, touch, testing).

Commands:

- First-time setup: `make deps` (UI deps), `make e2e-deps` (Playwright browsers).
- Dev loop: `make ui DEMO=1 && make server && make demo-serve`.
- Full validation: `make e2e` (builds and runs Playwright against the real API; remote is stubbed by default).
- Full remote lane (CI or local): set `E2E_REMOTE_STACK=1` to run against a local Nexus+Pebble stack without fake ACME.

Notes:

- Playwright tests are configured to fail on any browser console error to catch regressions early.
- UI builds to `web/` and is embedded via go:embed; override via `PICCOLO_UI_DIR` for local serving.
- E2E testing policy lives at `docs/testing/e2e-policy.md`.

## Integration Plan (UI → Real API)

- The phased plan to cut the demo UI over to the production backend is documented in `docs/real-api-integration-plan.md`.
- Follow phase gates (D0..D9), keep `VITE_API_DEMO=1` for routes not yet implemented, and flip panels to prod only after the backend route lands with tests and CI contract checks.

## Org Context (Product)
- Base dir: `$HOME/projects/piccolo/org-context/02_product` (source product context for Piccolo OS).
- PRD: `$HOME/projects/piccolo/org-context/02_product/piccolo_os_prd.md`.
- Acceptance features dir: `$HOME/projects/piccolo/org-context/02_product/acceptance_features/` containing:
  - `authentication_security.feature`
  - `backup_and_restore.feature`
  - `dashboard_and_navigation.feature`
  - `deploy_curated_services.feature`
  - `first_run_and_unlock.feature`
  - `install_to_disk_x86.feature`
  - `nexus_server_certificates.feature`
  - `observability_and_errors.feature`
  - `remote_publish.feature`
  - `responsive_ui.feature`
  - `security_defaults_and_networking.feature`
  - `service_discovery_and_local_access.feature`
  - `service_management_and_logs.feature`
  - `sso_continuity.feature`
  - `storage_and_encryption.feature`
  - `updates_and_rollback.feature`
  - `README.md`

## Bug‑Fix Workflow (Test‑First + RCA)
- Principle: fixes are test‑driven and root‑cause oriented; we avoid one‑off patches.
- Steps:
  - Add a failing test in the closest package (unit preferred). Use `testdata/` for fixtures and update OpenAPI tests when API shape is involved.
  - Reproduce and instrument: rely on existing logs/health/events; do not over‑log in production paths.
  - Apply a minimal, layered fix (respect `internal/*` boundaries: server ↔ app/services ↔ persistence ↔ remote). Keep policy in the right module.
  - Turn the suite green (`go test ./...`) and extend with boundary/edge tests covering sibling flows uncovered by the RCA.
  - Document RCA in the PR (template below) and, if systemic, update docs under `docs/runtime/*` or `docs/persistence/*` with the refined approach.
- Commits/PRs:
  - Conventional Commits (`fix(app):`, `test(server):`, `refactor(persistence):`, `docs:`).
  - PR must contain: failing test, fix, RCA, extrapolated tests, and test outputs.
- RCA template:
  - Symptom → concise description and reproduction.
  - Root cause → exact code path/assumption.
  - Fix → what changed and why it is correct.
  - Extrapolation → related flows now covered by tests.
  - Follow‑ups → design/doc tickets if architecture needs adjustment.
