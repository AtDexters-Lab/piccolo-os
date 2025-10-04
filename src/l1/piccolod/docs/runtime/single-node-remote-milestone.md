# Single-Node Remote Access Baseline — Milestone Scope

_Last updated: 2025-10-04_

This document defines the scope, acceptance criteria, and implementation checkpoints for bringing up Piccolo OS on a single device with remote access, one physical disk, and no hot/cold tier replication.

## Goals
- Run piccolod on one device (“node”) with encrypted persistence and leadership/lock enforcement.
- Provide remote access via Nexus using the existing nexus-proxy-backend-client (opaque L4 tunneling).
- Deploy and serve at least one app locally and through Nexus.
- Exercise leadership hooks (kernel/app) and router decisions, even if single node remains the leader.

## Assumptions
- One physical disk; no additional devices.
- No multi-node cluster replication/tunnels yet (hooks exist; not exercised).
- Existing Nexus client library handles WSS, keepalive, and stream multiplexing.
- System containers (L2 “system apps”) are optional in this milestone; kernel modules (L1) are always running.

## Terminology
- Kernel: piccolod core modules (supervisor, bus, consensus/registry, persistence, router/tunnel, mDNS/SD, telemetry, API).
- Apps: user app containers managed by AppManager and ServiceManager.
- Resources:
  - `cluster.ResourceKernel` — kernel/control-plane role.
  - `cluster.ResourceForApp(<name>)` — app-level role (`app:<name>`).

## Deliverables
- Encrypted control store with repos (auth, remote, appstate) + monotonic `rev` and checksum.
- Single-disk VolumeManager: control/app volumes attach, encrypted directory backend acceptable for v0; AttachOptions enforce RO/RW.
- Leadership wiring:
  - Kernel leader required for management ops (install/stop/etc.).
  - App follower → stop only that app’s container; app leader → run locally.
- Router (stub):
  - `RegisterKernelRoute(mode=local|tunnel, leaderAddr)`; `RegisterAppRoute(app, mode, leaderAddr)`.
  - On Nexus inbound streams, choose local (this milestone) or tunnel (future device hop) based on leadership.
- Nexus integration:
  - Start/Stop client under supervisor; register portal + app listeners; `OnStream` → Router.
- API behaviour:
  - Kernel write endpoints operate locally when leader; on followers, requests arriving through Nexus would be forwarded (hook in place).
- Health:
  - Standby role (follower) reported as OK with role annotation; lock/unlock reflected.

## Out of Scope (kept as TODOs)
- Cross-device tunnels for app/kernel traffic (router supports the mode but not the transport here).
- Nexus enrollment flows beyond device secret/connect; DNS automation/ACME issuance.
- Block-replication fences; eventual consistency is acceptable with local `rev` verification.

## Acceptance Criteria
1. First boot → admin setup → unlock → `/health/ready` reports ready and components OK.
2. Configure remote → Nexus client connects (WSS) and registers listeners; remote status reflects config.
3. Install a sample app → service ports allocated → local HTTP proxy serves it.
4. Remote access: inbound stream for the app proxies to the local app via Router.
5. Simulated app follower event: local app stops; Router switches route to tunnel (log-only in single-node); no local serving.
6. Auth/remote changes bump control-store `rev`; follower poller (if simulated) emits commit events; managers remain consistent.

## Test Plan (manual or automated)
- Unit tests: persistence `rev` bump; AppManager leader/follower reactions; Router stream hand-off (with fake Nexus client).
- Integration (dev env): 
  - Start piccolod; call `/api/v1/crypto/setup` then `/unlock`.
  - Call `/api/v1/remote/configure` with Endpoint + DeviceSecret.
  - POST `/api/v1/apps` with a simple HTTP app; confirm `/remote/status` and local proxy.
  - Simulate `LeadershipRoleChanged{app:demo,follower}` on the bus; confirm app stopped and router logs “tunnel”.

## Implementation Checklist
- [ ] Add `cluster.ResourceKernel`; refactor kernel checks.
- [ ] Router stub + leadership hook; connect Nexus client `OnStream` to Router.
- [ ] VolumeManager single-disk attach; enforce RO/RW per role.
- [ ] Control-store `rev` + checksum; follower poller; `TopicControlStoreCommit`.
- [ ] Gate control-store writes on kernel leader; app volume RW on app leader.
- [ ] Remote Manager ↔ Nexus client adapter and supervisor component.
- [ ] Documentation and AGENTS.md backlinks.

## References
- Runtime roadmap: `src/l1/piccolod/docs/runtime/runtime-architecture-roadmap.md`.
- Kernel/app leadership design: `src/l1/piccolod/docs/runtime/kernel-and-leadership-design.md`.
- Persistence design: `src/l1/piccolod/docs/persistence/persistence-module-design.md`.

