# UI Screen Inventory and Routes

Purpose: Freeze the initial UI based on the PRD and acceptance features, mapping each screen to primary user actions and API contracts. This guides development toward concrete, user-visible outcomes and enables a complete mock-driven demo.

## Screens and Routes

- Login / First-Run Setup (`/setup`, `/login`)
  - Create admin, sign in, idle timeout, logout
  - States: first-run gate, invalid creds, rate-limited, session expired
  - APIs: `POST /api/v1/auth/setup`, `POST /api/v1/auth/login`, `POST /api/v1/auth/logout`, `GET /api/v1/auth/session`

- Dashboard (`/`)
  - System summary: network, storage, services, updates, remote access
  - Primary actions: Add disk, Deploy service, Enable remote access
  - APIs: `GET /api/v1/health`, `GET /api/v1/services`, `GET /api/v1/storage/disks`, `GET /api/v1/updates/os`, `GET /api/v1/remote/status`

- App Catalog (`/apps/catalog`)
  - Browse curated apps; search/filter; install CTA
  - APIs: `GET /api/v1/catalog`, `POST /api/v1/apps` (YAML upload)

- Installed Apps (`/apps`)
  - List installed apps; status; quick actions (start/stop/restart)
  - APIs: `GET /api/v1/apps`, `POST /api/v1/apps/:name/{start,stop}`

- App Details (`/apps/:name`)
  - Overview: listeners, URLs, permissions, storage, env
  - Actions: update, revert, uninstall (with data purge option)
  - Logs: recent tail and download bundle
  - APIs: `GET /api/v1/apps/:name`, `GET /api/v1/apps/:name/logs`, `DELETE /api/v1/apps/:name?purge=true`, `POST /api/v1/apps/:name/update`, `POST /api/v1/apps/:name/revert`

- Storage (`/storage`)
  - Disks: model/size/health; actions Use as-is, Initialize
  - Mounts and default data root; attach to services
  - APIs: `GET /api/v1/storage/disks`, `POST /api/v1/storage/disks/:id/init`, `POST /api/v1/storage/disks/:id/use`, `GET /api/v1/storage/mounts`, `POST /api/v1/storage/default-root`

- Updates (`/updates`)
  - OS updates: available/current, apply, rollback, reboot coordination
  - App updates: list, apply, revert
  - APIs: `GET /api/v1/updates/os`, `POST /api/v1/updates/os/apply`, `POST /api/v1/updates/os/rollback`, `GET /api/v1/updates/apps`, `POST /api/v1/apps/:name/{update,revert}`

- Remote Access (`/remote`)
  - Nexus config: endpoint, creds, hostname; preflight checks; status & renewals
  - APIs: `GET /api/v1/remote/status`, `POST /api/v1/remote/configure`, `POST /api/v1/remote/disable`

- Logs & Events (`/events`)
  - Recent actions and errors across system and apps; export bundle
  - APIs: `GET /api/v1/events`, `GET /api/v1/logs/bundle`

- Settings (`/settings`)
  - Network defaults, telemetry, security policies
  - APIs: `GET/POST /api/v1/settings`

- Install to Disk (Live USB only) (`/install`)
  - Wizard: select target, preview contents, confirm by typing id, simulate/fetch latest
  - APIs: `GET /api/v1/install/targets`, `POST /api/v1/install/plan`, `POST /api/v1/install/run`, `POST /api/v1/install/fetch-latest`

## State Matrix (per screen)

- Global: loading, empty, error, success; offline network and mdns-troubleshooting hints
- Dashboard sections load independently; errors localized per panel
- Actions confirm with clear irreversible warnings (e.g., purge data, format disk)

## Notes

- All routes have mock equivalents under `/api/v1/demo/*` to support UI development without live backends.
- JSON shapes are mirrored in `docs/api/openapi.yaml` and fixtures in `testdata/api/`.
- See also: [Demo Fixture Index](./demo-fixture-index.md) for quick QA endpoints.

## SSO & App Access

- Gate in front of every app: reverse proxy enforces auth, performs SSO, and strips sensitive headers before forwarding to containers.
- Portal cookie isolation: portal session cookie is scoped to the portal origin only; apps never receive or read it.
- Ticket SSO flow: gate → 302 to portal `/sso/start?app=<id>&return=<url>` → portal issues one‑time code → gate calls `/sso/consume` → mint app‑scoped cookie (`app_<name>_session`).
- Endpoints: `/sso/start`, `/sso/consume`, `/sso/keys`, `/sso/logout` (see OpenAPI + Demo Index).
- Local vs remote: works for HTTP (local) and HTTPS (remote). Gate always strips Cookie/Authorization when proxying to apps. Per‑app `.local` subdomains can be added later for stricter origin isolation.
