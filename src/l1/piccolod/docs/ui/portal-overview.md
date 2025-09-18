# Piccolo Portal Overview (Pre-Beta)

Quick reference for the current portal IA and the APIs each screen consumes. This replaces the older demo/deck docs and matches the trimmed scope we are polishing for pre-beta.

## Global Notes
- Sign-in handled via `/api/v1/auth/*` with CSRF and rate limiting.
- All navigation must function on mobile (≈360×800) with no horizontal scroll.
- Service links always expose proxy ports; remote hostnames derive from the listener name (`listener.user-domain`).
- Demo fixtures are gone; the portal only talks to the real API.

## Routes & APIs

### `/` — Dashboard
Panels: Overview, Services, Storage, Remote, Updates.
APIs: `GET /api/v1/health`, `GET /api/v1/services`, `GET /api/v1/storage/disks`, `GET /api/v1/remote/status`, `GET /api/v1/updates/os`.

### `/apps`
List installed apps, show status, enable start/stop buttons.
APIs: `GET /api/v1/apps`, `POST /api/v1/apps/:name/start`, `POST /api/v1/apps/:name/stop`.

### `/apps/catalog`
Catalog of curated apps (WordPress today) with install CTA.
APIs: `GET /api/v1/catalog`, `POST /api/v1/apps` (YAML upload/template install).

### `/apps/:name`
App detail: status, service endpoints (disabled link when stopped), uninstall with optional purge.
APIs: `GET /api/v1/apps/:name`, `POST /api/v1/apps/:name/start`, `POST /api/v1/apps/:name/stop`, `DELETE /api/v1/apps/:name?purge=true`.

### `/storage`
Disk inventory, initialize/attach encrypted volumes, set default data root.
APIs: `GET /api/v1/storage/disks`, `GET /api/v1/storage/mounts`, `POST /api/v1/storage/disks/:id/init`, `POST /api/v1/storage/disks/:id/use`, `POST /api/v1/storage/default-root`.

### `/remote`
Configure Nexus endpoint and hostname, view status/cert expiry, disable remote access.
APIs: `GET /api/v1/remote/status`, `POST /api/v1/remote/configure`, `POST /api/v1/remote/disable`.
Notes: Remote URLs publish as `https://<listener>.<user-domain>[:remote_port]`; listeners may declare explicit `remote_ports` or fall back to 80/443.

### `/updates`
Show OS update status, apply or rollback, coordinate reboots.
APIs: `GET /api/v1/updates/os`, `POST /api/v1/updates/os/apply`, `POST /api/v1/updates/os/rollback`.

## Out-of-Scope (Removed)
- Install-to-disk wizard, system events feed, backup UX, and demo fixtures.
- Any additional flows need new acceptance criteria before returning to the portal.

## References
- Product scope: `docs/pre-beta-prd.md` (this repo) and symlinked `piccolo_os_prd.md` (org context).
- App manifest contract: `docs/app-platform/specification.yaml`.
- Acceptance tests: `org-context/02_product/acceptance_features/*.feature`.
