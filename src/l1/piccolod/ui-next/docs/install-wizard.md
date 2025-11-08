# Install-to-Disk Wizard (UI-next)

Status: planning · Source inputs: `src/l1/piccolod/02_product/piccolo_os_prd.md#distribution-&-install`, `src/l1/piccolod/02_product/acceptance_features/install_to_disk_x86.feature`, `src/l1/piccolod/docs/api/openapi.yaml#/paths/~1install~1*`.

## Goals
- Guided install flow inside the live-boot portal that lists eligible disks, warns about erase impact, and requires explicit confirmation as per the acceptance feature.
- Provide a zero-write “Simulate” (plan) mode, optional “Fetch latest signed image”, and final “Run install” with progress + post-install instructions.
- Maintain the Piccolo UI charter promises: calm copy, AA contrast, ≤3 deliberate taps per step, responsive layouts.

## Flow Outline
1. **Intro / Safety Check**
   - Explain what “Install to Disk” does, prerequisites (power/network), and link to install PRD section.
   - CTA: “Choose target disk” → Step 2.
2. **Select Target Disk**
   - Fetch `/install/targets` (Svelte Query) showing model, size, `/dev/disk/by-id/...`, existing partitions.
   - Each card includes “Type disk id to confirm” field; disable Continue until confirmation matches selected disk id.
   - Secondary action: “Simulate first” (skip confirmation; go to Plan step with simulate flag).
3. **Plan / Simulate**
   - Call `/install/plan` with `{target}`. Render summary: what gets erased, estimated time, root resize, identity preservation.
   - Provide CTA `Run Install` (requires user to retype disk id) plus `Download plan` (optional future) and `Back`.
4. **Fetch Latest (optional)**
   - Inline panel offering `/install/fetch-latest`; show signature verification result and release notes snippet if backend returns metadata.
   - If skipped, continue with embedded image plan; if accepted, switch plan to latest payload.
5. **Execution / Progress**
   - Trigger `/install/run` (mutation). Display progress states: downloading, writing, expanding, verifying, reboot prompt.
   - Surface failure reasons with retry/back links.
6. **Post-install Instructions**
   - Remind user the device will reboot and portal will be available in ≤60s at `http://piccolo.local`.
   - Offer CTA to “Reboot now” (API call TBD) or copy instructions.

## API Map
- `GET /install/targets` → list of `{id, model, size_bytes, partitions[]}`.
- `POST /install/plan` body `{target_id: string, fetch_latest?: boolean}` → returns `{steps[], warnings[], needs_confirmation_id}`.
- `POST /install/fetch-latest` → `{message, download_url}`; confirm signature + update plan.
- `POST /install/run` body `{target_id, fetch_latest?, acknowledge_id}` → `202/200` with job token; UI polls job status (follow-up: `/events` or `/install/status`).

## States & Components
- Use existing `Stepper` (Intro → Disk → Plan → Install → Finish).
- Add `DiskCard`, `PlanSummary`, and `ProgressLog` components under `src/lib/components/install/`.
- Errors: display inline alert with remediation (e.g., “Disk busy – unmount partitions from terminal”). Provide `Copy logs` action (calls `/logs/bundle`).
- Loading: skeleton cards for targets, spinner for plan.
- Accessibility: focus trap within modal-style steps, ensure confirmation input is labelled.

### Visual skin
- Hero shell mimics the setup wizard: frosted glass card on radial gradient background, but with a split layout (content rail + progress rail on desktop, stacked on mobile).
- Disk cards use elevation tier “surface container high” (shadow + accent stroke) and include device glyphs (Material Rounded storage icon) for quick scanning.
- Plan summary uses a timeline motif with accent dots + connecting line, matching the roadmap doc call for “calm control” statuses.
- Progress panel reuses AA accent/dark tokens, with a soft grid background to reinforce “device OS” identity.

## Telemetry & Follow-up
- Instrument step transitions and failure events (hook into existing event bus once available).
- Document open questions: job status endpoint, reboot trigger, ability to cancel install.

Update this doc as flows or APIs evolve.
