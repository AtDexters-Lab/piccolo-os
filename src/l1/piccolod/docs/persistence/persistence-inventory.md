# Piccolod Persistence Inventory

_Last updated: 2025-10-31_

Piccolod anchors all on-disk state beneath `$PICCOLO_STATE_DIR`. The volume
manager provisions encrypted gocryptfs volumes (`ciphertext/<id>` as the sealed
view, `mounts/<id>` as the plaintext mount) and gates writes on the control
lock state. This document enumerates every path the daemon writes today, the
owning module, and the protection guarantees in place so we can migrate
remaining host-root stragglers into managed volumes.

## Host Root (`$PICCOLO_STATE_DIR`)

These entries live on the host filesystem. Data either remains encrypted (the
sealed gocryptfs payloads) or contains only operational metadata that is safe
to persist outside a volume.

| Path (relative) | Owner | Contents | Notes |
|-----------------|-------|----------|-------|
| `crypto/keyset.json` | `crypt.Manager` | Argon2id-sealed SDEK and KDF parameters | Temporarily left on host root to avoid introducing a TPM single point of failure until TPM-backed unlock is complete; file is encrypted but not volume-backed. |
| `ciphertext/control/**` | VolumeManager | Encrypted control volume payload (`gocryptfs.conf`, `piccolo.volume.json`, cipher blocks) | Never contains plaintext; exported as part of control/full PCV artifacts. |
| `ciphertext/bootstrap/**` | VolumeManager | Encrypted bootstrap volume payload and metadata | Same guarantees as control ciphertext; required before mounting bootstrap. |
| `volumes/<volume-id>/state.json` | VolumeManager | Desired/observed mount state, role, repair flag, timestamps | Operational journal used for reconciliation and eventing. Does not store application secrets. |
| `exports/control/control-plane.pcv` | ExportManager | JSON envelope with base64 tar of `ciphertext/control/**` | Generated on demand while persistence is re-locked; payload stays encrypted at rest. |
| `exports/full/full-data.pcv` | ExportManager | JSON envelope with base64 tar of both ciphertext volumes | Includes bootstrap and control ciphertext; same streaming pipeline as control-only export. |
| `mounts/<volume-id>/` | VolumeManager | FUSE mountpoints | Directories exist even when volumes are detached; invariants require them to be empty while locked. |

## Control Volume (`mounts/control`)

The control volume carries leader-only runtime state for the control plane. It
is mounted read/write on the elected leader and sealed for followers.

| Path | Owner | Contents | Notes |
|------|-------|----------|-------|
| `control.db`, `control.db-shm`, `control.db-wal` | `sqliteControlStore` | SQLite control store (auth state, remote config replica, app inventory, revision journal) | WAL mode with `synchronous=FULL`; read/write on the leader, read-only on followers (mutations return `ErrLocked`). |
| `apps/<app>/app.yaml` | `app.FilesystemStateManager` | Declarative app definition last applied | Treated as desired state; copied from API uploads. |
| `apps/<app>/metadata.json` | `app.FilesystemStateManager` | Runtime metadata (status, container id, timestamps, enable flag) | Drives supervisor decisions and UI. |
| `apps/<app>/app.prev.yaml` | `app.FilesystemStateManager` | Previous app definition for rollback | Written during upgrades before the new definition is committed. |
| `enabled/<app>` | `app.FilesystemStateManager` | Symlink signalling that an app should auto-start | Followers read this to decide which services to cold-start post failover. |
| `cache/**` | `app.FilesystemStateManager` | Implementation-defined cache (image manifests, rendered specs) | Non-authoritative; safe to purge if we ever roll fuller clean-up tooling. |

Future application volumes (VolumeClassApplication) will mount under
`mounts/<volume-id>` with data owned by the corresponding service; as of this
checkpoint, control metadata remains the only active consumer within the
control volume.

## Bootstrap Volume (`mounts/bootstrap`)

The bootstrap volume holds the minimal runtime required while the appliance is
locked (portal TLS, remote control configuration). It is backed by the same
SDEK as the control volume but mounts even when the control store is sealed.

| Path | Owner | Contents | Notes |
|------|-------|----------|-------|
| `remote/config.json` | `bootstrapRemoteStorage` / `remote.Manager` | Latest remote configuration JSON (mirrors control store when unlocked) | Serves UI/API before unlock; writes fail with HTTP 423 if the volume is unmounted. |
| `remote/certs/portal.{crt,key}` | `remote.Manager` + ACME issuer | ACME-issued TLS material for the portal origin | Exposed through `remote.FileCertProvider`; each issuance overwrites the pair atomically. |
| `remote/certs/*.crt|*.key|*.pem` | `remote.Manager` | Additional listener/alias certificates | Naming matches hostname or wildcard. |
| `remote/acme/account.{key,json}` | `remote/acme.Manager` | Lego account key and registration cache | Guarded by bootstrap mount; reset automatically if directory URL changes. |
| `remote/acme/**` | `remote/acme.Manager` | Order history, challenge cache, issuance logs | Required for renewals and retry heuristics. |

## Application Volumes

Per-application volumes (class `application`) are allocated by the volume
manager but are not yet wired into workload lifecycles. When enabled, each
volume will appear as:

- Ciphertext: `ciphertext/<volume-id>/**`
- Mountpoint: `mounts/<volume-id>/`
- State journal: `volumes/<volume-id>/state.json`

Application owners will receive a `VolumeHandle` so they can write exclusively
through the mounted path. Until that plumbing lands, application data continues
to rely on the control volume’s `apps/**` tree.

## Test Harness Considerations

Unit and integration tests that exercise persistence without FUSE set
`PICCOLO_ALLOW_UNMOUNTED_TESTS=1`. The flag explicitly opts into bypassing the
mount verifier and allows the sqlite control store to operate against temp
directories. Production builds never set this flag; it should remain a test-only
escape hatch.

## Next Steps

1. Transition the cryptography keyset onto a TPM-backed bootstrap path once the
   TPM unlock flow is complete.
2. Migrate remaining modules (auth legacy filesystem, remote manager caches,
   future application volumes) to consume `VolumeHandle`s directly instead of
   resolving paths ad hoc.
3. Extend the export/import surface with scheduling, eventing, and operator
   tooling now that both volumes stream encrypted ciphertext consistently.
