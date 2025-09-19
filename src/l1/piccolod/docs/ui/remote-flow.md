# Remote Access UI Flow (Pre-Beta)

Purpose: translate the PRD remote access expectations into a single-screen experience in `/remote`.

## Screen Structure

1. **Status Banner**
   - Shows `state` (`disabled`, `preflight_required`, `active`, `warning`, `error`).
   - Displays leading message (DNS mismatch, renewal failure, tunnel offline) with link to "View preflight details" drawer.
   - Actions: `Run preflight` (always visible) and `Disable remote access` when active.

2. **Connection Overview Card** (left column)
   - Fields: solver, Nexus endpoint, portal hostname, tunnel latency, last handshake.
   - Summarizes current secrets status and next renewal.
   - Secondary actions: `Rotate credentials`, `Download guide` (links to docs).

3. **Configuration Workspace** (right column)
   - **Step 0: Provision Nexus helper** (optional modal)
     - `Set up Nexus server` opens a dialog with VM requirements, the installer command, and inputs for the Piccolo TLD, Nexus endpoint, and JWT secret.
     - After running the script, the admin pastes the generated secret and clicks `Verify & continue`; success timestamps the helper and closes the modal.
   - **Step 1: Nexus connection**
     - Endpoint (`wss://…/connect`) and JWT secret come from the installer output; both are always required.
   - **Step 2: Domain setup**
     - Collect the Piccolo domain/TLD (e.g., `myname.com` or `piccolo.myname.com`).
     - Portal host options: use the TLD itself or supply a subdomain prefix (defaults to `portal.`). Derived guidance shows the wildcard record (`*.tld`) and a listener preview.
     - UI reminders prompt the admin to aim the wildcard and portal records at the Nexus host (CNAME or A/AAAA).
   - **Step 3: Solver selection**
     - Radio buttons: `HTTP-01` (default) or `DNS-01`.
     - HTTP-01 requires no extra inputs beyond the shared fields; it just reiterates the port 80/443 requirement.
     - DNS-01 adds a provider picker populated from Lego metadata plus provider-specific credential fields.
   - **Step 4: Save & Preflight**
     - `Save configuration` triggers a preflight checklist (DNS → tunnel → ACME → alias coverage).
     - On success, the status banner updates; failures show actionable guidance inline.

4. **Alias Management Table**
   - Rows by listener (`portal`, `vaultwarden`, etc.).
   - Columns: Listener, Piccolo host, Alias chips (status pill per alias), Last validation, Actions.
   - `Add alias` dialog: alias FQDN, listener select, DNS hint; verifies CNAME before committing.
   - Row actions: `Retry validation`, `Remove alias`.

5. **Certificate Inventory**
   - Table columns: Domains, Solver, Issued (date), Expires (days), Next renewal (timestamp), Status.
   - Rows highlight ≤10 days (amber) and ≤3 days (red). Button `Renew now` per row.
   - Toolbar shows aggregated counts (Active, Warning, Expiring soon).

6. **Event Timeline**
   - Chronological list of preflight attempts, renewals, alias validations.
   - Each item shows timestamp, severity icon, message, suggested next step.

## State Machine

- `disabled` → `provisioning` (after Nexus helper verified) → `configured` (solver selected, config saved) → `active`.
- Errors move to `warning` (recoverable) or `error` (blocking) but retain last-known config.
- Preflight always returns latest `checks` array with `name`, `status`, `detail`, `next_step`.

## API Expectations

- `GET /remote/status` returns:
  ```json
  {
    "state": "active",
    "solver": "http-01",
    "endpoint": "wss://nexus.example.com/connect",
    "hostname": "mybox.example.com",
    "latency_ms": 42,
    "last_handshake": "2025-09-19T18:25:43Z",
    "next_renewal": "2025-10-01T00:00:00Z",
    "warnings": ["Alias forum.mybusiness.com pending CNAME"],
    "aliases": [...],
    "certificates": [...]
  }
  ```
- `POST /remote/configure` accepts solver-specific payload plus `aliases` delta.
- `POST /remote/preflight` triggers validation; response mirrors checklist.
- `GET /remote/dns/providers`, `POST /remote/dns/dry-run` support DNS-01 metadata.
- `GET /remote/aliases`, `POST /remote/aliases`, `DELETE /remote/aliases/{id}` manage alias rows.
- `GET /remote/certificates`, `POST /remote/certificates/{id}/renew` drive inventory.
- `POST /remote/nexus-guide/verify` records helper completion.

## Validation & UX Notes

- All forms debounce validation but allow manual `Re-run preflight`.
- Mask JWT secret fields; show generated token only once after provisioning.
- `Disable remote access` prompts confirmation and logs event in timeline.
- Alias coverage: when adding alias, highlight exact DNS instructions and auto-copy target host.
- Mobile layout: collapses into stacked sections with sticky status banner and CTA bar.
