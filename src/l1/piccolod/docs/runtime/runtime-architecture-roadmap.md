# Piccolod Runtime Architecture Roadmap

_Last updated: 2025-10-01_

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
   - Command dispatcher (planned) provides structured, idempotent requests for cross-module actions (e.g., `CreateVolume`, `RunExport`, `PublishRoute`).  
   - Enables retries, logging, and swapping implementations (stub vs real) without leaking internals.

4. **Process supervision**  
   - Supervisor (planned) manages subsystem lifecycle: start, stop, restart policy, and health checks.  
   - Every long-lived module registers with the supervisor, simplifying shutdown and crash recovery.

5. **State machines & reconciliation**  
   - Each domain formalizes its states (e.g., persistence `locked → preunlock → unlocked`).  
   - Desired state comes from the control-plane DB; managers reconcile toward it rather than executing ad-hoc sequences.

6. **Job scheduling (future)**  
   - Background work (exports, parity rebuilds, cold-tier flushes) runs via a shared job runner with prioritization and telemetry hooks.

## Current Status (2025-10-01)
- Shared event bus and leadership registry extracted to `internal/events` and `internal/cluster`.  
- Command dispatcher skeleton (`internal/runtime/commands`) and supervisor (`internal/runtime/supervisor`) landed; server now instantiates both and registers mDNS/service manager components.  
- Stub consensus manager (`internal/consensus.Stub`) publishes leadership events, supervised alongside an observer that currently logs transitions. Persistence listens to those events to track control-plane role.  
- Persistence module consumes bus/registry via constructor options, emits placeholder lock-state events, registers command handlers, and still uses stubs for storage adapter.  
- Gin server bootstraps the shared bus/registry, supervisor, dispatcher, and passes them to persistence.  
- Persistence design checkpoint lives in `docs/persistence/persistence-module-design.md`.

## Near-term Tasks
1. **Supervisor skeleton**  
   - Introduce `internal/runtime/supervisor` controlling mDNS, service manager, persistence workers, etc.  
   - Refactor server startup to register subsystems with the supervisor.

2. **Command dispatcher**  
   - Define initial command structs for persistence operations and wire them through a dispatcher.  
   - Update callers (e.g., API handlers, app manager) to issue commands instead of invoking persistence internals directly.

3. **Consensus stub integration**  
   - Implement a basic consensus manager that emits leadership events (single-node stub initially).  
   - Ensure persistence and app manager log/respond to role changes to validate wiring.

4. **Module refactors**  
   - Pass shared bus/registry to app manager, service manager, remote manager.  
   - Subscribe to leadership/lock events and apply policy (stop/start containers, advertise routes).

5. **Documentation updates**  
   - Keep this roadmap and the persistence design doc in sync as modules adopt the new patterns.  
   - Add notes in `AGENTS.md` pointing to relevant design docs.

## Longer-term Milestones
- **Desired-state controllers:** move app deployment, remote configuration, and storage policies to reconciliation loops driven by control-plane state.  
- **Job runner:** central scheduler for exports, parity rebuild, telemetry sampling.  
- **Telemetry/health bus:** aggregate module health, feed UI and supervisor decisions.  
- **Policy engine:** centralize RBAC, quotas, and access control on API/command boundaries.

## References
- [Persistence module design checkpoint](../persistence/persistence-module-design.md)
- [Repository guidelines (AGENTS.md)](../../../../AGENTS.md) *(contains org-context and PRD pointers)*
