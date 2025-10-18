# Piccolod Runtime Architecture Roadmap

_Last updated: 2025-10-03_

## Vision
Piccolod is evolving from a single Go binary into a platform runtime akin to a mini operating system. Subsystems (persistence, app orchestration, networking, remote access, telemetry) should run semi-independently, communicate through shared infrastructure, and reconcile desired state without a monolithic procedural flow.

This document captures the architectural patterns we are adopting and the breadth-first plan to introduce them incrementally while current work (e.g., persistence) continues.

## Core Patterns

1. **Event-driven coordination**  
   - Shared event bus (`internal/events.Bus`) for asynchronous notifications: lock state, leadership changes, device events, export results, health signals.  
   - Modules subscribe to the topics they care about; no direct coupling to publishers.

2. **Leadership registry**  
   - Central cluster registry (`internal/cluster.Registry`) tracks `leader`/`follower` roles per resource (control plane, app volumes, services).  
   - Consensus layer owns updates; consumers (persistence, app manager, remote) react via events and registry queries. Warm/cold follower behavior remains policy in the consuming modules.

3. **Typed command/response channels**  
   - Command dispatcher (in progress) now brokers persistence exports and lock-state notifications; upcoming work adds volume orchestration and remote operations.  
   - Enables retries, logging, and swapping implementations (stub vs real) without leaking internals.

4. **Process supervision**  
   - Supervisor (planned) manages subsystem lifecycle: start, stop, restart policy, and health checks.  
   - Every long-lived module registers with the supervisor, simplifying shutdown and crash recovery.

5. **Unified state paths**  
   - `internal/state/paths` resolves all directories from `PICCOLO_STATE_DIR`, which is the bootstrap volume mount.  
   - Tests guard against hard-coded `/var/lib/piccolod` strings so durable data never bypasses volume manager policy.

6. **State machines & reconciliation**  
   - Each domain formalizes its states (e.g., persistence `locked → preunlock → unlocked`).  
   - Desired state comes from the control-plane DB; managers reconcile toward it rather than executing ad-hoc sequences.

7. **Job scheduling (future)**  
   - Background work (exports, parity rebuilds, cold-tier flushes) runs via a shared job runner with prioritization and telemetry hooks.

## Current Status (2025-10-03)
- Shared event bus and leadership registry extracted to `internal/events` and `internal/cluster`.  
- Command dispatcher skeleton (`internal/runtime/commands`) and supervisor (`internal/runtime/supervisor`) landed; server now instantiates both, registers mDNS/service manager components, and routes control exports plus lock-state transitions through the dispatcher.  
- Stub consensus manager (`internal/consensus.Stub`) publishes leadership events, supervised alongside an observer that currently logs transitions. Persistence listens to those events to track control-plane role.  
- Persistence module consumes bus/registry via constructor options, registers command handlers (volumes, exports, lock-state), emits placeholder export artifacts, and still uses stubs for the storage/volume backends. Control-store repos now persist the admin credential hash inside the encrypted dataset, auth HTTP endpoints surface `423 Locked` while the control store is sealed, and remote configuration writes are routed through the same encrypted repo so Nexus setup is gated until unlock.  
- Crypto unlock/lock APIs dispatch `persistence.record_lock_state` so lock transitions hit the event bus before other modules act.  
- Service manager and app manager attach to the shared bus—current wiring logs leadership and lock-state changes to validate propagation; policy hooks (warm vs cold, proxy gating) come next.  
- Gin server bootstraps the shared bus/registry, supervisor, dispatcher, and passes them to persistence.  
- Persistence design checkpoint lives in `docs/persistence/persistence-module-design.md`.
- Remote HTTPS enforcement now uses a dedicated loopback listener: the TLS mux terminates transport only and forwards to `127.0.0.1:<ephemeral secure port>` (HTTP-level handler that injects HSTS/redirects). The original plain HTTP listener still serves LAN traffic, keeping the mux transport-agnostic while guaranteeing secure semantics for remote hosts.

## Near-term Tasks
1. **Dispatcher coverage**  
   - Route volume ensure/attach callers (app lifecycle, future storage workers) through the command bus and add structured logging/metrics.  
   - Formalize responses and error handling around command dispatch.

2. **Event consumer behavior**  
   - Upgrade service/app manager handlers from logging to enforcement (stop follower containers, toggle proxies, reconcile warm vs cold modes).  
   - Gate API write paths on lock-state events instead of direct crypto checks.

3. **Export pipeline**  
   - Replace placeholder export artifacts with deterministic bundles on disk, add upload/download plumbing, and cover with integration tests.

4. **Persistence backends**  
   - Swap stubs for real storage/volume managers, including encrypted mount orchestration and device discovery.  
   - Connect control-store unlock to storage adapters and bootstrap rebuild workflows.

5. **Documentation cadence**  
   - Keep this roadmap and the persistence design doc in sync as modules adopt new behavior.  
   - Ensure AGENTS.md and runtime docs link to the latest persistence plan and command catalog.

## Longer-term Milestones
- **Desired-state controllers:** move app deployment, remote configuration, and storage policies to reconciliation loops driven by control-plane state.  
- **Job runner:** central scheduler for exports, parity rebuild, telemetry sampling.  
- **Telemetry/health bus:** aggregate module health, feed UI and supervisor decisions.  
- **Policy engine:** centralize RBAC, quotas, and access control on API/command boundaries.

## References
- [Persistence module design checkpoint](../persistence/persistence-module-design.md)
- [Repository guidelines (AGENTS.md)](../../../../AGENTS.md) *(contains org-context and PRD pointers)*
