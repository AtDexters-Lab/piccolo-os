# Piccolo UI Foundations

## North Star
- Anchor all UI work on `src/l1/piccolod/02_product/piccolo_os_ui_charter.md`.
- Charter targets: calm control, ≤3 deliberate taps for key flows, readiness within 90 seconds, AA contrast and predictable motion.

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
