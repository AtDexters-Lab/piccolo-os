# Setup Wizard Execution Plan (v0 → v1)

Goal: ship a first-class `/setup` flow that meets the first-run acceptance criteria, exercises the real backend APIs, and hands users off to the Piccolo Home shell in ≤ 90 seconds with clear status, accessibility, and auditability.

## Source Inputs
- **Product charter & blueprint:** calm control, ≤3 deliberate taps, responsive layouts, AA contrast (`piccolo_os_ui_charter.md`, `piccolo_os_ui_blueprint.md`).
- **PRD:** first-run time-to-portal ≤ 60s, time-to-first-task ≤ 90s, encrypted volumes unlock only after admin creation.
- **Acceptance feature:** `first_run_and_unlock.feature` (portal reachable fast, setup creates admin + mounts volumes, TPM remote unlock flows).
- **API contracts:** `/auth/initialized`, `/auth/setup`, `/auth/login`, `/auth/session`, `/auth/csrf`, `/crypto/status`, `/crypto/setup`, `/crypto/unlock`, `/crypto/recovery-key`, `/system/identity` (if available).

Keep this doc in sync as flows evolve; append decision dates in the engineering journal when we materially change direction.

## User & Environment States
| State | Description | Required Data |
| --- | --- | --- |
| **Bootstrapping** | Device freshly powered; portal reachable, admin not created. | `initialized=false`, crypto `locked=true`.
| **Needs Admin** | Waiting for password entry + confirmation. | Device name, guidance copy, password policy.
| **Initializing Crypto** | `/auth/setup` succeeded, `/crypto/setup` running. | Spinner, log line, restart instructions on failure.
| **Initialized & Locked** | Admin exists (local or remote unlock required). | CTA to `/login`, TPM hint if remote.
| **Unlocked / Ready** | Flow complete; route to `/` (home) or `/install` follow-up. | Session cookie + success toast.
| **Error / Rate Limited** | Backend returned 4xx/5xx or network error. | Actionable remedy (retry, wait, reboot, check logs).

## UX Checklist
1. **Identity header:** show device name + status pill (“Locked”, “Ready”, “Remote resume required”).
2. **Stepper (≤3 steps):** `Review → Credentials → Finish`, with state overrides for error/success.
3. **Password form:** live policy feedback (length, match, strength), reveal toggle, future copy for passkeys.
4. **Crypto lock banner:** note that services stay locked until setup completes; highlight TPM/remote case.
5. **Accessibility:** keyboard order, focus rings, screen-reader labels for state transitions, AA contrast on banners.
6. **Motion:** respect reduce-motion (already handled globally) and avoid spinner loops >3s without messaging.

## API Integration Plan
| Endpoint | Purpose | Client Task |
| --- | --- | --- |
| `GET /crypto/status` | Primary signal for the wizard. | `initialized=false` → first-run path; `initialized=true && locked=true` → unlock path; `locked=false` → ready state.
| `POST /crypto/setup` | Only for first boot. | Use the chosen password to initialize crypto before unlocking.
| `POST /crypto/unlock` | Unlocks volumes and seeds the auth repo/session. | Call after setup on first boot and whenever the device reboots locked. Accept password or recovery key.
| `GET /auth/initialized` | Confirm admin existence after unlock. | Expect 423 until unlock succeeds. If still `false`, auto-run `/auth/setup` with the same password.
| `POST /auth/setup` | Rare repair case when auth repo wasn’t seeded. | Execute programmatically (no extra prompt) after unlock if needed.
| `GET /auth/session` | Device identity + context for hero copy. | Soft dependency; failures shouldn’t block the wizard.
| `GET /auth/csrf` | CSRF token for mutations. | `http.ts` attaches automatically.
| `GET /crypto/recovery-key` | Determine whether to prompt for recovery key generation. | Optional reminder post-setup.

All mutation helpers should accept `AbortSignal` and propagate detailed `ApiError` objects (`code`, `message`, `details?`).

## Recovery Key Requirements
- The setup wizard now includes a first-class Recovery step that blocks completion until the operator saves a 24-word key.
- `/crypto/recovery-key` is checked whenever the wizard reaches the ready phase. Missing or `stale=true` keys automatically reopen the Recovery step; `/setup?focus=recovery` deep links to it as well.
- Operators must either generate a fresh key (copy/download, acknowledge) or explicitly “Continue with existing key” when the backend marks it stale. Continuing acknowledges the risk via `/auth/staleness/ack`.
- Regenerating a key uses the same `/crypto/recovery-key/generate` endpoint but is gated by a confirmation modal so the previous key can’t be overwritten accidentally.
- The Staleness banner in AppShell links to `/setup?focus=recovery` so users can immediately address stale credentials.

## State Machine Sketch
```
boot → fetch /crypto/status
  ↳ initialized=false → phase:first-run (ask for password) → crypto.setup → crypto.unlock → auth.initialized? (if false → auth.setup) → ready
  ↳ initialized=true & locked=true → phase:unlock (ask password or recovery key) → crypto.unlock → auth.initialized? (if false → auth.setup) → ready
  ↳ initialized=true & locked=false → phase:ready (CTA to dashboard/login)

Any failure → phase:error → retry refresh
```

Represent this as a typed discriminated union so UI components consume a single source of truth (`setupStateStore`).

## Error & Copy Guidelines
- Prefix errors with action: “Setup failed — device already has an admin. Go to Sign in.”
- Rate limit (429): “Too many attempts. Please wait {{retry_after}} seconds.”
- Network failure: “Unable to reach Piccolo services. Check power/network or retry.”
- Crypto failure: encourage logs bundle + support doc link.

## Telemetry Hooks (phased)
1. Emit local console events for QA (`console.info('[setup]', state, metadata)`).
2. When event bus lands, dispatch `ui.setup.step.enter`, `ui.setup.completed`, `ui.setup.error` with metadata (duration, source state, error code).
3. Capture time from portal load to setup completion; expose via debug overlay for perf budgets.

## Testing Strategy
- **Unit:** add store tests for the state machine transitions and validators (password policy, API error classification).
- **Playwright:**
  - `setup-success`: stub APIs for the happy path.
  - `setup-already-initialized`: ensure redirect banner & CTA to `/login`.
  - `setup-rate-limit`: show retry timer.
  - `setup-crypto-error`: stay on credentials, show remediation.
- **Screenshots:** update `scripts/capture-ui-screenshots.mjs` flows to include the success and error states (both light/dark).

## Deliverables Checklist
- [ ] API helpers (`session`, `csrf`, `crypto.status/unlock`).
- [ ] State store + typed machine.
- [ ] Updated `/setup` route UI + copy.
- [ ] Tests (unit + Playwright) green.
- [ ] Light/dark screenshots refreshed with new states.
- [ ] Journals updated (engineering + bug-fixing if defects addressed).
- [ ] PR with Conventional Commit(s), spec links, and test/screenshot artifacts.

Revisit this checklist after the first integration slice to add/remove steps as the backend hardens.
