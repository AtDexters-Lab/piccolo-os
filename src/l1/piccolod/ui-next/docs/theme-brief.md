# Piccolo OS — Theme Brief (v1, Final)

**Purpose.** Capture the visual and interaction decisions that define the Piccolo OS look & feel. This brief is the source of truth for tokens, components, and review criteria; keep it aligned with **Foundations** and the product charter. 

---

## Vision & Mission

**Vision.** *To distill the power of digital independence into an experience that is simple, personal, and beautiful.*
**Mission.** *To empower individuals and small businesses to reclaim their digital independence with a simple, private, and dependable personal server.*

> The “single espresso with a little milk” metaphor guides tone: concentrated power, softened with warmth. Target feelings: **personal**, **private**, **self‑sufficient**, **beautiful**.

---

## Inputs & References

* **Product charter**: calm control; ≤3 deliberate taps for key tasks; readiness ≤90 seconds; AA contrast + predictable motion. 
* **Foundations**: Material‑leaning layout, grid, accessibility, testing cadence; SvelteKit + Radix component architecture; tokens via CSS variables/Tailwind. 
* **Brand artifacts**: rounded aluminum enclosure and Comfortaa wordmark inform soft radii, frosted panels, and friendly accents. 

---

## Design Philosophy

* **Material 3 core, Radix primitives.** Use M3 semantics for spacing, elevation, shape, and motion; implement with Radix + Tailwind to reach an accessible baseline quickly. 
* **Apple/HIG cues.** Subtle gradients, frosted panels, soft shadows, pill CTAs—used sparingly to support the espresso metaphor, not as decoration. 
* **Piccolo identity.** Calm, device‑class admin with welcoming tone. Strong accent + neutral body; copy is plain‑spoken and helpful. 

---

## Theme Pillars

1. **Frosted Canvas (with accessible fallbacks).** Elevated cards on a soft canvas; use blur where content behind is contextual. In **High‑Contrast** (`prefers-contrast: more`) or **Reduced Motion**, swap gradients/blur for solids—no “low‑power mode” is implemented or required. 
2. **Bold Accent, Neutral Body.** Accent carries CTAs and highlights; body stays quiet and legible. Gradients are constrained to hero/primary CTAs. 
3. **Progressive Panels.** Wizards use pill steppers and stacked cards; a progress rail appears on desktop for orientation. 
4. **Device‑grade Typography.** Comfortaa for hero/labels; Inter for UI/body. Friendly without sacrificing efficiency. 

---

## Token Architecture

**Two layers.**

* **System (core) tokens** — stable semantic roles mapped per theme:
  `--sys-surface`, `--sys-surface-variant`, `--sys-on-surface`,
  `--sys-accent`, `--sys-on-accent`,
  `--sys-success`, `--sys-on-success`, `--sys-warning`, `--sys-on-warning`, `--sys-info`, `--sys-on-info`, `--sys-critical`, `--sys-on-critical`,
  `--sys-ink`, `--sys-ink-muted`, `--sys-link`, `--sys-outline`,
  `--sys-scrim`, `--sys-overlay`, `--sys-hairline`,
  `--sys-disabled-bg`, `--sys-disabled-fg`. 
* **Component aliases** — resolve to system tokens:
  `--btn-primary-bg: var(--sys-accent)`, `--btn-primary-fg: var(--sys-on-accent)`,
  `--card-bg: var(--sys-surface)`, `--toast-error-bg: var(--sys-critical)`…

**Spacing & layout tokens.**
`--space-4, -8, -12, -16, -24, -32, -40`; grid: mobile first, tablet 8‑col, desktop 12‑col up to 1200px. 

**Motion tokens.**
`--motion-dur-fast: 120ms; --motion-dur-med: 180ms; --motion-dur-slow: 240ms;`
`--motion-ease-standard: cubic-bezier(.2,0,0,1); --motion-ease-emphasized: cubic-bezier(.16,1,.3,1);`
`--motion-distance-sm: 8px; --motion-distance-md: 16px;` 

**Shape & elevation tokens.**
Radius scale: `--radius-xs: 6px; --radius-sm: 10px; --radius-md: 14px; --radius-lg: 20px; --radius-xl: 28px; --radius-pill: 999px`.
Elevation (shadow + overlay): `--elev-0…5`, where higher levels add tinted ambient + key shadows. 

---

## Color System

**Neutrals & accent (light theme defaults).**

