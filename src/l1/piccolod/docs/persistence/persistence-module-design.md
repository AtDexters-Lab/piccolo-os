# Piccolod Persistence Module Design (checkpoint)

_Last updated: 2025-10-03_

## Goals
- Provide an embedded, encrypted persistence layer for the piccolod control plane and application data.
- Support single-node and clustered deployments with clear leader/follower semantics.
- Ensure no plaintext rests on disk; all persistence flows through gocryptfs-style encrypted volumes.
- Enable repeatable recovery via PCV exports, federation replication, and external snapshots.

## Module Topology
```
RootService
├── BootstrapStore      (device-local bootstrap volume)
├── ControlStore        (control-plane repos over encrypted SQLite)
├── VolumeManager       (AionFS-backed encrypted volumes & replication)
├── DeviceManager       (disk discovery, health, parity orchestration)
└── ExportManager       (control-only + full-data export/import)
```

For a path-by-path catalog of persisted artifacts, consult
[`persistence-inventory.md`](./persistence-inventory.md).

All components share a core event bus. The consensus/leader-election layer feeds a Leadership Registry that surfaces `Leader`/`Follower` state for each resource. Persistence subscribes to remount volumes appropriately, while app and service managers apply policy (e.g., warm vs cold followers) based on the app specification.

### Bootstrap mount (`PICCOLO_STATE_DIR`)
- `PICCOLO_STATE_DIR` points to the mount of the bootstrap volume. The volume manager is responsible for provisioning and mounting it before any control-plane writes occur.
- All durable data (control-store snapshot, export manifests, volume metadata, auth state) must live under this mount. Modules resolve paths via `internal/state/paths` so no package hard-codes `/var/lib/piccolod`.
- The cryptography keyset (`crypto/keyset.json`) currently remains on the host root. It is already wrapped by the SDEK and keeping it outside the bootstrap volume avoids creating a TPM single point of failure before we finish the TPM-backed unlock workflow. Once TPM wrapping is mandatory we must plan a migration to move the keyset safely onto the bootstrap volume.
- If the bootstrap volume is unavailable, persistence refuses to unlock/control-store writes; bootstrap provisioning is retried end-to-end instead of writing to the host filesystem.

## Volume Classes & Replication
- `VolumeClassBootstrap` – device-local, no cluster replication. Rebuilt after admin unlock using secrets from the control store. Holds TPM-sealed rewraps when available.
- `VolumeClassControl` – replicated to all cluster peers (hot tier). Only the elected leader mounts read/write; followers mount read-only.
- `VolumeClassApplication` – per-app volumes with tunable replication factors, optional cold-tier policies, and cluster-mode awareness.

VolumeManager handles encryption (gocryptfs-style), mount lifecycle, AionFS integration, and role change notifications.

### Cluster Modes
- `stateful` (default): single elected writer per service. Followers stay cold-standby by default; warm replicas allowed only when the workload tolerates read-only mounts.
- `stateless_read_only`: active-active replicas on every node; volumes are exposed read-only everywhere and the app must not mutate external state. No leader election is required for these services.

VolumeManager consults the app’s cluster mode when allocating volumes and relays leader/follower state to consumers. App orchestration decides whether followers remain attached read-only or fully detached. Control-store readiness also emits lock/unlock events (`TopicLockStateChanged`) on the shared bus so other modules can react (e.g., gate API actions).

### Command Interface
- Runtime components dispatch typed commands via the shared dispatcher (`internal/runtime/commands`).
- Initial commands: `persistence.ensure_volume`, `persistence.attach_volume`, `persistence.record_lock_state`, `persistence.run_control_export`, `persistence.run_full_export`.
- Lock/unlock flows use `persistence.record_lock_state` so that the module rebroadcasts control-store readiness on the event bus before other components proceed.
- Responses carry structured payloads (e.g., `VolumeHandle`, `ExportArtifact`). Future commands (import, status, repair) will extend this surface.

### Lock-state propagation
- Crypto APIs dispatch lock/unlock events through the dispatcher; the persistence module publishes `TopicLockStateChanged` with the new state.
- Service manager and app manager subscribe to the topic (current implementation logs transitions to confirm wiring; enforcement hooks will follow). Auth endpoints now surface HTTP 423 when control storage is sealed, matching the persistence event stream instead of relying on crypto manager polling.
- API middleware can eventually rely on the event stream instead of polled crypto checks to gate mutating requests.

