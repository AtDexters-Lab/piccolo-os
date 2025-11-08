# Piccolo UI Foundations

## North Star
- Anchor all UI work on `src/l1/piccolod/02_product/piccolo_os_ui_charter.md`.
- Charter targets: calm control, ≤3 deliberate taps for key flows, readiness within 90 seconds, AA contrast and predictable motion.
- Product context (PRD, acceptance features, etc.) lives under `src/l1/piccolod/02_product/`; review those specs before committing UX changes.
- API contracts live in `src/l1/piccolod/docs/api/openapi.yaml`; keep UI flows aligned with that source of truth and add/extend endpoints there before coding.

## Design Methodology
- We currently ship without a dedicated UI designer, so we lean heavily on established design systems (Material, Radix primitives) and high-quality component libraries to reach a mature baseline quickly.
- Use Material You/M3 rules for spacing, typography, elevation, and motion; customize via tokens but stay within widely recognized patterns for trust and accessibility.
- When introducing bespoke interactions, document rationale and references to the external guidelines we’re extending.

## Architecture
- **Framework:** SvelteKit + TypeScript (SSR, routing, forms).
- **Components:** Radix UI primitives + Tailwind CSS themed with Material 3 tokens.
- **State/data:** TanStack Query (Svelte Query) for fetching/caching; derived Svelte stores for status chips and preferences.
- **Build:** Vite (via SvelteKit), bundled with piccolod like existing `web-src`.

## Layout & Tokens
- Mobile-first stack; tablet 8-col, desktop max 1200px (12-col grid).
- Spacing tokens (8/12/16/24) defined as CSS variables + Tailwind config.
- Semantic colors (surface, on-surface, status ok/warn/error, accent) sourced from Material 3 palette.
- Typography ramp: 14/16/20/24/32 with consistent letter spacing.

## Interaction Patterns
- Bottom tab bar on mobile, side rail on desktop (identical structure).
- System Status Dock with Remote/Storage/Updates/Apps chips; one CTA per chip.
- Sheet-based flows (bottom/side) with max 3 steps, inline validation, focus trap.
- Banners for global issues, toasts for transient feedback.

## Remote & Storage Flows
- Remote: single entry CTA → sheet with Connect helper → Assign domain → Verify & enable.
- Storage: Unlock volumes → Attach/manage disks → Recovery key.
- Advanced diagnostics gated until base setup completes.

## Theming & Personalization
- Preferences stored via `/api/ui/preferences`; client hydrates on load and updates via API.
- CSS variables drive tokens; user changes update vars in real time (optimistic UI optional).
- Guardrails enforce AA contrast before applying custom palettes.
- Optional dynamic color generation (Material You) from user accent.

## Copy Standards
- Tone: calm, friendly, device-class. Use verb + object CTAs; include remedies in error text.

## Accessibility & Motion
- AA contrast minimum; high-contrast theme available.
- Respect `prefers-reduced-motion`; safe-area padding for notches; sheet modals trap focus.

## Data Loading & Offline
- Query stale times: status chips 5s, storage 15s, updates 60s.
- Skeletons for hero/cards; offline banner on failure; API client generated from OpenAPI spec.

## Testing & Tooling
- Playwright suites: visual tour (desktop/mobile), remote flow, storage unlock.
- `npm run review`: runs e2e, syncs screenshots (`tools/sync-review-screenshots.mjs`), triggers reviewer.
- Storybook/SvelteKit preview documents components with mobile viewports.
- Design review outputs stored under `src/l1/reviews/<date>/`.
- **Screenshot cadence:** run `npm run screenshots` (which reuses `scripts/run-e2e-with-server.sh` to boot piccolod and then executes `scripts/capture-ui-screenshots.mjs`) to traverse core flows in a headless browser and save PNGs under `src/l1/piccolod/ui-next/screenshots/<timestamp>/`. Every new screen/flow must add a step to that script so reviewers always get an updated visual record.
- **Screenshot review ritual:** after capturing, write down (a) the specific states/visual traits you expect to see and (b) anything that must *not* appear (unstyled HTML, incorrect data, etc.). Only then open the images and perform a visual inspection to confirm the expectations list, and document next steps if the captures diverge.
- **Engineering journal:** any noteworthy build/runtime shifts (framework upgrades, tooling rollbacks, infra decisions) must be logged append-only in `src/l1/piccolod/ui-next/docs/journal.md` with date, cause, action, and follow-ups so future contributors understand why a change happened.

## Directory Layout
```
src/l1/piccolod/ui-next/
├── docs/
│   └── foundation.md
├── src/
│   ├── lib/
│   ├── components/
│   └── routes/
├── package.json
└── ...
```

Update this document as tokens, flows, or tooling evolve so every contributor stays aligned with the charter and "minimum code" philosophy.
