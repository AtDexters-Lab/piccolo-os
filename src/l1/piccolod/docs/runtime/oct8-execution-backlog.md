# Oct 8 Execution Backlog — Single-Node Remote Access Baseline

Last updated: 2025-10-06

Release target: 2025-10-08

This file tracks P0 tasks required to hit the milestone. Owners and PR links can be appended inline.

## Runtime & Health
- [ ] Readiness 503 on fatal persistence/crypto states; health transitions and unit tests. (Owner: Codex)
- [ ] Leader‑hint behavior on write endpoints when follower (scaffold + tests). (Owner: Codex)
- [ ] Follower event → app stop + router ModeTunnel; unit tests assert stop and log. (Owner: Codex) — partially implemented (stop+ModeTunnel); tests pending.

## Remote, Router & TLS
- [ ] Nexus adapter: register portal + per‑listener hosts; hot‑update on app add/remove. (Owner: Codex) — in progress (TLS mux + resolver routing landed; host array restart TODO).
- [ ] ACME HTTP‑01 (lego): challenge handler, encrypted key/cert storage, inventory + manual renew. (Owner: Codex)
- [ ] Proxy TLS: remote‑only device‑terminated TLS via single SNI mux (loopback) for HTTP listeners (flow=tcp); passthrough for `flow: tls`. (Owner: Codex) — in progress (TLS mux+mapping landed; certs pending).
- [ ] Renewal scheduler stub with backoff; Pebble unit/integration tests. (Owner: Codex)

## L0 Images
- [ ] Ensure packages (lego, fuse3, gocryptfs) and piccolod service for x86_64 and RPi prod. (Owner: Codex)
- [ ] Produce prod images; attach build logs; basic service start validation. (Owner: Codex)

## Automated E2E (CI)
- [ ] Compose: Nexus Proxy Server + Pebble + piccolod; local DNS mapping. (Owner: Codex)
- [ ] Flow: setup → unlock → remote configure (Pebble) → install sample HTTP app → remote HTTPS OK → follower event tunneling asserted. (Owner: Codex)

## Progress notes (Oct 06)
- Implemented loopback SNI TLS mux and resolver mapping (remote 443 → mux for flow=tcp; 80 → HTTP; flow=tls passthrough).
- Simplified Nexus adapter to let Dial fail (removed per‑port disable gate) to reduce moving parts.
- Wired ServiceManager lifecycle with publish hooks (no‑ops for now) to ease future adapter enhancements.
- Next: ACME manager + encrypted cert store; hook challenge handlers (portal + HTTP proxies); readiness 503 + leader hints; Nexus host array restart.

## Notes
- TLS termination policy follows `src/l1/piccolod/docs/app-platform/specification.yaml`.
- Pebble is used for automated issuance; Let’s Encrypt Staging is for optional manual verification.
