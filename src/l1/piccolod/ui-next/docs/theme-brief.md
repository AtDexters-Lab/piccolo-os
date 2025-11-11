# Piccolo OS — Theme Brief (v1, Final · Cobalt Neutral)

**Purpose.** Capture the visual and interaction decisions that define the Piccolo OS look & feel. This brief is the source of truth for tokens, components, and review criteria; keep it aligned with **Foundations** and the product charter.

---

## Vision & Mission

**Vision.** *To distill the power of digital independence into an experience that is simple, personal, and beautiful.*
**Mission.** *To empower individuals and small businesses to reclaim their digital independence with a simple, private, and dependable personal server.*

> The “single espresso with a little milk” metaphor guides tone: concentrated power, softened with warmth. Target feelings: **personal**, **private**, **self-sufficient**, **beautiful**.

---

## Inputs & References

* **Product charter**: calm control; ≤3 deliberate taps for key tasks; readiness ≤90 seconds; AA contrast + predictable motion.
* **Foundations**: Material-leaning layout, grid, accessibility, testing cadence; SvelteKit + Radix component architecture; tokens via CSS variables/Tailwind.
* **Brand artifacts**: rounded aluminum enclosure and Comfortaa wordmark inform soft radii, frosted panels, and friendly accents.

---

## Design Philosophy

* **Material 3 core, Radix primitives.** Use M3 semantics for spacing, elevation, shape, and motion; implement with Radix + Tailwind to reach an accessible baseline quickly.
* **Apple/HIG cues.** Subtle gradients, frosted panels, soft shadows, pill CTAs—used sparingly to support the espresso metaphor, not as decoration.
* **Piccolo identity.** Calm, device-class admin with welcoming tone. Strong accent + neutral body; copy is plain-spoken and helpful.

---

## Theme Pillars

1. **Frosted Canvas (accessible fallbacks).** Elevated cards on a soft canvas; use blur where background context helps. In **High-Contrast** (`prefers-contrast: more`) or **Reduced Motion**, swap gradients/blur for solids—no “low-power mode” is implemented or required.
2. **Bold Accent, Neutral Body.** Accent (Cobalt) carries CTAs and highlights; the body stays neutral for legibility. Gradients are constrained to hero/primary CTAs.
3. **Progressive Panels.** Wizards use pill steppers and stacked cards; a progress rail appears on desktop.
4. **Device-grade Typography.** Comfortaa for hero/labels; Inter for UI/body. Friendly without sacrificing efficiency.

---

## Token Architecture

**System (core) tokens** — stable semantic roles mapped per theme
`--sys-surface`, `--sys-surface-variant`, `--sys-on-surface`,
`--sys-accent`, `--sys-on-accent`,
`--sys-success`, `--sys-on-success`, `--sys-warning`, `--sys-on-warning`, `--sys-info`, `--sys-on-info`, `--sys-critical`, `--sys-on-critical`,
`--sys-ink`, `--sys-ink-muted`, `--sys-link`, `--sys-outline`,
`--sys-scrim`, `--sys-overlay`, `--sys-hairline`,
`--sys-disabled-bg`, `--sys-disabled-fg`.

**Component aliases** — resolve to system tokens so components share a single source of truth
`--btn-primary-bg`, `--btn-primary-fg`, `--btn-primary-shadow`, `--btn-primary-bg-hover`, `--btn-primary-bg-pressed`,
`--btn-secondary-bg`, `--btn-secondary-fg`, `--btn-secondary-outline`, `--btn-secondary-hover-bg`,
`--card-bg`, `--card-border`, `--card-overlay`,
`--stepper-active-bg`, `--stepper-pending-bg`, `--stepper-error-bg`, `--stepper-focus-ring`,
`--chip-info-bg`, `--chip-warning-bg`, `--chip-critical-bg`,
`--toast-info-bg`, `--toast-error-bg`, `--toast-border`,
`--focus-ring`, `--outline-strong`, `--elev-1`, `--elev-2`, `--elev-3`.

**Spacing & layout tokens**
`--space-4, -8, -12, -16, -24, -32, -40`; grid: mobile first, tablet 8-col, desktop 12-col up to 1200px.

**Motion tokens**
`--motion-dur-fast: 120ms; --motion-dur-med: 180ms; --motion-dur-slow: 240ms;`
`--motion-ease-standard: cubic-bezier(.2,0,0,1); --motion-ease-emphasized: cubic-bezier(.16,1,.3,1);`
`--motion-distance-sm: 8px; --motion-distance-md: 16px;`

**Shape & elevation tokens**
Radius scale: `--radius-xs: 6px; --radius-sm: 10px; --radius-md: 14px; --radius-lg: 20px; --radius-xl: 28px; --radius-pill: 999px`.
Elevation tiers (shadow + overlay): `--elev-0…4`, standardized below.

---

## Color System

### Neutrals & accent (light theme defaults)

