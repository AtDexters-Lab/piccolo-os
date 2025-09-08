# UI Traceability: Scenarios → Screens → APIs

Purpose: Ensure every acceptance scenario maps to a visible screen state and API contract. This lets us validate outcomes with mocks first and real integrations later.

## Mapping Overview

- First-run and Unlock → Setup/Login → `POST /auth/setup`, `POST /auth/login`, `GET /auth/session`
- Dashboard overview → Dashboard → `GET /health`, `GET /services`, `GET /storage/disks`, `GET /updates/os`, `GET /remote/status`
- Deploy curated services (Vaultwarden/Gitea/WordPress) → Catalog, Installed Apps, App Details → `POST /apps`, `GET /apps`, `GET /apps/:name`, `GET /services`
- Service discovery and local access → Dashboard, Installed Apps → `GET /services` (includes `local_url`), in-UI “Open locally” links
- Service management & logs → App Details → `POST /apps/:name/{start,stop}`, `GET /apps/:name/logs`, `DELETE /apps/:name?purge=true`, `POST /apps/:name/{update,revert}`
- Storage & encryption → Storage → `GET /storage/disks`, `POST /storage/disks/:id/{init,use}`, `GET /storage/mounts`, `POST /storage/default-root`
- Updates & rollback → Updates → `GET /updates/os`, `POST /updates/os/{apply,rollback}`, `GET /updates/apps`
- Remote publish (HTTP-01, device-terminated TLS) → Remote Access → `POST /remote/configure`, `GET /remote/status`, error/warning states for DNS/ports/CAA
- Observability & errors → Events → `GET /events`, localized error panels throughout
- Auth security (rate limiting, session timeout, anti-double-submit) → Login/Setup + middleware → response codes + UX prompts
- Backup & restore → Settings/Backup → `POST /backup/export`, `POST /backup/import`, `GET /backup/list`
- Install to Disk (Live USB) → Install Wizard → `GET /install/targets`, `POST /install/plan`, `POST /install/run`, `POST /install/fetch-latest`

## Acceptance Coverage Notes

- Each scenario has at least one visible success path and clear error feedback.
- Demo mode fixtures cover success states first; add targeted error fixtures for preflight failures (e.g., DNS not set, port 80 closed).
- Time-bound goals (≤ 60s portal, ≤ 5–10m app deploy) surface as progress indicators in the Dashboard and App Details.
- See also: [Demo Fixture Index](./demo-fixture-index.md) for endpoint references per scenario.
