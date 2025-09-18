# Piccolo OS Pre‑Beta PRD

Purpose: Ship a dependable, privacy‑first image that tinkerers can boot, install a curated app, and optionally publish it remotely with device‑terminated TLS, while keeping all user data encrypted at rest and gated by an admin unlock.

## Scope & Targets
- Architectures: x86_64 UEFI (Secure Boot) and Raspberry Pi aarch64.
- TPM‑only remote unlock. Non‑TPM devices require LAN unlock after every reboot.
- TPM-dependent flows are stubbed on non-TPM pre-beta hardware; remote unlock UX is present but hardware-backed unseal will land after TPM boards arrive.
- Base OS: SUSE MicroOS with piccolod baked in. No SSH/shell/serial access; the product surface is the piccolod web portal.

## Networking Model
- Portal: binds on `tcp/80` (LAN); remote access via Nexus L4 passthrough; device terminates TLS.
- Service networking uses a three-layer port model: containers expose their declared `guest_port` (Layer 1), piccolod binds loopback security ports in the 15000–25000 range (`127.0.0.1:<host_bind_port>`, Layer 2), and piccolod proxies LAN traffic on managed ports in the 35000–45000 range (`0.0.0.0:<public_port>`, Layer 3).
- mDNS: `http://piccolo.local` (internal implementation; Avahi not used).
- WAN: closed by default; public access only via Nexus. Nexus never terminates device traffic.

## User Journey (Happy Path)
1) Boot device → visit `http://piccolo.local` → create a strong admin password → auto‑login.
2) Install curated app (WordPress + SQLite) → local URL is shown; app runs in a rootless container.
3) Enable remote access → configure domain + Nexus endpoint/JWT → device serves portal with device‑terminated TLS.
4) Reboot behavior → volumes locked; on TPM devices the portal is reachable remotely over HTTPS in locked mode for admin sign‑in; non‑TPM requires LAN unlock.

## Security & Privacy
- No shell: disable SSH/serial/TTY by default in pre‑beta images.
- Input validation: strict validation at API boundaries, with context timeouts on I/O and network calls.
- Auth: Argon2id password hashing; secure, HttpOnly cookies; CSRF protection; login rate‑limiting and lockouts.
- Least privilege: rootless containers where possible; drop capabilities; containers bind to 127.0.0.1 and are proxied by piccolod.
- Logging: never log secrets; redact sensitive data.

## Encryption Architecture
- Per‑app encrypted directories (gocryptfs‑style) under `/var/piccolo/apps/<app>/data`.
- Keys
  - SDEK: random per‑directory data key used by gocryptfs.
  - KEK: derived via Argon2id(admin password + salt). Optional TPM‑sealed pepper to harden KEK derivation. TPM never auto‑unlocks data.
  - Recovery Key (RK): 24‑word mnemonic; can rewrap SDEK without re‑encrypting data.
- Policies
  - Unlock required after every reboot (password or RK). Strict mode may require password + TPM.
  - “Stolen lock” tamper flag can refuse unlock until cleared by the owner.
- Lifecycle: init encrypted dirs on first‑run; mount on unlock; rotate KEK to rewrap SDEK; purge shreds SDEK and metadata.

## TPM‑Gated Remote Reachability (Locked Mode)
- Non‑TPM devices: remote unlock unavailable; must unlock on the LAN; remote publish resumes only after unlock.
- TPM devices: remote portal can be available over HTTPS in locked mode within ≤ 5 minutes after reboot.
- TEK approach
  - Generate TEK (256‑bit) and seal to TPM PolicyPCR (PCR7 pre‑beta). Store sealed blob at `/var/piccolo/tpm/sealed_tek`.
  - On boot, unseal TEK and decrypt the auto‑unlock secrets directory `/var/piccolo/auto-unlock/` (AES‑256‑GCM per file):
    - `nexus.jwt.enc` — full‑claim JWT (Phase 1) for Nexus tunnel while locked.
    - `portal_acme_account.key.enc` — ACME account key (e.g., ECDSA P‑256).
    - `portal_tls.key.enc` — portal TLS private key; `portal_tls.chain.pem` stores public chain (may be plaintext).
  - Start portal HTTPS using the TEK‑decrypted key; data and apps remain locked until admin unlock.
- PCR continuity: during OS updates, pre‑authorize the next PCR state (PolicyOR) and reseal TEK so remote reachability persists. On mismatch, suspend remote reachability; require LAN unlock to re‑seal.

## Remote Publish & TLS
- DNS: user points Nexus A/AAAA to VPS; CNAME `portal.<domain>` and `*.pclo.<domain>` (or `*.<domain>`) to Nexus. Portal/app split is configurable.
- Listener hostnames: each listener publishes as `https://<listener>.<user-domain>[:remote_port]`. If `remote_ports` are omitted in the manifest, piccolod advertises 80 and 443.
- ACME: lego; HTTP‑01 over Nexus tunnel; Let’s Encrypt staging for tests.
- Portal TLS
  - TPM devices: portal HTTPS available in ≤ 5 minutes post‑reboot using TEK‑decrypted key; ACME account key also TEK‑protected.
  - Locked mode serves only the login UX and `/.well-known/acme-challenge/*` for renewals.