* `--sys-surface` (**Mist**): `#F4F6FB`
* `--sys-surface-variant` (**Porcelain**): `#FFFFFF`
* `--sys-ink`: `#141821`; `--sys-ink-muted`: `#6B7380`
* `--sys-accent` (**Iris**): `#6660FF`; gradient stop (hero only) **Lavender** `#8F82FF`
* Status: `--sys-success: #10B981`, `--sys-info: #3B82F6`, `--sys-warning: #F59E0B`, `--sys-critical: #EF4444`. 

**On‑color pairings.**

* `--sys-on-accent` defaults to white for buttons/chips; **must pass AA** for the smallest label size used (generally 14–16 px). If any accent step fails, choose the next darker accent step or increase label weight/size. (CI should check this.) 
* `--sys-on-*` (success/info/warning/critical) similarly guarantee AA for text/icons.

**Tonal steps & states.**
Define `accent-90/80/70/60/50/40` from the base hue to drive **hover**, **pressed**, and **selected** without ad‑hoc blends. In **High‑Contrast**, gradients and frosts become **solid** fills; outlines strengthen via `--sys-outline`. 

**Links.**
`--sys-link` is distinct from button accent and gets a visited variant; never rely on hue alone—underline persists. 

**Dark theme.**
Mirror tokens with deeper surfaces (`#0B0E18` / `#1A2030`) and text opacities tuned per layer so stacks don’t flatten; borders use higher opacity to maintain separation. 

---

## Typography

**Roles.**
`--font-display: Comfortaa` (hero/labels); `--font-ui: Inter` (UI/body); `--font-mono: ui-monospace, Menlo, Consolas` (keys, addresses). 

**Ramp (fluid where possible).**

* Hero: `clamp(28px, 4vw, 40px)` / line 1.2
* Section: `clamp(20px, 2.5vw, 28px)` / line 1.3
* Body: `16px` / `24px`
* Meta: `12px` / `16px`, **tracking 0.12–0.20em** (cap at 0.2em; uppercase for short labels only). 

**Numerals & code.**
Tabular‑lining numerals for tables/badges; monospace for URIs, IPs, keys.
Prefer variable fonts; set weight axes in tokens so components remain consistent. 

---

## Components (spec & states)

**Buttons** (Primary / Tonal / Secondary / Ghost / Destructive / Quiet)

* Sizes: default 40–44 px height; compact 32–36 px.
* States: default → hover (accent‑step up/down) → **pressed** (tone darken + elevation reduce) → focus (2px ring with inner offset) → disabled (reduced contrast + no shadow). 

**Inputs & Forms**

* Text fields, selects, radios, checkboxes: comfortable and compact densities.
* Validation on blur + on submit; helper text reserves 2 lines; clear affordance (×) and password visibility toggle included. 

**Stepper & Progress**

* Pill steppers with states **pending / active / done / error / blocked** (icon + color + text); mobile stacks vertically; desktop shows a progress rail. Long titles truncate with tooltip. 

**Chips & Status**

* Color + icon + text; shape and iconography ensure meaning isn’t conveyed by color alone. 

**Tables & Lists**

* Row heights (comfy/compact), zebra option, sticky headers, progress placeholders.

**Toasts & Banners**

* Error hierarchy: banner (global) > inline (local) > toast (transient). Copy style is remedy‑oriented. 

**Iconography**

* Radix/Phosphor line icons, 2px stroke, rounded caps; sizes 16/20/24/32. Comfortaa mark reserved for brand moments. 

---

## Motion

* Default transitions 150–180 ms; sheets/wizard 200 ms with emphasized easing; animate **one hierarchy level** per interaction. Honor `prefers-reduced-motion`. 
* Micro‑interactions: button press uses `translateY(1px)` + shadow reduction; invalid submit performs a short 1‑D shake (disabled under reduced motion). 

---

## Accessibility

* **Contrast:** AA minimum for body text (≥4.5:1), 3:1 for UI text/icons. CI should fail if a component’s token pairing regresses below thresholds. 
* **Focus:** 2px accent ring with inner offset; use `:focus-visible`. 
* **High‑Contrast:** `prefers-contrast: more` strengthens borders, drops gradients/frosts to solids. 
* **Hit targets:** minimum 44×44 px touch; 24 px in dense desktop with invisible padding. 
* **Never hue‑only.** Pair color with icon/text; provide clear error remedies. 

---

## Internationalization & Personalization

* **RTL** mirroring for steppers/rails and directional icons.
* **Font fallbacks** for CJK/Indic (Noto subsets) to preserve visual weight with Inter/Comfortaa. 
* **Preferences API** stores theme selections; updates propagate live through CSS variables (guardrails enforce AA). Optional dynamic accent generation with instant revert. 

---

