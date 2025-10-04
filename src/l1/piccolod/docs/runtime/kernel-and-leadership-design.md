# Kernel and Leadership Design — Checkpoint

_Last updated: 2025-10-04_

This document captures the current system design decisions for piccolod as a mini‑OS runtime, with a clear separation between the kernel (L1 modules owned by piccolod) and user/system apps (L2 containers). It also records leadership/lock semantics, routing, persistence consistency, and the near‑term implementation plan.

## Terms
- Kernel: piccolod core modules (supervisor, event bus, consensus client/registry, persistence, router/tunnel, mDNS/SD, telemetry, API server). Kernel services always run; role changes switch behaviour (active vs standby) rather than killing the kernel.
- Apps (L2): container workloads (user apps and platform “system apps” that live as containers). Their runtime behaviour depends on leadership policy.
- Resources:
  - `cluster.ResourceKernel`: kernel/control‑plane leadership resource.
  - `cluster.ResourceForApp(<name>)` → `app:<name>`: per‑app leadership resource derived from the app spec name.

## Leadership and Lock Semantics
- Kernel management operations (install/upsert/start/stop/uninstall/enable/disable/update‑image) require: unlocked AND kernel leader. We gate these in the AppManager; API will add leader‑hints for followers.
- App runtime operations:
  - Leader → serve traffic locally and attach volumes RW.
  - Follower → stop local container and route/tunnel traffic to leader; volumes are RO (or detached) per warm/cold policy.
- Persistence is the source of truth:
  - Control‑store writes: allowed only on kernel leader.
  - App volume writes: allowed only on app leader; followers attach RO or detach.
  - Reads always allowed; API may still redirect for UX.

## Routing and Redirects
- Kernel follower for write endpoints returns a leader hint (HTTP 307/409 + leader address). Read‑only GETs may remain local.
- App follower: a Router component decides local vs tunnel. We add a stub now with `RegisterAppRoute(app, mode=local|tunnel, leaderAddr)` and integrate later with TCP/UDP/QUIC forwarding.

## Health Semantics
- Follower is not degraded by default. Readiness marks router/service‑manager as `OK` and annotates role=leader/follower (standby). Only true faults (not role) should surface as WARN/ERROR.

## Control‑Store Revision and Consistency
- The control store carries a monotonic `rev` that increments on every committed write.
- Leader publishes a `ControlRevChanged{rev}` hint; followers never act on the hint alone.
- Followers poll `Control().Revision()` and only reconcile once local `rev` ≥ announced rev and payload verifies (decrypt/checksum/parse). This makes us safe under block‑level eventual replication.
- Stronger semantics (future): VolumeManager offers `FlushAndFence` and followers report `AppliedFence` so the leader only announces rev after blocks are durable on replicas. Alternatively, couple rev publication to consensus log commit/application. Followers still verify locally.

## Module Behaviour
- Kernel modules: run everywhere. Role flips switch behaviour to standby (e.g., router stays up to forward), not shutdown. Components that don’t hot‑reload cleanly declare `RestartOnKernelRevChange` so the supervisor restarts them when `rev` advances on followers; reloadable components re-read config instead.
- Apps: per‑app leadership events stop/start containers and notify the router and service manager. Persistence gates RO/RW at attach time.

## Dispatcher and Events
- Dispatcher is used for command routing (persistence, remote). Enforcement is handled in managers/persistence; we avoid duplicating lock/role checks in middleware. Middleware remains available for metrics/auditing.
- Event bus topics:
  - `LockStateChanged` — emitted by persistence on lock/unlock.
  - `LeadershipRoleChanged` — emitted by consensus for kernel and apps.
  - `ControlStoreCommit` — emitted when a follower observes `rev` advance locally (and optionally when leader commits). Managers subscribe to reconcile/reload.

## Near‑Term Implementation Plan
1. Introduce `cluster.ResourceKernel` and refactor kernel role checks to use it explicitly.
2. Router stub: `internal/router.Manager` with `RegisterAppRoute`; wire app leadership changes to choose `local|tunnel` (log only for now).
3. Control‑store `rev` and commit events: bump on save, publish hints, follower poller emits `ControlStoreCommit{rev}` when local rev advances.
4. Persistence gates: refuse control‑store writes on kernel followers; enforce app volume RO for followers.
5. API: add kernel follower leader‑hints (redirect/JSON) on write endpoints.
6. System‑app policy map (placeholder) to classify L2 platform containers (leader_only, follower_ok_ro, follower_proxy, always_on).

## Rationale Recap
- Events are hints; correctness comes from local `rev` verification.
- Leadership is scoped by resource: kernel for management, `app:<name>` for runtime.
- Standby is a first‑class, healthy state.
- Persistence is always the final enforcement point; higher layers guard early for UX and error clarity.

