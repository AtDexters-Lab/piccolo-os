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