- Apps: app certs/keys live in encrypted storage and are available only after unlock; issuance starts post‑unlock.
- Future (Phase 2): optional portal‑only JWT sealed in TPM; full‑claim JWT remains in encrypted storage.

## Updates & Reboot Policy
- OS: transactional‑update enabled.
- Reboots: no unattended reboots by default. If a maintenance window is scheduled, prompt at window start; defer if unattended.
- Post‑reboot: TPM device exposes remote portal (locked) within ≤ 5 minutes; non‑TPM requires LAN unlock.

## Portal UX (Minimum)
- First‑run: password setup → auto‑login.
- Dashboard: system/network/storage/services/remote access/update status; independent sections.
- Storage: add disk; create encrypted dirs; mount/unmount; show lock state; display/rotate Recovery Key.
- Services: catalog (WordPress+SQLite), install/start/stop/restart, logs, local URL; uninstall with optional purge.
- Remote access: configure Nexus endpoint/JWT + hostnames; preflight DNS/challenge; status and renewal schedule.
- Updates: current/available, apply + reboot coordination; rollback indicator.
- Locked mode (TPM): minimal portal with sign‑in; ACME HTTP‑01 responder; no other APIs.

## Acceptance Criteria
- Boot & discover
  - `http://piccolo.local` responds in ≤ 60 seconds after boot on LAN.
  - TPM device: remote portal reachable via HTTPS in ≤ 5 minutes after reboot; shows locked state until sign‑in.
  - Non‑TPM: remote portal not reachable while locked; LAN portal shows locked state.
- First‑run & unlock
  - Strong password enforced; Argon2id; secure cookies; CSRF; rate‑limited login.
  - Encrypted volumes mount only after unlock; persist across reboot.
- Curated app
  - WordPress+SQLite installs in ≤ 2 minutes on a mid‑range NUC; local URL works; survives reboot (after unlock).
- Remote publish
  - Device terminates TLS; Nexus remains L4. Portal cert issued via HTTP‑01 and renews automatically; keys never leave device.
- Encryption
  - 100% user data (app data, app TLS, ACME account) encrypted at rest.
  - TEK never stored plaintext; unseals only under accepted PCR policy.
  - Recovery Key path validated; rewrap without re‑encrypting data.
- Updates
  - No unattended reboots by default; user‑initiated reboots apply updates; rollback verified.
- Performance/UX
  - Mobile viewport 360×800 usable; no blocked flows; NTP available.
- Security sanity
  - Only ports 80 and the managed proxy range (35000–45000) are exposed on LAN; loopback guard ports (15000–25000) never egress; Nexus is the sole remote path; no SSH; secrets not logged.

## Out of Scope (Pre‑Beta)
- Measured boot hardening beyond PCR7; remote attestation.
- DNS‑01 wildcard issuance; mTLS to Nexus; portal‑only JWT split (Phase 2).
- Additional curated apps (Immich, Vaultwarden) until core paths stabilize.
- Local TLS on LAN (device CA) — consider later.

## Build & Deliverables
- Images
  - x86_64 UEFI Secure Boot MicroOS with piccolod, gocryptfs, fuse3, tpm2‑tss, tpm2‑tools.
  - RPi aarch64 MicroOS image with piccolod, gocryptfs, fuse3 (no Secure Boot); remote unlock disabled.
- Flashing: write `.img` to USB (x86_64) or microSD (RPi); provide concise instructions.
- Smoke tests
  - QEMU x86_64 + swtpm: first‑run, TPM locked‑mode reachability, unlock, app install, HTTP‑01, update/reboot.
  - QEMU aarch64: first‑run, LAN unlock requirement, app install.
- Repo commands
  - Build daemon: `go build ./cmd/piccolod` (inject version via `-ldflags "-X main.version=<ver>"`).
  - Tests: `go test ./...` and `go test -cover ./...`.
  - Image build: `src/l0/build.sh [dev|prod]`; artifacts under `src/l0/releases/<version>/`.

## Data & Secret Paths
- Auto‑unlock (TPM only)
  - TEK sealed blob: `/var/piccolo/tpm/sealed_tek`
  - Encrypted secrets dir: `/var/piccolo/auto-unlock/`
    - `nexus.jwt.enc`
    - `portal_acme_account.key.enc`
    - `portal_tls.key.enc`
    - `portal_tls.chain.pem`
- Encrypted app data: `/var/piccolo/apps/<app>/data` (gocryptfs mount)
- App certs (encrypted after unlock): `/var/piccolo/certs/<app>/`

## Risks & Mitigations
- PCR drift after updates → Pre‑seal next PCRs with PolicyOR; on mismatch, require LAN unlock and re‑seal.
- ACME rate limits → Exponential backoff; single renewal worker; use staging in tests.
- RPi variability → Publish tested models; document power/SD quality requirements.
- SQLite limits in WordPress → Adequate for “aha”; note constraints; plan upgrade path later.

## References
- Product PRD (org‑context): `org-context/02_product/piccolo_os_prd.md`
- Acceptance features (org‑context): `org-context/02_product/acceptance_features/`
- Piccolod architecture/docs (this repo): `docs/`
