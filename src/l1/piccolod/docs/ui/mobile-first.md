# Mobile‑First UI Foundation

This UI targets mobile devices as a first‑class environment. The design and tests prioritize small viewports and touch interactions.

## Layout & Navigation
- Header: compact bar with a menu toggle (aria‑controls/expanded). On mobile, the nav opens as an overlay sheet; on desktop, it is inline.
- Content: single column on small screens; expand to grid at `md`/`lg`.
- Avoid fixed widths; prefer fluid widths and responsive spacing.

Notes
- Mobile nav is rendered conditionally (Svelte `{#if}`) to avoid class conflicts; desktop nav is a separate element visible at `md+`.
- Menu button tap target is >= 44px and exposes `aria-label`, `aria-controls`, and `aria-expanded`.
- E2E asserts that toggling the menu shows/hides the nav and that the page has no horizontal scroll.

## Overflow & Scrolling
- Do not hide overflows globally. Fix offenders explicitly:
  - `pre` and `table` use `overflow-x: auto`.
  - Media are fluid: `img, video { max-width: 100%; height: auto; }`.
  - Long content: `word-break: break-word` for `code`, `pre`, and table cells.
- No horizontal scroll at the page level; tests assert this on key screens.

## Lists, Tables, and Cards
- Prefer lists/cards on mobile; tables should degrade to scrollable blocks.
- Move non‑critical columns into details views.

## Touch Targets & Buttons
- Touch targets ≥ 40–44px height. Use Tailwind spacing to ensure comfortable taps.
- Group actions into a horizontal bar on desktop; stack on mobile.

## Performance
- Keep initial JS small; code‑split by route. Defer heavy content until visible.
- Disable heavy animations; respect `prefers-reduced-motion`.

## Accessibility
- Semantic headings and roles for navigation and content.
- Visible focus and adequate contrast.
- Menu toggle exposes `aria-controls` and `aria-expanded`.

## Testing Strategy (Playwright)
- Projects: Desktop Chrome + Pixel 5.
- Mobile checks:
  - No horizontal scroll (`scrollWidth <= innerWidth`).
  - Menu toggle opens/closes; links are reachable.
  - Key flows render and are tappable (apps list → details → actions).
- Tests fail on any browser console error.
- Optionally add visual snapshots when layouts stabilize.

## Implementation Checklist (BFS)
- Shell: responsive header and container spacing (done).
- Dashboard: panels stack on mobile (done).
- Apps list: ensure buttons are accessible on mobile; switch to card list if needed.
- App details: services links visible; logs scrollable (done).
- Storage/Updates/Remote/Install/Backup/Events/Settings: shallow mobile pages (done); deepen iteratively.
