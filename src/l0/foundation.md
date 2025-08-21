# Piccolo-OS Foundations

This document captures the foundational decisions and guiding principles for **Piccolo-OS** — a custom OS powering the **Piccolo personal cloud device**.  

---

## Vision

Piccolo-OS is a **minimal, immutable, container-native OS** designed to run 24×7 on headless personal cloud devices.  
Its only interface is the HTTP API/UI of **`piccolod`**, which orchestrates everything else.  
The OS itself must guarantee immutability, reliability, and self-updating behavior with no need for SSH or direct user intervention.

---

## Core Design Principles

1. **Container-Native**  
   - All workloads (including user applications) run as containers.  
   - The OS provides just enough to support container orchestration.  

2. **Immutability & Reliability**  
   - Root filesystem is immutable.  
   - Automatic updates with rollback support are mandatory.  

3. **Headless by Default**  
   - No SSH access for users in production.  
   - Device is controlled solely via **`piccolod`** APIs/UI.  

4. **Tight Integration of Piccolod**  
   - `piccolod` is a **native systemd service** baked into the OS image.  
   - This ensures it is inseparable from the OS, making it the "control plane" for the device.  

5. **Security First**  
   - Secure boot and UEFI support are non-negotiable.  
   - Measured boot and remote attestation will be supported **within `piccolod` itself**, minimizing external dependencies.  

---

## Base OS Decision

- **Base OS:** Fedora CoreOS (FCOS)  
  - Rationale:  
    - Strong support for UEFI + Secure Boot.  
    - Ignition + Butane provide a clean, declarative configuration mechanism.  
    - Automatic updates (OSTree-based) with rollback support.  
    - Backed by Red Hat with long-term support and active development.  

- **Why Not Flatcar?**  
  - UEFI + Secure Boot support less mature.  
  - Smaller ecosystem and community.  

---

## Piccolod Design

- Runs as a **systemd-managed binary** on the host (not in a container).  
- Provides:  
  - HTTP API / Web UI (sole control surface).  
  - Automatic update orchestration.  
  - Integration with container runtime (Podman/Docker).  
  - Device security features (measured boot, remote attestation, integrated into `piccolod`).  

- **Design Principle:** Maximum functionality inside `piccolod` itself → fewer moving parts, tighter guarantees, more flexibility.  

---

## Build Strategy

1. **Phase 1 – Move Fast (Dev/Test Loop)**  
   - Use **vanilla Fedora CoreOS + Ignition/Butane** to inject `piccolod` for rapid iteration.  
   - Iterate quickly on the binary + service design without rebuilding OS images.  

2. **Phase 2 – Foundation Lock-in (Custom OS Build)**  
   - Once stable, bake `piccolod` into a **custom CoreOS derivative image**.  
   - This ensures `piccolod` is inseparable and always available.  
   - Custom OS build pipeline will produce artifacts that automatically roll out to beta/prod devices.  

3. **Phase 3 – Distribution & Updates**  
   - Build pipeline publishes signed OSTree updates.  
   - Devices pull updates automatically.  
   - Beta/stable channels supported for controlled rollout.  

---

## Update & Distribution Pipeline

- **Automated Build Pipeline:**  
  - Triggered on changes to `piccolod` or base configuration.  
  - Produces signed OSTree commits + bootable images.  

- **Release Channels:**  
  - **Beta:** Early testers, faster updates.  
  - **Stable:** Production users, well-tested releases.  

- **Automatic Rollouts:**  
  - Devices receive updates automatically.  
  - Rollback supported via OSTree if failures occur.  

---

## Open Questions

1. Do we want to build **our own OSTree update server** or leverage FCOS tooling directly?  
2. Should `piccolod` manage **all container orchestration** itself, or should it delegate to an existing runtime (e.g., Podman with systemd integration)?  
3. How early should we integrate secure boot + attestation into the dev flow?  

---

# Piccolo Storage and Encryption Design

## Core Concepts

* **Decentralized Storage**: Each user's Piccolo device never stores their own data. Instead, it stores encrypted fragments of other users’ data.
* **Fragmentation**: Files are split into `N-1` fragments (where `N` is the number of users in the network). Each fragment is distributed to a different Piccolo device.
* **Encryption Layers**:

  * **File Encryption Key (FEK)**: Each file is encrypted with a unique FEK.
  * **Key Encryption Key (KEK)**: The FEK is encrypted with the user’s KEK.
  * **TPM Key**: All persistence to local disk on a Piccolo device is transparently encrypted with a TPM-backed key.

## Data Flow

1. **User File Handling**:

   * User1 uploads a file.
   * File is encrypted with FEK.
   * FEK is encrypted with User1’s KEK.
   * File is fragmented into 9 pieces (if there are 10 users total).
   * Fragments are distributed to User2–User10’s devices.

2. **Local Device Storage**:

   * Each Piccolo device only holds encrypted fragments belonging to other users.
   * The device also holds its **local metadata database** (SQLite), which contains mappings for file fragments, keys, and distribution details.
   * All this local storage is encrypted by the TPM-backed disk key.