## ControlStore
- Physical store: SQLite in WAL mode with `synchronous=FULL`, mounted on the control volume (`mounts/control/control.db`). The ciphertext tree only contains gocryptfs metadata while the database files live inside the mounted volume.
- Exposes domain repositories (`AuthRepo`, `RemoteRepo`, `AppStateRepo`, `AuditRepo`, etc.) instead of raw SQL handles. `AuthRepo` now persists the Argon2id admin password hash and initialization flag; reads/writes return `ErrLocked` until the control store is unlocked with the SDEK.
- `RemoteRepo` stores the full remote-manager configuration as an encrypted blob so remote API calls are gated by the same lock semantics (HTTP 423 until unlock) and replicate cleanly with the control shard.
- Only the leader performs writes. When the kernel role registry reports `Follower`, the persistence module mounts the control volume read-only; the SQLite store detects the RO mount, opens the database in `mode=ro`, and rejects mutating calls with `ErrLocked`. Followers therefore continue to hydrate state while preventing accidental writes.
- Transactions commit through the repositories; the interim JSON payload still flushes via fsync, and the SQLite layer preserves single-transaction semantics to keep the meta revision/checksum consistent.
- Health tooling: periodic `PRAGMA quick_check`, timer-based WAL checkpoints, automatic `VACUUM INTO`/`.recover` repair attempts. Failures emit events and block PCV exports until resolved.
- A background health monitor runs `PRAGMA quick_check` every five minutes and publishes summaries on `events.TopicControlHealth`. Leader commits invoke `PRAGMA wal_checkpoint(PASSIVE)` (bounded by a one-minute cadence) to keep the WAL bounded without impacting followers.

## PCV & Recovery
- PCV exports now quiesce persistence by temporarily re-locking the control store, detaching the gocryptfs mounts, and streaming the sealed ciphertext trees for the requested volumes. The copy is taken from `ciphertext/control/**` (and `ciphertext/bootstrap/**` for full exports), keeping the payload encrypted at rest while guaranteeing consistency.
- ExportManager offers two operator APIs (exposed at `POST /api/v1/exports/control` and `POST /api/v1/exports/full`):
  - Control-only exports bundle the control volume ciphertext for reinstating a node into an existing cluster/federation.
  - Full-data exports include both control and bootstrap ciphertext to recover standalone devices when federation replicas are unavailable.
- Imports mount the control plane read-only until an admin unlocks. At unlock, BootstrapStore rewraps portal/Nexus/DiEK secrets onto the local bootstrap volume.
- Recovery tiers:
  - Single-node: PCV + external full-volume snapshots.
  - Federated clusters: PCV + leader snapshots + cold tier.
  - Parity-only: PCV + parity rebuild; external snapshots still required for chassis loss.

## Identity & Cluster Auth
- Durable IDs: TPM EK/AIK fingerprints where available; generated device key (stored in bootstrap) otherwise.
- Device metadata (friendly name, routing prefs, roles) lives in the control store and replicates across the cluster.
- Internal mTLS uses a cluster CA in the control plane; each node holds a client cert tied to its durable ID in its bootstrap volume.
- Public ACME certificates serve end-user traffic only.

## Event Bus
- Topics include: `LockStateChanged`, `LeadershipRoleChanged`, `DeviceEvent`, `ExportResult`, `ControlStoreHealth`.
- Persistence publishes lock-state updates via the dispatcher; service manager and app manager already subscribe and log receipt to verify propagation.
- Modules subscribe to react (e.g., persistence remounts volumes on role change, app manager stops followers for stateful workloads, portal surfaces export failures).

## Open Tasks
1. Replace placeholder export artifacts with deterministic bundles, upload/download wiring, and integration tests.
2. Teach service/app managers to enforce leadership policy (stop followers, gate proxies) instead of only logging events.
3. Connect VolumeManager to a real storage adapter (encrypted mounts, device discovery) and integrate with lock-state gating.
4. Specify export manifest schema (control-only vs full-data) with checksums, metadata, and reseal hooks.
5. Build ControlStore repair utilities (quick-check, WAL compaction, TPM reseal) and document operational runbooks.
