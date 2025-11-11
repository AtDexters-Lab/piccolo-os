# UI-next Bug-Fixing Journal (append-only)

> Record each defect we discover during day-to-day development with symptom, root cause, fix, and any guardrails added so future contributors can avoid repeating it.

## 2025-11-10 — Theme + layout regressions
- **Stepper done state lost its green outline**
  - *Symptom:* Completed steps showed a neutral border and white bubble, giving no success cue.
  - *Root cause:* Tokens like `rgba(var(--sys-success), 0.35)` were invalid because `--sys-success` is a space-separated RGB triplet. Browsers dropped the declaration.
  - *Fix:* Switched to slash syntax (`rgb(var(--sys-success) / 0.35)`) and added tests via `npm run check`. Added guidance to theme brief about token formats.
- **Setup wizard failed to compile**
  - *Symptom:* Svelte threw "`steps` is not defined" after adding reactive state to the Stepper.
  - *Root cause:* Replaced the static array with a reactive mapping but forgot to declare `let steps` before the `$:` block.
  - *Fix:* Declared `let steps: StepDefinition[] = baseSteps;` prior to the reactive assignment.
- **Dark-mode steppers/toasts lost fills**
  - *Symptom:* In dark mode, stepper bubbles became transparent for done/error/blocked states.
  - *Root cause:* Same invalid `rgba(var(--token), alpha)` usage for dark tokens.
  - *Fix:* Converted those aliases to `rgb(var(--token) / alpha)`. Added note to foundation doc.
- **App header badge disappeared**
  - *Symptom:* The "P" glyph circle rendered white-on-white.
  - *Root cause:* Tailwind’s `bg-accent` expected an RGB triplet, but `--sys-accent` is hex (#2F5AF3), so `rgb(#hex / alpha)` collapsed to white.
  - *Fix:* Introduced `--sys-accent-rgb`/`--sys-accent-hero-rgb` tokens and pointed Tailwind’s `accent` colors at them.
- **Install sidebar overlapped stepper**
  - *Symptom:* On mid-width desktops, the sticky progress card floated above the “Finish” pill.
  - *Root cause:* Two-column grid kicked in before there was enough horizontal space, and sticky positioning kept the sidebar on top.
  - *Fix:* Responsive layout now keeps the Stepper full-width, then splits into main+sidebar rows beneath it.
- **Stepper pills had uneven heights**
  - *Symptom:* Steps with two-line descriptions rendered taller, breaking the rhythm.
  - *Root cause:* Heights were driven solely by content.
  - *Fix:* Gave the default Stepper variant a fixed min-height (92px) and consistent padding. Future variants can revisit if needed.

## 2025-11-11 — Dark theme gaps + screenshot parity
- **Dark hero/cards failed AA contrast**
  - *Symptom:* In dark screenshots the hero card and “Need to capture logs?” panel were barely legible.
  - *Root cause:* We reused the light-theme `--sys-ink`/`--sys-ink-muted` tokens and left inputs/cards with light backgrounds, so contrast dropped below 4.5:1.
  - *Fix:* Brightened dark-mode ink tokens, lightened stepper pending backgrounds, and added dark-specific input backgrounds/placeholder colors to keep all card copy AA-compliant.
- **Chrome screenshots only captured light mode**
  - *Symptom:* Reviewers only saw light-theme PNGs; dark-theme regressions went unnoticed until manual testing.
  - *Root cause:* Screenshot harness launched a single browser context and relied on manual theme toggles.
  - *Fix:* `scripts/capture-ui-screenshots.mjs` now records both light and dark contexts (with `dark-XX` filenames) so every run documents parity.
- **Favicon unreadable on dark tabs**
  - *Symptom:* The black Piccolo “P” favicon disappeared against Chromium’s dark tab strip.
  - *Root cause:* We only served `/piccolo-p.svg` regardless of color scheme.
  - *Fix:* Added theme-specific favicon links (`piccolo-p.svg` for light, `piccolo-p-white.svg` for dark) so branding stays visible.