3. **Metadata Database (SQLite)**:

   * Holds references to fragment ownership, distribution, and KEK-encrypted FEKs.
   * Extremely sensitive — leaking it would expose distribution structure.
   * Backup strategy:

     * SQLite DB is encrypted with the user’s KEK.
     * Backup is uploaded to the Piccolo central cloud.
     * Recovery only possible with user’s KEK (which never leaves the user).

## Security Considerations

* **Privacy**: No device ever holds its own data in raw form.
* **Security**: Multiple encryption layers:

  * FEK ensures confidentiality of the file.
  * KEK ensures only the owner can decrypt the FEK.
  * TPM-backed disk encryption ensures fragments and metadata at rest are safe.
* **Recovery**:

  * Device loss: User recovers SQLite DB from Piccolo cloud (requires KEK).
  * TPM replacement: TPM key is regenerated, and DB+fragments are restored from encrypted backup.

## SQLite Justification

* **Advantages**:

  * Lightweight, file-based, no server overhead.
  * Stable and widely used in embedded/distributed systems.
  * Strong reliability guarantees (ACID compliant).
  * Simple to backup and restore.
* **Why good fit here**:

  * Each Piccolo device has a self-contained metadata store.
  * Easy encryption of the whole DB with KEK.
  * Low operational complexity.

---

✅ **Conclusion**:

* No full-disk encryption required — TPM-backed disk protection is enough.
* Sensitive user metadata (SQLite DB) is encrypted with KEK and backed up to Piccolo cloud.
* File data fragments remain confidential due to layered encryption and distribution.
* SQLite is a perfect fit for local metadata storage in this architecture.
---

# Piccolo OS - Privacy, Security, and Data Management Design

## Core Principles

* **Privacy and security are the soul of Piccolo OS**. No one but the user should be able to access their data.
* Even if someone gains physical access to a device, they must not be able to access the data.
* Piccolo will operate on a **federated storage model**, where a user’s data is stored on and retrieved from other users’ Piccolo nodes.
* It is essential to ensure that bad actors are kept out of the Piccolo network. This necessitates a mechanism to verify that each Piccolo node is running a **genuine, untampered version of `piccolod`**.

## Genuine Piccolo Node Verification

* We require **measured boot and remote attestation**:

  * The boot process and binary integrity of `piccolod` are cryptographically measured.
  * Nodes can request and verify each other’s measurements before trusting them in the network.
  * This ensures only authentic, non-tampered software instances can participate in federated storage.

## Disk Encryption vs File-Level Encryption

* **Full-disk encryption (FDE)** will not be implemented, because:

  * It provides protection only if the device is powered off, not against runtime attacks.
  * It introduces performance overhead and complicates federated storage verification.
* Instead, **file-level encryption** will be used for critical data:

  * User files are encrypted before distribution.
  * Metadata (such as fragment locations) is protected within an encrypted SQLite database.

## SQLite Database Design

* **Role of SQLite**:

  * A lightweight, embeddable database with a single-file design.
  * Stores metadata, particularly mappings and details about distribution of a user’s file fragments across the network.

* **Backup & Recovery**:

  * The SQLite database is highly sensitive and critical for recovery.
  * To ensure resilience, backups will be encrypted and stored in **Piccolo Central Cloud**.
  * Backup encryption uses a **Key Encryption Key (KEK)** derived from the user’s credentials or hardware root of trust.
  * This ensures that only the rightful user can decrypt and restore the database.

* **Handling Encryption at Rest**:

  * SQLite normally performs in-place modifications (append, B-tree rewrites, etc.) rather than full rewrites.
  * Directly encrypting the whole database file at rest would mean portions of it may temporarily be in plaintext if not handled carefully.

* **Proposed Solution**:

  1. The database file on disk is **always encrypted**.
  2. On startup, Piccolo:

     * Loads the encrypted SQLite file from disk.
     * Decrypts it using the KEK.
     * Mounts it into an **in-memory filesystem**.
  3. All read/write/query operations happen **entirely in memory**.
  4. On commit or checkpoint, the database is re-encrypted and persisted back to disk.
  5. WAL (Write Ahead Log) and temp files are also memory-mapped to avoid plaintext leaks.

* **Advantages**:

  * Guarantees that the database never exists in plaintext on disk.
  * Provides high-performance query execution with SQLite’s normal semantics.
  * Ensures crash recovery is possible (since periodic re-encryption and syncs to disk occur).

* **Trade-offs**:

  * Increased RAM usage (database size must fit in available memory).
  * Potential performance overhead on encryption/decryption during commits.
  * Requires careful handling of crash consistency to prevent corruption.

## Choice of SQLite

* **Why SQLite is a good fit**:

  * Embedded, no server process required.
  * Lightweight and portable.
  * Well-supported and widely audited for reliability.
  * Single-file design aligns with Piccolo’s simplicity principle.
* With the in-memory + encrypted persistence approach, SQLite becomes both **secure and performant** for Piccolo’s needs.
