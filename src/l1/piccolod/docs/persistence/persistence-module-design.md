# Piccolod Persistence Module Design (checkpoint)

_Last updated: 2025-09-30_

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

All components communicate via an internal event bus. The consensus/leader-election layer feeds a Leadership Registry that surfaces current roles (`Leader`, `FollowerCold`, `FollowerWarm`). App and service managers subscribe to these events to start/stop workloads and remount volumes.

## Volume Classes & Replication
- `VolumeClassBootstrap` – device-local, no cluster replication. Rebuilt after admin unlock using secrets from the control store. Holds TPM-sealed rewraps when available.
- `VolumeClassControl` – replicated to all cluster peers (hot tier). Only the elected leader mounts read/write; followers mount read-only.
- `VolumeClassApplication` – per-app volumes with tunable replication factors, optional cold-tier policies, and cluster-mode awareness.

VolumeManager handles encryption (gocryptfs-style), mount lifecycle, AionFS integration, and role change notifications.

### Cluster Modes
- `stateful` (default): single elected writer per service. Followers stay cold-standby by default; warm replicas allowed only when the workload tolerates read-only mounts.
- `stateless_read_only`: active-active replicas on every node; volumes are exposed read-only everywhere and the app must not mutate external state. No leader election is required for these services.

VolumeManager consults the app’s cluster mode when allocating volumes and emitting role changes. For `stateless_read_only` apps it publishes a fixed `FollowerWarm` role on every node and skips container stop/start churn.

## ControlStore
- Physical store: SQLite in WAL mode with `synchronous=FULL`, mounted on the control volume.
- Exposes domain repositories (`AuthRepo`, `RemoteRepo`, `AppStateRepo`, `AuditRepo`, etc.) instead of raw SQL handles.
- Only the leader performs writes. Transactions commit through the repositories; SQLite durability handles fsync.
- Health tooling: periodic `PRAGMA quick_check`, timer-based WAL checkpoints, automatic `VACUUM INTO`/`.recover` repair attempts. Failures emit events and block PCV exports until resolved.

## PCV & Recovery
- PCV exports package the control-plane volume plus device registry metadata (durable IDs, routing prefs). Bootstrap shards are not included; each device recreates its shard post-unlock.
- ExportManager offers two APIs:
  - Control-only exports for reinstating a node into an existing cluster/federation.
  - Full-data exports (control + all volumes) for standalone recovery when federation replicas are unavailable.
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
- Topics include: `LockStateChanged`, `VolumeRoleChanged`, `DiskEvent`, `ExportResult`, `ControlStoreHealth`.
- Modules subscribe to react (e.g., app manager stops followers on role downgrade, portal surfaces export failures).

## Open Tasks
1. Define Go interfaces for repositories and event bus topics/payloads.
2. Implement Leadership Registry glue between Raft and VolumeManager/App manager.
3. Specify export manifest schema (control-only vs full-data) with checksums and metadata.
4. Build ControlStore repair utilities and TPM reseal workflows.
5. Update reference docs (PRD/persistence guide) with the new PCV + bootstrap behavior and cluster-auth model.
