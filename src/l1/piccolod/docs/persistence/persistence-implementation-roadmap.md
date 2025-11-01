# Persistence Implementation Roadmap

_Last updated: 2025-10-01_

The goal is to transition the persistence module from scaffolding into the production-safe substrate described in the PRD and persistence design doc. We will land the work breadth-first: make the current surfaces functional (even with limited policy), then iterate on depth (replication, device orchestration, cold tier).

## Phase 0 – Foundations (current work)
- Provide an encrypted control-store implementation backed by the SDEK managed by `internal/crypt.Manager`.
- Wire crypto lock/unlock flows through the dispatcher so persistence authoritatively controls lock state.
- Replace persistence stubs for repositories with working read/write logic that persists across restarts.
- Centralize all state paths via `internal/state/paths` and fail builds/tests if new code embeds `/var/lib/piccolod` directly.
- Keep VolumeManager/DeviceManager/ExportManager as stubs but ensure the module no longer panics when unlocked.

## Phase 1 – Volume orchestration (ship-ready)
- Implement a minimal storage adapter that can create encrypted directories per volume (initially using gocryptfs-style wrapping with the in-process SDEK).
- Teach VolumeManager to ensure/attach/detach volumes for the control plane and application namespaces.
- Mount the bootstrap volume at `PICCOLO_STATE_DIR` before other modules attempt persistence writes.
- Maintain a journal at `volumes/<id>/state.json` that records `desired_state`, `observed_state`, `role`, `generation`, `needs_repair`, and the last error. All writes are temp-file + fsync + rename so crash recovery reads a consistent snapshot.
- Publish every journal transition on `events.TopicVolumeStateChanged` with the same payload so supervisors, the portal, and cluster coordination can react without scraping the filesystem.
- On startup, sweep the journal and reconcile reality: auto-reattach/detach volumes to match intent, clear `needs_repair` once the desired state is observed, and flag failures so higher layers can block leadership promotion or user writes.
- Store pre-unlock runtime assets (remote Nexus config, ACME account cache, portal TLS material) on the bootstrap volume so remote access and HTTPS work before the control store unlocks; reseal with TPM keys when available.
- Have persistence publish role updates per volume and verify service/app managers respond (stop followers, mount leaders RW). Followers must default to read-only/detached until the leadership registry authorizes promotion.

## Phase 2 – Control-store repos & API surface
- Flesh out repositories (`AuthRepo`, `RemoteRepo`, `AppStateRepo`, etc.) with schema-level validation and transactional guarantees.
- Gate API endpoints on lock-state events instead of checking the crypto manager directly.
- Introduce repair/health tooling (`PRAGMA quick_check`, WAL checkpoints, `VACUUM INTO`) and surface results on the event bus. _(Shipped October 2025: periodic quick_check monitor with WAL checkpoint cadence publishing `TopicControlHealth` events.)_

## Phase 3 – Exports and recovery
- Replace placeholder export artifacts with deterministic PCV bundles on disk.
- Implement import flows that remount the control store read-only until unlock completes, and verify bootstrap rebuild hooks execute.
- Add integration tests covering control export/import and the persistence HTTP endpoints.

## Phase 4 – Cluster and federation integration
- Integrate a real consensus module (Raft) to distribute leadership/replication signals.
- Implement hot-tier replication, warm/cold follower policies, and federation snapshot jobs.
- Extend the event bus with export progress, device health, and replication lag metrics.

Each phase is expected to leave the system runnable: later work may extend functionality but should not regress previously delivered guarantees. The immediate focus is Phase 0.
