# Piccolo UI development cadence

This playbook keeps every UI pass aligned with the charter and gives reviewers consistent artifacts.

## 1. Capture fresh screenshots
- Desktop tour (automation migrating to `ui-next`): run your preferred script (e.g., `codex-ui-run-ui-review` capture pass) against the latest build, then copy the resulting PNGs into `reviews/screenshots/desktop`. The legacy `make e2e-visual` target has been removed because it depended on `web-src`.
- Mobile: repeat the capture with mobile viewport settings (or a mobile-specific reviewer pass) and stash those artifacts under `reviews/screenshots/mobile`.
- Keep both folders under `reviews/` so reviewer reruns don’t clobber your evidence.

## 2. Run automated reviews
- **Default regression loop – UI reviewer**
  - Use when validating incremental fixes (copy, spacing, bug patches).
  - Command from repo root:
    ```bash
    codex-ui-run-ui-review reviews/screenshots/desktop reviews/screenshots/mobile \
      --output reviews/ui-review-<date>.md
    ```
  - Attaches all PNG files and runs the `ui_reviewer` prompt (default) focused on usability regressions.
- **Structural or milestone critique – Design reviewer**
  - Use for layout changes, new flows, or milestone readiness checks.
  - Provide the charter (and other context as needed):
    ```bash
codex-ui-run-ui-review reviews/screenshots/desktop reviews/screenshots/mobile \
  --prompt-name design_reviewer \
  --context src/l1/piccolod/02_product/piccolo_os_ui_charter.md \
  --output reviews/design-review-<date>.md
    ```
  - Add further context with repeated `--context` flags (blueprint, acceptance criteria) when relevant.
- Escalate from a UI review to a design review if the UI critique flags systemic issues (e.g., Remote flow clarity, navigation).

## 3. Shareable artifacts
- Screenshots: keep desktop and mobile folders side-by-side so reviewers can compare breakpoints.
- Reviews: store the JSON output in `reviews/` with date-based filenames; include links/paths in PR descriptions.
- When follow-up changes ship, rerun the loop so reviewers always see current captures before sign-off.

## 4. Checklist before submitting PRs
- ✅ `make e2e` succeeds (portal smoke + console/network logs) and fresh screenshot folders exist for both breakpoints.
- ✅ Latest review JSON (UI or Design) attached or linked in the PR.
- ✅ Reviewer action items triaged—either addressed in the diff or filed as follow-ups.
- ✅ Any accessibility warnings from the review (contrast, tap targets, focus order) are resolved or tracked.

Sticking to this cadence keeps the “calm control” promise measurable and gives stakeholders a predictable review surface every sprint.

## Appendix: portal smoke test (ui-next)

Use the new harness when you need end-to-end evidence that the embedded portal loads cleanly (no console errors / missing assets):

```bash
cd src/l1/piccolod
make e2e
```

The target compiles the latest piccolod binary (including the Svelte build), starts it on a random localhost port with a scratch state directory (both via `mktemp`), runs the Playwright `portal_logs.spec.ts` suite against `http://127.0.0.1:<port>`, then tears everything down (state dir removed automatically). Artifacts:

- Server log: `src/l1/piccolod/test-results/piccolod-e2e.log`
- Playwright attachments (console & network JSON): `src/l1/piccolod/ui-next/test-results/e2e/…`

Override defaults with `PICCOLO_E2E_PORT`, `PICCOLO_E2E_STATE_DIR`, or `PICCOLO_BASE_URL` (forwarded to Playwright) if you need to bind to a fixed port or reuse an existing state volume.