## Testing & Review Ritual

* **Screenshot cadence** (desktop & mobile) from headless flows; reviewers write expected traits before opening images. Keep this ritual for every visual change. 
* **Storybook**: include “Theme v1” snapshots for buttons, cards, inputs, chips, steppers (light/dark/HC).
* **Contrast checks in CI**: automated WCAG tests for every token pairing used by components. 

---

## Token Tables (initial values)

> Values are the **light theme** defaults; dark/HC variants live in the theme switch. Component aliases map these into Button/Card/etc.

```txt
Colors
--sys-surface:            #F4F6FB   /* Mist */
--sys-surface-variant:    #FFFFFF   /* Porcelain */
--sys-ink:                #141821
--sys-ink-muted:          #6B7380
--sys-accent:             #6660FF   /* Iris */
--sys-accent-hero:        #8F82FF   /* Lavender, hero-only gradient stop */
--sys-success:            #10B981
--sys-info:               #3B82F6
--sys-warning:            #F59E0B
--sys-critical:           #EF4444
--sys-on-accent:          #FFFFFF   /* AA required for smallest button text */
--sys-on-success:         #FFFFFF
--sys-on-info:            #FFFFFF
--sys-on-warning:         #141821
--sys-on-critical:        #FFFFFF
--sys-link:               #3B82F6
--sys-outline:            rgba(20,24,33,.14)
--sys-scrim:              rgba(0,0,0,.40)
--sys-overlay:            rgba(20,24,33,.06)
--sys-hairline:           rgba(20,24,33,.08)
--sys-disabled-bg:        rgba(20,24,33,.06)
--sys-disabled-fg:        rgba(20,24,33,.38)

States (opacities / tonal steps)
--state-hover:            +1 accent step / +8% overlay
--state-pressed:          +2 accent steps / +14% overlay
--state-selected:         outline->accent; bg +4% tint
```

```txt
Shape & Elevation
--radius-xs: 6px; --radius-sm: 10px; --radius-md: 14px; --radius-lg: 20px; --radius-xl: 28px; --radius-pill: 999px
--elev-0: none
--elev-1: ambient 0 1px 2px rgba(0,0,0,.06)
--elev-2: ambient 0 2px 6px rgba(0,0,0,.08), key 0 1px 2px rgba(0,0,0,.06)
--elev-3: ambient 0 8px 20px rgba(0,0,0,.10), key 0 2px 6px rgba(0,0,0,.08)
--elev-4: modal shadow + scrim var(--sys-scrim)
```

```txt
Typography
--font-display: Comfortaa
--font-ui: Inter
--font-mono: ui-monospace, Menlo, Consolas
Hero: clamp(28px, 4vw, 40px) / 1.2
Section: clamp(20px, 2.5vw, 28px) / 1.3
Body: 16px / 24px
Meta: 12px / 16px, tracking 0.12–0.20em, uppercase for short labels
```

---

## Component State Matrix (abbrev.)

| Component        | Default                            | Hover          | Pressed                    | Focus                   | Disabled           | HC Mode                               |
| ---------------- | ---------------------------------- | -------------- | -------------------------- | ----------------------- | ------------------ | ------------------------------------- |
| Button – Primary | `--sys-accent` / `--sys-on-accent` | accent‑step±1  | accent‑step±2 + `--elev-0` | 2px ring (inner offset) | `--sys-disabled-*` | solid (no gradient), stronger outline |
| Button – Tonal   | accent tint on `--sys-surface`     | +overlay       | +overlay+darken            | ring                    | disabled           | solid, stronger outline               |
| Chip – Status    | status bg + icon + text            | tint           | darken                     | ring                    | reduced contrast   | solid, icon persists                  |
| Input Field      | `--sys-surface-variant`            | outline darken | bg tint                    | ring + shadow tighten   | disabled           | outline high‑contrast                 |
| Stepper          | pending/active/done/error tokens   | n/a            | n/a                        | ring on active          | disabled           | solid fills, clearer borders          |

---

## What we are **not** doing

* **Low‑power rendering mode.** Not required; Piccolo UI renders on the client device. We still honor accessibility preferences (reduced motion, high contrast) and provide solid fallbacks for blur/gradients. 

---

## Freeze Criteria (for “Theme v1”)

1. **Token tables** (light/dark/HC) committed and consumed by Button, Input, Card, Chip, Toast, Stepper. 
2. **Contrast CI** passes for all on‑color pairings and component states. 
3. **Storybook snapshots** (light/dark/HC) for the components above.
4. **Screenshot tour** updated; reviewers record expected traits before inspection. 
