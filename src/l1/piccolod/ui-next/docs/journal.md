# UI-next Engineering Journal (append-only)

## 2025-11-08 – Tailwind rollback for stable builds
- **Event:** Screenshot + browser renders were unstyled even though CSS assets existed.
- **Cause:** We had upgraded to Tailwind CSS v4 (via `@tailwindcss/postcss`). The new pipeline stripped most utility classes when bundled with Vite/SvelteKit, leaving only bare HTML.
- **Action:** Reverted to Tailwind CSS 3.4 (`tailwindcss` + classic PostCSS plugin). Rebuilt the UI and re-embedded assets; screenshots now show the frosted skin.
- **Follow-ups:** Track the Tailwind v4 migration separately once Vite/SvelteKit integration documents stabilize and we can validate the build output with visual diffs.
## 2025-11-13 – Recovery banner + CSRF hardening
- **Event:** The Staleness banner needed a clearer remediation path and logout occasionally returned 403 on first click after login.
- **Cause:** The banner offered only Acknowledge/Details, forcing users to hunt for `/setup`, and CSRF tokens weren’t primed/reset after authentication transitions.
- **Action:** Banner now links directly to `/setup?focus=recovery`, recovery step handles stale keys (regenerate or continue), and CSRF tokens are refreshed during login/logout so POSTs succeed immediately.
- **Follow-ups:** Monitor for additional flows that should deep-link into the recovery step; consider adding automated tests for redirect decoding.

