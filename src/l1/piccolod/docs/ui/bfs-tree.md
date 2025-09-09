# Piccolo Web UI — BFS Tree (Working Backwards)

Purpose: Keep the UI build focused and traceable by implementing breadth‑first slices that map directly to PRD goals, acceptance features, and the OpenAPI contract. Each slice adds visible user value with demo fixtures first, then production.

Legend
- Status: [Done] [Next] [Pending]
- Demo-first: implement against `/api/v1/demo/*` then flip to `/api/v1`.
- Mobile-first: every slice includes a 360×800 viewport pass in e2e.

## Level 0 — Foundation [Done]
- Shell: hash router, header/menu (aria), toasts, session bootstrap, error normalization, CSRF injection on mutations.
- Infra: Vite+Svelte+Tailwind; aliasing; build to `web/`; go:embed served by piccolod.
- E2E harness: Playwright (Desktop + Pixel 5), console‑error guard, no horizontal scroll check.

## Level 1 — Read States [Done]
- Dashboard panels: Health, Services, Storage, Updates, Remote (independent loading + localized errors).
- Apps: list, details (listeners/URLs), logs (recent JSON view).
- Events list; Backup list; Install: targets + plan simulate; Settings placeholder.

## Level 2 — Minimal Actions [Done]
- Apps: start/stop; update/revert; uninstall (confirm).
- Updates: OS apply/rollback; app update/revert.
- Remote: status; configure demo error sims (DNS/80/CAA); disable.
- Storage: Use as‑is; Initialize (confirm); Set default root.
- Backup: export.
- E2E: nav, deep‑link, app actions, OS apply, remote error sim, storage action, mobile checks.

## Level 3 — Security + Storage Flows [Next]
- Auth: Setup/Login; 401/429 handling; session timeout prompt; CSRF bootstrap; first‑run gate.
- Storage: unlock volumes; recovery key status/generate; encrypt‑in‑place dry‑run/confirm.
- Apps: uninstall with data purge option; logs bundle download.
- Remote: successful configure path; rotate credentials; renewal warnings.
- E2E: success + key error paths per feature; enforce mobile constraints.

## Level 4 — Install + Backup Completeness [Pending]
- Install: confirm by typing disk id; fetch‑latest OK/verify_failed; post‑install banner.
- Backup: import config; per‑app backup/restore.
- E2E: simulate + confirm, error fixtures.

## Level 5 — SSO + Polishing [Pending]
- SSO: ticket handoff stubs; portal cookie isolation; per‑app cookie; “Open locally” verified.
- A11y: focus/contrast; touch target sizes; keyboard nav.
- Performance: route code‑split; size budgets; optional visual snapshots for stable layouts.

## Acceptance Mapping (Source: org‑context/02_product/acceptance_features)
- First‑run/auth → L3
- Dashboard/navigation → L1–L2
- Deploy curated services → L2; polish in L3
- Service discovery/local access → L1–L2
- Storage & encryption → L2 basics; L3 unlock/recovery/encrypt‑in‑place
- Updates & rollback → L2
- Remote publish → L2 error sims; L3 success/rotate/renewal warnings
- Observability & errors → L1 events; L3 logs bundle
- Install to disk → L1 simulate; L4 wizard
- Backup & restore → L2 export; L4 import/per‑app backups
- Responsive UI → e2e at each level

## Definition of Demo‑Complete (v1)
- Every route has load/empty/error/success states wired to demo fixtures.
- Each acceptance feature has at least one success path and one representative error path.
- Mobile (Pixel 5) and Desktop e2e pass; no browser console errors; no page‑level horizontal scroll on key routes.

Related docs
- PRD: `org-context/02_product/piccolo_os_prd.md`
- Acceptance features: `org-context/02_product/acceptance_features/*.feature`
- OpenAPI: `docs/api/openapi.yaml`
- Screen inventory: `docs/ui/screen-inventory.md`
- Traceability: `docs/ui/traceability.md`
- Demo fixtures: `docs/ui/demo-fixture-index.md`
- Dev quickstart: `docs/ui/dev.md`

