# Repository Guidelines

## Project Structure & Module Organization
- `src/l0`: OS image build (KIWI). Key files: `build.sh`, `build_piccolo.sh`, `test_piccolo_os_image.sh`, `kiwi/`, `piccolo.env`, outputs in `src/l0/releases/<version>/`.
- `src/l1/piccolod`: Go daemon and HTTP API. Entry: `cmd/piccolod/main.go`; packages under `internal/`; tests as `*_test.go`.
- `src/l2`: Reserved for runtime components (currently minimal).
- `docs/`: Architecture, development, security, and operations guides.

## Build, Test, and Development Commands
- Build OS image: `cd src/l0 && ./build.sh [dev|prod]` — builds `piccolod`, generates MicroOS image, and (dev) runs smoke tests. Artifacts: `src/l0/releases/<version>/`.
- Test built image: `cd src/l0 && ./test_piccolo_os_image.sh --build-dir ./releases/1.0.0 --version 1.0.0` — boots in QEMU and validates services.
- Build daemon only: `cd src/l1/piccolod && ./build.sh 1.0.0` or `go build ./cmd/piccolod`.
- Run tests: `cd src/l1/piccolod && go test ./...` (coverage: `go test -cover ./...`).

## Coding Style & Naming Conventions
- Go: `gofmt`/`go fmt` before committing; idiomatic package layout; table‑driven tests; version injected via `-ldflags -X main.version`.
- Shell: Bash with `set -euo pipefail`; executable `.sh` files; kebab‑case filenames.
- Naming: use clear scopes (e.g., `internal/app`, `internal/mdns`); tests end with `_test.go`.

## Testing Guidelines
- Unit tests: `go test ./...` for all packages; prefer small, isolated tests and mocks.
- Integration: tests under `internal/*` may require Podman/QEMU; ensure dependencies are installed.
- System: use `src/l0/test_piccolo_os_image.sh` for end‑to‑end validation.
- Add tests for new logic and keep/raise coverage; place fixtures under `testdata/`.

## Commit & Pull Request Guidelines
- Commits: follow Conventional Commits seen in history, e.g., `feat(server): add API`, `fix(container): handle errors`, `build(l0): update KIWI config`, `docs:`.
- PRs: include goal/summary, scope (`l0`/`l1`), linked issues, test plan and outputs (e.g., `go test`/smoke test logs), and impact notes. Screenshots optional for web assets under `src/l1/piccolod/web/`.

## Security & Configuration Tips
- Do not commit secrets or signing keys; configure locally in `src/l0/piccolo.env`.
- Prefer least privilege; avoid introducing new host capabilities in services without review.

## Integration Plan (UI ↔ Backend)
- For the `src/l1/piccolod` daemon and web UI, see `src/l1/piccolod/docs/real-api-integration-plan.md` for the phased cutover from demo UI to the real API.

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
