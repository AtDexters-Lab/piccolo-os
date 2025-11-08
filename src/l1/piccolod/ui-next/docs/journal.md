# UI-next Engineering Journal (append-only)

## 2025-11-08 – Tailwind rollback for stable builds
- **Event:** Screenshot + browser renders were unstyled even though CSS assets existed.
- **Cause:** We had upgraded to Tailwind CSS v4 (via `@tailwindcss/postcss`). The new pipeline stripped most utility classes when bundled with Vite/SvelteKit, leaving only bare HTML.
- **Action:** Reverted to Tailwind CSS 3.4 (`tailwindcss` + classic PostCSS plugin). Rebuilt the UI and re-embedded assets; screenshots now show the frosted skin.
- **Follow-ups:** Track the Tailwind v4 migration separately once Vite/SvelteKit integration documents stabilize and we can validate the build output with visual diffs.
