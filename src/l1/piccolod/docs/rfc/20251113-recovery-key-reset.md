# RFC: Recovery-Key–Driven Password Reset & Credential Staleness Flags

_Last updated: 2025-11-13_

## Summary
Recovery keys currently unlock the encrypted control store but do not help an operator regain access to the admin portal if the password is lost. This RFC proposes replacing the “unlock with recovery key” flow with a single "reset password with recovery key" endpoint, introducing credential staleness flags that the UI can surface, and adding an acknowledge API so operators can opt to accept the risk without rotating credentials immediately.

## Background
- Crypto manager already supports generating and consuming recovery keys (`internal/crypt/manager.go`).
- `/api/v1/crypto/unlock` accepts either the admin password or the recovery key and, on success, auto-logins via `SessionStore`.
- Admin password hashes live in the control store; `authManager.ChangePassword` currently requires the old password.
- There is no supported way to set a new password when only the recovery key is available, and the UI has no visibility into whether a recovery key or password is “stale”.

## Problems
1. **Lost password scenario**: Recovery key unlocks storage but leaves the operator unable to log in because auth still requires the old password.
2. **Security posture**: Reusing the same recovery key indefinitely goes unnoticed; operators are not warned that a recovery-assisted unlock occurred.
3. **UX gaps**: The UI cannot distinguish “device unlocked via recovery” from normal unlocks; there is no warning or way to acknowledge the residual risk.

## Goals
- Provide a single, atomic API that validates the recovery key, unlocks if necessary, and sets a new admin password.
- Track and expose credential staleness so the UI can show persistent warnings when the recovery key or password should be rotated.
- Allow operators to acknowledge the warning (recorded in the control store) without forcing immediate rotation.
- Keep recovery key reuse possible (operator choice) while clearly signaling the associated risk.

## Non-Goals
- Forcing automatic regeneration of recovery keys after use.
- Supporting multiple admin accounts or external identity providers (future work can build on this RFC).

## Proposed Changes
### 1. New API: `POST /api/v1/crypto/reset-password`
Request body:
```json
{
  "recovery_key": "word1 word2 ... word24",
  "new_password": "<admin password>"
}
```
Behavior:
1. Validate recovery key (fails with 401 if invalid or not set).
2. If the control store is currently locked, unlock volumes for the duration of this operation (reuse existing persistence attach logic and relock before returning).
3. Update `auth` repo with the new Argon2 hash without requiring the previous password (special recovery branch inside `authManager.ChangePassword`).
4. Rewrap `crypto/keyset.json` under the new password (reuse `Setup` primitives).
5. Set `password_stale=true` (noting this represents a duress scenario) and `recovery_stale=true` fields in the control-store metadata so the UI can warn operators.
6. Emit lock-state events and, if the device started locked, relock volumes using the existing `cryptoManager.Lock()` + `persistence.record_lock_state` path.
7. Return 200 with `{ "message": "ok" }`.
8. Apply the same brute-force protections as `/auth/login` (rate limiting) to this endpoint.
9. Publish an audit event on the shared bus (new `TopicAudit` or reuse `TopicDeviceEvent`) summarizing the reset (IP, store state, flags set).

### 2. Expose staleness flags
- Store booleans in the control store (e.g., `auth_state.password_stale`, `auth_state.recovery_stale`) along with timestamps for audit.
- Surface them via:
  - `GET /api/v1/auth/session` → add `"password_stale": bool` and `"recovery_stale": bool`.
  - `GET /api/v1/crypto/recovery-key` → include `"stale": bool` so the recovery UI shows the warning even before login.

### 3. Acknowledge endpoint
`POST /api/v1/auth/staleness/ack`
```json
{ "password": true, "recovery": true }
```
Clears the requested flags when the caller has a valid session. The UI will call this when the operator accepts the warning without rotating credentials. Emit an audit event for each acknowledgement.

### 4. Remove recovery key branch from `/api/v1/crypto/unlock`
After this change, `/crypto/unlock` accepts only the password. Operators must use `/crypto/reset-password` if they only have the recovery key.

### 5. Session & audit semantics
- Password-based unlock can keep its current auto-login behavior.
- Recovery reset does not auto-login; after a successful reset the operator logs in with the new password.
- Audit events (via the event bus) capture both resets and acknowledgements for downstream loggers.

## Data Model Updates
- Extend the control-store auth state to persist `password_stale`, `recovery_stale`, timestamps, and ack metadata.

## Rollout Plan
1. Implement backend changes (no feature flag).
2. Update UI to:
   - Use `/crypto/reset-password` for the recovery flow.
   - Display persistent warnings based on the staleness flags from `/auth/session`/`/crypto/recovery-key`.
   - Offer a “dismiss/acknowledge” action wired to `/auth/staleness/ack`.
3. Remove recovery-key unlock option from the UI once backend is deployed.
4. Document the new flow in user-facing docs.

## Open Questions
- None (rate limiting mirrors login; audit logging handled via event bus).

## Implementation Notes & Status
- ✅ Backend landed in November 2025 (piccolod `dev` branch). Key elements:
  - `POST /api/v1/crypto/reset-password`, rate-limited like `/auth/login`, unlocks only for the operation, rotates auth + SDEK, raises audit events.
  - Control-store schema now persists `password_stale`, `recovery_stale`, and ack timestamps. `AuthRepo.Staleness`/`UpdateStaleness` expose helpers for server/UI.
  - `GET /auth/session` and `GET /crypto/recovery-key` surface staleness booleans; `POST /auth/staleness/ack` clears them with its own audit log entry.
  - `/api/v1/crypto/recovery-key/generate` now rotates the mnemonic when one already exists (single active key), and `/api/v1/crypto/unlock` accepts only the password, keeping the UI flow aligned with this RFC.
- Test coverage: `go test ./...` (including `gin_auth_handlers_test.go`, persistence/auth/crypt suites) plus manual recovery-reset validation on dev nodes.
- Track work via issue `TBD` once created; UI follow-ups remain pending.
