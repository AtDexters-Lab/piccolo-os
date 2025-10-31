# Control Store SQLite Migration Plan

_Last updated: October 30, 2025_

## Goals

* Replace the encrypted JSON snapshot (`control.enc`) with an encrypted SQLite database on the control volume.
* Preserve the existing repository interfaces (`AuthRepo`, `RemoteRepo`, `AppStateRepo`) so higher layers remain unchanged.
* Ensure every control-store write remains transactional, crash-safe, and fully encrypted using the existing SDEK + gocryptfs volume.
* Make future schema changes predictable via forward-only migrations.

## Requirements & Constraints

* **Encryption:** The control volume remains gocryptfs-backed. The SQLite database (`control.db`) lives under the mounted volume (e.g. `<mount>/control.db`) and is therefore encrypted at rest. We continue to gate access on unlock (SDEK required).
* **Durability:** SQLite runs in WAL mode with `PRAGMA synchronous=FULL` to guarantee atomic commits even under sudden power loss.
* **No back-compat:** There are no deployed devices yet, so we can initialize fresh databases and drop the legacy JSON path once the swap lands.
* **Schema evolution:** Ship versioned migrations and bump `PRAGMA user_version` per release; no ad-hoc DDL.
* **Repository surface:** Keep Go interfaces stable. Introducing SQL should be invisible to the rest of piccolod.

## Proposed Schema

| Table | Purpose | Columns | Notes |
|-------|---------|---------|-------|
| `meta` | Single-row metadata and monotonic revision | `id INTEGER PRIMARY KEY CHECK (id=1)`, `revision INTEGER NOT NULL`, `checksum TEXT NOT NULL`, `updated_at TIMESTAMPTZ NOT NULL` | Revision increments with every write. `checksum` keeps the existing SHA-256 behaviour. |
| `auth_state` | Portal authentication state | `id INTEGER PRIMARY KEY CHECK (id=1)`, `initialized BOOLEAN NOT NULL`, `password_hash TEXT`, `updated_at TIMESTAMPTZ NOT NULL` | Only one row. Matches `AuthRepo` expectations. |
| `remote_config` | Remote manager payloads | `id INTEGER PRIMARY KEY CHECK (id=1)`, `payload BLOB NOT NULL`, `updated_at TIMESTAMPTZ NOT NULL` | Stored as opaque JSON blob from the remote manager. |
| `apps` | App catalog state for `AppStateRepo` | `name TEXT PRIMARY KEY`, `payload BLOB NOT NULL`, `updated_at TIMESTAMPTZ NOT NULL` | `payload` keeps the marshalled `AppRecord`. |
| `events` *(optional future)* | Append-only control events for audit/logging | `id INTEGER PRIMARY KEY AUTOINCREMENT`, `type TEXT`, `payload BLOB`, `created_at TIMESTAMPTZ NOT NULL` | Not needed immediately but worth reserving. |

### Indices

* Primary keys cover lookups (`auth_state`, `remote_config`, `meta`).
* `apps` already keyed by `name`. Add `CREATE INDEX apps_updated_at_idx ON apps(updated_at);` if we need chronological scans later.

### Migrations

* Ship SQL migration files in `internal/persistence/migrations/`. Example:
  * `0001_init.sql`: create tables, set user_version=1.
* Startup sequence:
  1. Open database under the control volume.
  2. `PRAGMA journal_mode=WAL;`
  3. `PRAGMA synchronous=FULL;`
  4. Run pending migrations inside a transaction (`BEGIN; ...; COMMIT;`).
  5. Verify invariant rows exist (`meta`, `auth_state`, etc.).

## Repository Layer Changes

* `encryptedControlStore` becomes a thin wrapper around a SQLite connection guarded by the same mutex.
* Replace JSON marshal/unmarshal with SQL queries:
  * `AuthRepo.SetInitialized` → `UPDATE auth_state SET initialized=1, updated_at=?`.
  * `AuthRepo.SavePasswordHash` → `UPDATE auth_state SET password_hash=?, updated_at=?`.
  * `RemoteRepo.SaveConfig` → `INSERT INTO remote_config ... ON CONFLICT(id) DO UPDATE`.
  * `AppStateRepo.UpsertApp` → `INSERT ... ON CONFLICT(name) DO UPDATE`.
* Bump revision & checksum inside the same transaction:
  1. Perform repo-specific mutation.
  2. Compute checksum (SHA-256 of logical payload snapshot, same as today).
  3. `UPDATE meta SET revision=revision+1, checksum=?, updated_at=?`.

## Operational Considerations

* **Backups:** Before applying migrations, copy `<mount>/control.db` to `<mount>/control.db.bak`. Clean old backups on success.
* **Integrity checks:** Add `persistence.Service` helper to run `PRAGMA integrity_check;` and emit an event if it fails. Expose through `/api/v1/remote/status`.
* **Exports:** Control-plane exports now bundle `control.db` and any WAL/SHM files into a tarball instead of relying on the legacy `control.enc` snapshot.

## Implementation Roadmap

1. Land this schema plan (doc).
2. Implement the SQLite-backed control store as the default path (done).
3. Add automated tests covering migrations, crash recovery, and repo behaviours (ongoing).
4. Expand schema as new auth/authz features arrive (roles, policy tables, audit logs), following the migration discipline above.
