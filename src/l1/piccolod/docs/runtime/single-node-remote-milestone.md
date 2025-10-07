# Single-Node Remote Access Baseline — Milestone Scope

_Last updated: 2025-10-06_

Release target: 2025-10-08

This document defines the scope, acceptance criteria, and implementation checkpoints for bringing up Piccolo OS on a single device with remote access, one physical disk, and no hot/cold tier replication.

## Goals
- Run piccolod on one device (“node”) with encrypted persistence and leadership/lock enforcement.
- Provide remote access via Nexus using the existing nexus-proxy-backend-client (opaque L4 tunneling).
- Deploy and serve at least one app locally and through Nexus.
- Exercise leadership hooks (kernel/app) and router decisions, even if single node remains the leader.

## Scope Clarifications (Oct 06)
- Hardware: x86_64 and Raspberry Pi (aarch64) — produce production images for both.
- TPM/locked‑mode reachability: out of scope for this milestone (may be validated later).
- Install‑to‑disk wizard: out of scope.
- Transactional updates: in scope at minimum for read status and apply/reboot plumbing on production images.
- TLS issuance: in scope for portal and for all HTTP app listeners. Behavior is governed by the app spec at `src/l1/piccolod/docs/app-platform/specification.yaml`:
  - `listeners.flow: tcp` with `protocol: http` → device‑terminated TLS at the Piccolo proxy (certs issued/renewed by the device).
  - `listeners.flow: tls` or `protocol: raw` → passthrough; app/container terminates TLS (no device termination).

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
- Single-disk VolumeManager: control/app volumes mount through an encrypted backend (e.g., gocryptfs) rooted at `PICCOLO_STATE_DIR`; AttachOptions enforce RO/RW.
- Leadership wiring:
  - Kernel leader required for management ops (install/stop/etc.).
  - App follower → stop only that app’s container; app leader → run locally.
- Router (stub):
  - `RegisterKernelRoute(mode=local|tunnel, leaderAddr)`; `RegisterAppRoute(app, mode, leaderAddr)`.
  - On Nexus inbound streams, choose local (this milestone) or tunnel (future device hop) based on leadership.
- Nexus integration:
  - Adapter around the real `nexus-proxy-backend-client` supervised by runtime; register portal + app listeners; `OnStream` feeds Router decisions.
- API behaviour:
  - Kernel write endpoints operate locally when leader; on followers, requests arriving through Nexus would be forwarded (hook in place).
- Health:
  - Standby role (follower) reported as OK with role annotation; lock/unlock reflected.
  - Fatal persistence/crypto failures flip readiness to HTTP 503 so MicroOS rollback hooks can trigger.

## Out of Scope (kept as TODOs)
- Cross-device tunnels for app/kernel traffic (router supports the mode but not the transport here).
- Nexus enrollment flows beyond device secret/connect; DNS automation; wildcard/DNS‑01 issuance.
- Block-replication fences; eventual consistency is acceptable with local `rev` verification.
- Streaming PCV export pipeline (streaming writer, integrity bundle upload) — deferred to a follow-on milestone.

## Acceptance Criteria
1. First boot → admin setup → unlock → `/health/ready` reports ready and components OK.
2. Configure remote → Nexus client connects (WSS) and registers listeners; remote status reflects config.
3. Install a sample app → service ports allocated → local HTTP proxy serves it.
4. Remote access: inbound stream for the app proxies to the local app via Router.
5. Simulated app follower event: local app stops; Router switches route to tunnel (log-only in single-node); no local serving.
6. Auth/remote changes bump control-store `rev`; follower poller (if simulated) emits commit events; managers remain consistent.
7. Inject simulated fatal persistence/crypto error → `/health/ready` flips to HTTP 503 with component status explaining the fault.
8. ACME HTTP‑01 issuance succeeds via Nexus path for portal and all HTTP app listeners; device installs and serves issued certs; inventory and manual renewal work; renewals are scheduled.
9. Transactional‑update: production images surface read‑only OS update status; apply + reboot path is callable (success can be simulated in CI).

## Test Plan (manual or automated)
- Unit tests: persistence `rev` bump; AppManager leader/follower reactions; Router stream hand-off (with fake Nexus client).
- Integration (dev env): 
  - Start piccolod; call `/api/v1/crypto/setup` then `/unlock`.
  - Call `/api/v1/remote/configure` with Endpoint + DeviceSecret.
  - POST `/api/v1/apps` with a simple HTTP app; confirm `/remote/status` and local proxy.
  - Simulate `LeadershipRoleChanged{app:demo,follower}` on the bus; confirm app stopped and router logs “tunnel”.

Automated E2E (CI)
- Nexus + Pebble + piccolod in containers (no QEMU boots).
- DNS/host mapping inside CI to resolve portal/listener hostnames to Nexus.
- Validate HTTP‑01 challenge flow and remote HTTPS to app via device‑terminated TLS.

## Implementation Checklist
- [ ] Add `cluster.ResourceKernel`; refactor kernel checks.
- [ ] Router stub + leadership hook; connect Nexus client `OnStream` to Router.
- [ ] VolumeManager single-disk encrypted mount; enforce RO/RW per role.
- [ ] Control-store `rev` + checksum; follower poller; `TopicControlStoreCommit`.
- [ ] Gate control-store writes on kernel leader; app volume RW on app leader.
- [ ] Remote Manager ↔ Nexus client adapter and supervisor component.
- [ ] Documentation and AGENTS.md backlinks.

## Implementation Checklist — P0 Addendum (Oct 08)
Runtime & Health
- [ ] Readiness 503 on fatal persistence/crypto states; wire health transitions and unit tests.
- [ ] Leader‑hint behavior on write endpoints when kernel role=follower (scaffold now; single‑node stays leader). Unit tests.
- [ ] Verify follower event → app stop + Router mode=tunnel; unit tests assert stop and log.

Remote, Router & TLS
- [ ] Nexus adapter registers portal + per‑listener hostnames; hot‑updates on app add/remove.
- [ ] ACME (lego) HTTP‑01: challenge handler reachable at `/.well-known/acme-challenge/*` over remote port 80.
- [ ] Encrypted storage for keys/certs; expose inventory via existing endpoints; manual renew path functional.
- [ ] Use issued certs to terminate TLS for HTTP listeners (flow=tcp) on remote flows; passthrough for `flow: tls`.
- [ ] Simple renewal scheduler with backoff; unit tests with Pebble.

L0 Images
- [ ] Ensure packages (lego, fuse3, gocryptfs) and piccolod service in KIWI for x86_64 and RPi prod images.
- [ ] Produce prod images for both arches; save build logs; basic service start validation.

Automated E2E (CI)
- [ ] Compose stack: Nexus Proxy Server + Pebble ACME + piccolod.
- [ ] Test: setup → unlock → remote configure (Pebble) → install sample HTTP app → confirm local proxy → confirm remote HTTPS with Pebble cert → simulate follower event and assert tunnel decision.

## References
- Runtime roadmap: `src/l1/piccolod/docs/runtime/runtime-architecture-roadmap.md`.
- Kernel/app leadership design: `src/l1/piccolod/docs/runtime/kernel-and-leadership-design.md`.
- Persistence design: `src/l1/piccolod/docs/persistence/persistence-module-design.md`.
 - App spec (TLS flow policy): `src/l1/piccolod/docs/app-platform/specification.yaml`.
