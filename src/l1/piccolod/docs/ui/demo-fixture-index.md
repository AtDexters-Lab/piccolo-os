# Demo Fixture Index (Acceptance → Endpoints)

Purpose: Quickly exercise success and error states for each acceptance scenario using demo mode fixtures. Start piccolod with `PICCOLO_DEMO=1` and hit these endpoints under `/api/v1/demo/*`.

## Auth & First‑Run
- Setup: `/auth/setup`
- Login (ok): `/auth/login`
- Login (401): `/auth/login_failed`
- Login (429): `/auth/login_rate_limited`
- Session: `/auth/session`
- CSRF token: `/auth/csrf`
- Change password (ok): `/auth/password`
- Change password (fail): `/auth/password_failed`

## Dashboard, Health, Events
- Health (ok): `/health`
- Health (degraded): `/health_degraded`
- Events: `/events`
- Services (list): `/services`
- Single service (example): `/services/http`

## App Catalog & Install
- Catalog: `/catalog`
- Install app (post app.yaml to real `/api/v1/apps`; demo provides read-only fixtures)

## Installed Apps & Details
- List apps: `/apps`
- App details (vaultwarden example): `/apps/vaultwarden`
- App logs: `/apps/vaultwarden/logs`
- Update app (ok): `/apps/vaultwarden/update`
- Update app (conflict): `/apps/vaultwarden/update_failed`
- Revert app: `/apps/vaultwarden/revert`

## Service Discovery & Local Access
- All services: `/services`
- Single service: `/services/http` (includes `host_port`, `local_url`)

## Storage & Encryption
- Disks: `/storage/disks`
- Mounts: `/storage/mounts`
- Set default root: `/storage/default-root`
- Recovery key (status): `/storage/recovery-key`
- Recovery key (generate): `/storage/recovery-key/generate`
- Unlock volumes (ok): `/storage/unlock`
- Unlock volumes (fail): `/storage/unlock_failed`
- Encrypt in place (dry-run): `/storage/encrypt-in-place_dry-run`
- Encrypt in place (confirm): `/storage/encrypt-in-place_confirm`
- Encrypt in place (failed): `/storage/encrypt-in-place_failed`
- Initialize disk (failed example): `/storage/init_failed`

## Updates & Rollback
- OS status: `/updates/os`
- Apply OS update (ok): `/updates/os/apply`
- Apply OS update (failed): `/updates/os/apply_failed`
- Rollback OS: `/updates/os/rollback`
- App updates status: `/updates/apps`

## Remote Publish (Device‑Terminated TLS)
- Status: `/remote/status`
- Configure (ok): `/remote/configure`
- Configure (DNS error): `/remote/configure/dns_error`
- Configure (port 80 blocked): `/remote/configure/port80_blocked`
- Configure (CAA error): `/remote/configure/caa_error`
- Rotate credentials: `/remote/rotate`
- Disable remote: `/remote/disable`

## Backup & Restore
- List backups: `/backup/list`
- Export configuration: `/backup/export`
- Import configuration: `/backup/import`
- App backup (vaultwarden): `/backup/app/vaultwarden`
- App restore (vaultwarden): `/restore/app/vaultwarden`

## Install to Disk (x86 Live)
- Targets: `/install/targets`
- Plan (simulate): `/install/plan`
- Run install: `/install/run`
- Fetch latest (ok): `/install/fetch-latest`
- Fetch latest (verify failed): `/install/fetch-latest/verify_failed`

## Notes
- Error fixtures typically return an error-shaped JSON: `{ "error", "code", "message" }` (HTTP 200 in demo mode). Treat presence of `error` as failure.
- Real API should return proper 4xx/5xx; see `docs/api/openapi.yaml` for response codes and schemas.
- For app-specific nested fixtures, use `/apps/<name>/...` (e.g., logs, update, revert).
