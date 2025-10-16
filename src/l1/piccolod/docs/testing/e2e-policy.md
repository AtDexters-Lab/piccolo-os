# E2E Testing Policy

This document summarizes ground rules for end‑to‑end testing of piccolod and the portal UI.

## Principles
- Prod‑parity first: run the real `./piccolod` binary. Stub only unavoidable externals behind explicit env flags.
- Determinism and isolation: each run starts with a fresh `PICCOLO_STATE_DIR=.e2e-state`. Avoid reliance on wall‑clock sleeps; poll bounded endpoints.
- Setup via API, verify via UI: use API only for preconditions not under test (e.g., admin setup, unlock), then drive and assert via the UI flow being validated.
- Flake‑intolerant: fail on browser console errors and missing CSRF/session. Retries exist only at defined boundaries (e.g., issuance polling) with clear logging.
- Security hygiene: never commit real domains/secrets; mask secrets in logs; default lane has no outbound internet unless the scenario requires it.
- Artifacts and triage: keep Playwright traces/videos/screenshots on failure and generate a concise issues summary.
- Traceability: each spec maps to acceptance features in `src/l1/piccolod/02_product/acceptance_features/*` and uses tags for filtering.
- Accessibility/mobile: keep a small a11y and mobile set green at all times.
- Stateful/upgrade: support seeding an older state and upgrading to validate continuity.

## Execution Lanes
- Default (offline, stubbed remote):
  - Env: `PICCOLO_DISABLE_MDNS=1 PICCOLO_NEXUS_USE_STUB=1 PICCOLO_REMOTE_FAKE_ACME=1`
  - Purpose: fast, deterministic validation of core portal, auth, crypto, storage, and service lifecycle.

- Full‑remote (opt‑in, required for release):
  - Toggle: `E2E_REMOTE_STACK=1`
  - Stacks a local Nexus proxy and a Pebble ACME CA inside CI. Uses real HTTP‑01 over WSS into piccolod.
  - Pipeline must hard‑fail if this lane is unavailable.

## Commands
- `make e2e-deps` — install Playwright browser binaries.
- `make e2e` — build and run the suite with the unified Playwright config.
- `make e2e-one ARGS="..."` — run a subset, e.g., a single spec or project.

