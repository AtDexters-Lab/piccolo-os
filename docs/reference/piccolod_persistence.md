# Piccolod Persistence Model

## Overview
Piccolod persists its control plane in the Piccolod Datastore (PDD) and exposes pre-unlock runtime state through the Bootstrap Shard Store (BSS). Both are encrypted volumes packaged together in the Piccolo Vault (PCV) export so a device can recover completely from backups.

## Data Stores
- **PDD (Piccolod Datastore):** Encrypted control-plane volume that holds portal/session data, service specs, disk metadata, policies, backup targets, audit/event logs, and the wrapped AionFS encryption keybag. Unlock requires the admin password (with TPM pepper when available) or the 24-word recovery key; both unwrap the same Symmetric Data Encryption Key (SDEK) via independent Key Encryption Keys (KEKs).
- **BSS (Bootstrap Shard Store):** Separate encrypted volume that must be accessible before the admin authenticates. It carries only pre-unlock necessities: disk-level encryption key (DiEK), TLS/ACME/Nexus credentials, AionFS node identity, federation checkpoint token, and any runtime metadata AionFS needs to serve peers while the device is locked. On TPM hardware the SDEK is sealed to PCR policy and unseals automatically at boot; without TPM, BSS unlocks with the password and pre-unlock remote behavior stays disabled.

## Key Management
- Both volumes use gocryptfs-style encryption, each protected by an SDEK.
- The PDD keybag sits beside the ciphertext and wraps the single SDEK twice: `KEK_pwd` (admin password + TPM pepper in strict mode) and `KEK_recovery` (24-word recovery key). There are no duplicate SDEKs.
- BSS maintains its own TPM-sealed SDEK. No user secrets are stored inside BSS beyond what is needed pre-unlock.
- Piccolod releases exactly one AionFS Encryption Key (EK) after unlock. AionFS derives per-application Data Encryption Keys (DEKs) internally.

## Boot and Unlock Flow
1. **Boot:** TPM unseals the BSS SDEK, mounts BSS, and AionFS enters pre-unlock mode (federation heartbeat, DiEK-backed chunk handling). Piccolod remains sealed.
2. **Unlock:** The admin authenticates; piccolod derives `KEK_pwd`, unwraps the PDD SDEK, mounts PDD, and passes the AionFS EK to the storage service. AionFS pivots to full functionality, attaches volumes, resumes backups, and consumes any pre-unlock journal from BSS. On non-TPM devices both volumes unlock after authentication, so remote pre-unlock remains unavailable by design.

## Secret Rotation
- Rotations that touch BSS-resident items (TLS/ACME/Nexus credentials, DiEK, federation checkpoint) update the live BSS volume and immediately rerun integrity checks so exports pick up the new state.
- Controllers mutating post-unlock-only state continue writing to PDD.
- Strict mode is always enforced when TPM is present (password + TPM pepper required for PDD unlock).

## PCV Export and Recovery
- PCV bundles both encrypted volumes with their metadata (hashes, schema versions).
- Daily schedule: briefly quiesce writes, snapshot PDD and BSS ciphertext, record integrity hashes, package into the encrypted PCV artifact, reseal the BSS key to the current TPM PCR state, and resume services.
- Import restores both volumes atomically, reseals the BSS SDEK for the destination TPM, mounts PDD after unlock, delivers the EK to AionFS, and lets controllers reconcile services.

## Integrity and Drift Controls
- Hash-check BSS at boot/unlock and before every export; log and alert on mismatches so operators can restore from the most recent PCV if necessary.
- Apply the same checks to PDD so corruption is detected before services start.
- AionFS emits "shard changed" events whenever it mutates BSS, allowing piccolod to refresh recorded hashes and ensure exports stay current.
- Daily PCV exports keep drift windows short and maintain recent recovery points.

## Next Steps for Implementation
1. Define on-disk schemas for PDD (database layout, document structure) and finalize the keybag interface.
2. Specify piccolod ↔ AionFS APIs for pre-unlock mount handoff, post-unlock EK delivery, shard-change events, and export checkpoint payloads.
3. Build the daily export scheduler with crash-safe staging and integrity verification.
4. Add monitoring and alerting for volume mount failures, hash mismatches, and TPM unseal errors.
5. Develop tests covering unlock flows with/without TPM, PCV round-trips, shard corruption recovery, and secret rotation propagation.