* `--sys-surface` (**Mist**): `#F4F6FB`
* `--sys-surface-variant` (**Porcelain**): `#FFFFFF`
* `--sys-ink`: `#141821`
* `--sys-ink-muted`: `#6B7380` *(raise to ~#5E6673 if any body text falls under AA)*
* **Accent family (Cobalt):**
  `--accent-700: #254BDD` (pressed) · `--accent-600: #2F5AF3` (base) · `--accent-500: #3D66FF` (hover top) · `--accent-400: #5F80FF` (hover bottom) · `--accent-300: #7EA2FF` (tints)
* `--sys-accent: var(--accent-600)`
* `--sys-on-accent: #FFFFFF`
* `--sys-link: var(--accent-600)` · **Visited:** `#1D4ED8` (keeps underline)
* Status: `--sys-success: #10B981`, `--sys-info: #3B82F6`, `--sys-warning: #F59E0B`, `--sys-critical: #EF4444`.

### Hero canvas (neutral gray-blue)

`--gradient-hero: radial-gradient(1150px 560px at 18% -5%, #F6F8FC 0%, #E9EEF6 42%, #E4EAF3 100%);`

### Dark theme mirrors

* Surfaces: `#0B0E18` / `#1A2030`
* Ink: `#E7ECF6` / Muted `#B4BDCB`
* Accent: lighten for dark (`--accent-500: #7EA2FF`, `--accent-600: #5B86FF`, `--accent-700: #3F6BFF`)
* Link/focus use the same family.

### On-color pairings

* `--sys-on-accent` is white and **must pass AA** for the smallest label size used (14–16 px). If any step fails, darken the accent or raise label weight/size.
* `--sys-on-*` (success/info/warning/critical) similarly guarantee AA.

---

## Typography

**Roles**
`--font-display: Comfortaa` (hero/labels) · `--font-ui: Inter` (UI/body) · `--font-mono: ui-monospace, Menlo, Consolas` (keys, addresses).

**Ramp (fluid where possible)**

* Hero: `clamp(28px, 4vw, 40px)` / 1.2
* Section: `clamp(20px, 2.5vw, 28px)` / 1.3
* Body: `16px / 24px`
* Meta: `12px / 16px`, **tracking 0.16–0.20em** (cap at 0.20em; uppercase for short labels only)

**Numerals & code**

* Use **tabular-lining numerals** for steps, percentages, and tables.
* Monospace for URIs/IPs/keys.

---

## Components (spec & states)

### Buttons (Primary / Tonal / Secondary / Ghost / Destructive / Quiet)

* Sizes: default 40–44 px height; compact 32–36 px.
* **Primary (cobalt)**

  * Default: `linear-gradient(180deg, #3D66FF, #2F5AF3)` + soft shadow
  * Hover*: `linear-gradient(180deg, #5F80FF, #3D66FF)`
  * Pressed: solid `#254BDD` with `--elev-0` (drop shadow) and optional `translateY(1px)`
  * Focus: **2px** ring `#2F5AF333` + 1px inner hairline
  * Disabled: `--sys-disabled-bg/fg`
* **Secondary/Ghost**

  * Outline: `#C9D3E3`
  * Hover: subtle tint `rgba(63,107,255,.06)`
  * Ensure they never resemble disabled.
* *Apply hover styles only on hover-capable devices: `@media (hover:hover) and (pointer:fine)`.*

### Inputs & Forms

* Comfortable & compact densities; on-blur + on-submit validation.
* Clear affordance (×) and password visibility toggle.
* Focus ring matches buttons.

### Stepper & Progress

* States: **pending / active / done / error / blocked** (icon + color + text).
* Inactive labels meet AA; long labels truncate with tooltip (desktop) and **wrap** (mobile, 2 lines).
* Mobile vertical stack; desktop rail for orientation.

### Chips & Status

* Color + icon + text; never hue-only.

### Tables & Lists

* Row heights (comfy/compact), zebra option, sticky headers, progress placeholders.

### Toasts & Banners

* Error hierarchy: **banner** (global) > **inline** (local) > **toast** (transient).
* Info toasts use cobalt icon/border, not purple.

### Iconography

* Radix/Phosphor line icons (2px stroke, rounded caps) at 16/20/24/32.
* Comfortaa mark reserved for brand hero moments.

---

## Motion

* Default transitions **150–180 ms**; sheets/wizard **200 ms** with emphasized easing.
* Animate **one hierarchy level** per interaction.
* Micro-interactions: button press reduces elevation; invalid submit uses short 1-D shake.
* Honor `prefers-reduced-motion`.

---

## Accessibility

* **Contrast:** AA for body text (≥4.5:1), 3:1 for UI text/icons. CI fails if a component/token pairing regresses.
* **Focus:** consistent 2px ring with inner offset using the cobalt family; `:focus-visible` only.
* **High-Contrast:** `prefers-contrast: more` strengthens borders and swaps gradients/frosts for solids.
* **Hit targets:** min 44×44 px touch; 24 px in dense desktop with invisible padding.
* **Never hue-only:** pair color with icon/text and concrete remedies.

---

## Elevation (standardized)

```
--elev-0: none
--elev-1: 0 1px 2px rgba(0,0,0,.06)
--elev-2: 0 2px 8px rgba(14,19,34,.08), 0 1px 2px rgba(14,19,34,.06)
--elev-3: 0 8px 20px rgba(14,19,34,.12), 0 2px 6px rgba(14,19,34,.08)
--elev-4: modal shadow + scrim var(--sys-scrim)
```

---

## Token Tables (initial values)

```txt
Colors
--sys-surface:            #F4F6FB   /* Mist */
--sys-surface-variant:    #FFFFFF   /* Porcelain */
--sys-ink:                #141821
--sys-ink-muted:          #6B7380
--accent-700:             #254BDD   /* pressed */
--accent-600:             #2F5AF3   /* base */
--accent-500:             #3D66FF   /* hover (top) */
--accent-400:             #5F80FF   /* hover (bottom) */
--accent-300:             #7EA2FF   /* tints */
--sys-accent:             var(--accent-600)
--sys-on-accent:          #FFFFFF
--sys-success:            #10B981
--sys-info:               #3B82F6
--sys-warning:            #F59E0B
--sys-critical:           #EF4444
--sys-on-success:         #FFFFFF
--sys-on-info:            #FFFFFF
--sys-on-warning:         #141821
--sys-on-critical:        #FFFFFF
--sys-link:               var(--accent-600)
--sys-outline:            rgba(20,24,33,.14)
--sys-scrim:              rgba(0,0,0,.40)
--sys-overlay:            rgba(20,24,33,.06)
--sys-hairline:           rgba(20,24,33,.08)
--sys-disabled-bg:        rgba(20,24,33,.06)
--sys-disabled-fg:        rgba(20,24,33,.38)
--gradient-hero:          radial-gradient(1150px 560px at 18% -5%, #F6F8FC 0%, #E9EEF6 42%, #E4EAF3 100%)

States
--state-hover:            +1 accent step / +8% overlay
--state-pressed:          +2 accent steps / +14% overlay
--state-selected:         outline->accent; bg +4% tint
```

```txt
Shape & Elevation
--radius-xs: 6px; --radius-sm: 10px; --radius-md: 14px; --radius-lg: 20px; --radius-xl: 28px; --radius-pill: 999px
--elev-0: none
--elev-1: 0 1px 2px rgba(0,0,0,.06)
--elev-2: 0 2px 8px rgba(14,19,34,.08), 0 1px 2px rgba(14,19,34,.06)
--elev-3: 0 8px 20px rgba(14,19,34,.12), 0 2px 6px rgba(14,19,34,.08)
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
Meta: 12px / 16px, tracking 0.16–0.20em, uppercase for short labels
```

---

## Component State Matrix (abbrev.)

| Component        | Default                                          | Hover                      | Pressed                     | Focus                                     | Disabled           | HC Mode                               |
| ---------------- | ------------------------------------------------ | -------------------------- | --------------------------- | ----------------------------------------- | ------------------ | ------------------------------------- |
| Button – Primary | gradient `#3D66FF→#2F5AF3` / `#FFF` / `--elev-2` | gradient `#5F80FF→#3D66FF` | solid `#254BDD`, `--elev-0` | 2px ring `#2F5AF333` + 1px inner hairline | `--sys-disabled-*` | solid (no gradient), stronger outline |
| Button – Tonal   | accent tint on `--sys-surface-variant`           | +overlay                   | +overlay+darken             | ring                                      | disabled           | solid, stronger outline               |
| Button – Ghost   | outline `#C9D3E3`, ink text                      | bg `rgba(63,107,255,.06)`  | bg `rgba(63,107,255,.12)`   | ring                                      | disabled           | stronger outline                      |
| Chip – Status    | status bg + icon + text                          | tint                       | darken                      | ring                                      | reduced contrast   | solid, icon persists                  |
| Input Field      | `--sys-surface-variant`                          | outline darken             | bg tint                     | ring + shadow tighten                     | disabled           | outline high-contrast                 |
| Stepper          | pending/active/done/error/blocked tokens         | n/a                        | n/a                         | ring on active                            | disabled           | solid fills, clearer borders          |

---

## Accessibility & Testing

* **Contrast:** AA minimum for body text (≥4.5:1), 3:1 for UI text/icons. CI should fail if a component’s token pairing regresses below thresholds.
* **Focus:** 2px accent ring with inner offset; use `:focus-visible`.
* **High-Contrast:** `prefers-contrast: more` strengthens borders, drops gradients/frosts to solids.
* **Hit targets:** minimum 44×44 px touch; 24 px in dense desktop with invisible padding.
* **Never hue-only.** Pair color with icon/text; provide clear error remedies.
* **Review ritual:** Storybook snapshots (light/dark/HC) and screenshot tour updated per release.

---

## What we are **not** doing

* **Low-power rendering mode.** Not required; Piccolo UI renders on the client device. Preferences for reduced motion/high contrast are respected, and solid fallbacks exist.

---

## Freeze Criteria (for “Theme v1”)

1. **Token tables** (light/dark/HC) committed and consumed by Button, Input, Card, Chip, Toast, Stepper.
2. **Contrast CI** passes for all on-color pairings and component states.
3. **Storybook snapshots** (light/dark/HC) for the components above.
4. **Screenshot tour** updated; reviewers record expected traits before inspection.
