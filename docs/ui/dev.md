# Piccolo UI development cadence

This playbook keeps every UI pass aligned with the charter and gives reviewers consistent artifacts.

## 1. Capture fresh screenshots
- Build and capture desktop tour from `src/l1/piccolod`:
  ```bash
  make e2e-visual
  ```
  This runs the non-demo build, launches Playwright, and stores desktop captures under `web-src/test-results/visual-Visual-tour-demo-capture-screenshots-of-key-pages-chromium/`.
- Capture the mobile set separately:
  ```bash
  cd web-src
  npx playwright test tests/mobile.spec.ts --project=mobile-chromium
  cd ..
  ```
  Mobile screenshots land in `web-src/test-results/mobile-Mobile-layout-visual-tour-mobile-screenshots--mobile-chromium/`.
- Copy both folders into a stable location (for example `reviews/screenshots/desktop` and `reviews/screenshots/mobile`) before running reviews so re-runs do not wipe them.

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
- ✅ `make e2e-visual` (desktop) and `npx playwright … mobile` succeed without failures.
- ✅ Latest review JSON (UI or Design) attached or linked in the PR.
- ✅ Reviewer action items triaged—either addressed in the diff or filed as follow-ups.
- ✅ Any accessibility warnings from the review (contrast, tap targets, focus order) are resolved or tracked.

Sticking to this cadence keeps the “calm control” promise measurable and gives stakeholders a predictable review surface every sprint.
