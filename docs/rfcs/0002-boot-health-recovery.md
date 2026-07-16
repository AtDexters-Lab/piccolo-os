# RFC: Bounded Boot-Health Recovery for Piccolo Appliances

Status: Implemented and validated — RFC-to-code closure completed 2026-07-16

## Scope

**Problem:** A repeated MicroOS boot-health failure can leave a powered-on
Piccolo appliance indefinitely inaccessible because health-checker enters
emergency mode after stopping the appliance's only management plane.

**In scope:** Piccolo OS support-package containment of failed boot validation;
a non-destructive Piccolo health-check stop hook; a PID-1-owned bounded reboot
fallback for failed health-checker completion; compatibility with the
existing MicroOS snapshot, rollback, and marker behavior; package, image, and
MicroOS VM validation of the resulting recovery paths using OS-owned,
deterministic health responses.

**Out of scope:** The `piccolod` multi-NIC observation fix; the definition and
implementation of `piccolod` local-control-plane readiness; general network
supervision; replacement of the upstream MicroOS rollback engine; a new
networked recovery target or recovery UI; changing systemd's generic
`emergency.target` semantics; retrofitting the recovery policy into rollback
snapshots created before this support-package version.

## Context and Evidence

The 2026-07-15 incident occurred on openSUSE MicroOS `20260712` with
`piccolod v0.2.34`, `piccolo-os-support 0.3.12`, health-checker
`1.13+git20260414.bb3e4ad`, and systemd `260.3`. The checksum-verified incident
bundle is retained in the sibling `piccolod` repository at
`artifacts/incidents/20260715-network-health-emergency/`.

The evidence establishes the following sequence:

1. One Ethernet interface was disconnected while another obtained an address,
   NetworkManager reached `CONNECTED_GLOBAL`, STUN succeeded, and the remote
   relay connected.
2. `piccolod` nevertheless returned HTTP 503 from
   `/api/v1/health/ready`; the Piccolo plugin exhausted 15 attempts over 30
   seconds and failed boot validation.
3. On the first failed boot, upstream health-checker wrote
   `/var/lib/misc/health-check.rebooted`, requested a reboot, and exited with
   status 1.
4. With that marker present on later boots, upstream health-checker invoked all
   plugin `stop` hooks and started `emergency.target`. The Piccolo hook
   explicitly stopped `piccolod`; the emergency transition stopped
   NetworkManager and terminated health-checker with SIGTERM.
5. PID 1 remained healthy, so the service and hardware watchdogs could not
   recover the intentionally stopped management plane.

The installed health-checker also contains other terminal or incomplete
paths: a missing health state file stops plugin-owned services and returns;
rollback failures exit; and reboot requests are not followed by an independent
fallback. All of these paths eventually leave `health-checker.service` failed.
The retained journal confirms both failure forms seen in the incident:
`exit-code` after the first reboot request and `signal` during the emergency
transition.

## Goals

1. No failed boot-validation path may settle indefinitely with the appliance
   powered on and `piccolod` or networking stopped.
2. Preserve the upstream MicroOS choice of snapshot, rollback behavior, and
   successful-boot bookkeeping.
3. Ensure recovery remains autonomous when a normal reboot request, rollback,
   state lookup, or emergency transition does not complete the recovery.
4. Keep the Piccolo health plugin from making management-plane availability
   worse while health-checker decides what to do.
5. Validate the policy in the packaged files, the final image artifact, and a
   real MicroOS VM across more than one boot.
6. Complete Piccolo OS implementation and acceptance without consuming an
   unmerged `piccolod` change or waiting for a `piccolod` release.

## Non-Goals

1. Reimplementing health-checker's snapshot or boot-loader state machine in a
   Piccolo-owned script.
2. Guaranteeing remote or LAN reachability when the local control plane itself
   cannot start.
3. Using external connectivity as a Piccolo OS boot-success criterion.
4. Providing an operator shell after repeated failures. For an appliance with
   no independent management plane, autonomous retry is safer than an
   inaccessible shell.
5. Preserving `health-checker-reboot=disabled` as a way to enter a terminal
   emergency shell. Recovery media remains the maintenance path when
   autonomous reboot must be bypassed.

## Required Invariants

- A failed Piccolo readiness check remains a real boot-health failure; the OS
  does not silently convert HTTP 503 into success.
- The OS plugin contacts only the loopback readiness endpoint. It performs no
  direct external-connectivity or per-interface check. The production meaning
  of that endpoint is owned by `piccolod`, but it is not a dependency of the OS
  recovery test suite.
- The Piccolo plugin's `stop` action never stops `piccolod` or networking.
- Upstream health-checker remains authoritative for snapshot and marker
  changes. Piccolo OS introduces no second persistent recovery counter.
- Health-checker cannot remain indefinitely in its start transaction; the
  service has an explicit five-minute start deadline.
- Any failed boot-health completion of `health-checker.service`, whether by
  non-zero exit or signal, causes PID 1 to initiate a reboot unless the system
  is already shutting down. The reboot transaction has its own forced-reboot
  timeout.
- The persistent `health-check.rebooted` marker may select the upstream
  emergency branch, but it cannot make that branch terminal. A later successful
  health check retains the upstream behavior of clearing the marker.
- These invariants apply only while the selected root snapshot contains this
  support-package policy. A rollback into a pre-policy snapshot restores that
  snapshot's older behavior.
- The manager fallback remains active when upstream
  `health-checker-reboot=disabled` suppresses its own reboot decision. Piccolo
  does not expose the resulting emergency shell as a usable management plane.

## Proposed Design

### 1. Make the Piccolo stop hook non-destructive

`piccolo-health-check.sh stop` becomes a documented no-op. Health-checker may
still invoke it for compatibility, but the hook will not explicitly stop
`piccolod.service`.

This removes the direct self-disable mechanism without altering the readiness
check, its retry window, or upstream plugin discovery. Other upstream plugin
stop hooks are not changed by this RFC.

### 2. Add a manager-owned failure action

The existing `health-checker.service` drop-in gains
`FailureAction=reboot` in its `[Unit]` section. This makes PID 1, rather
than the checker script or a service that emergency mode can stop, the owner of
the last-resort recovery action.

The same drop-in sets `TimeoutStartSec=5min`. This bounds a checker or plugin
that hangs before producing a success or failure result. Exceeding the deadline
makes the unit fail and enters the same manager-owned recovery path.

The non-force action is deliberate. In systemd v260, it declines a new reboot
request when `shutdown.target` is already active, so an intentional poweroff or
shutdown remains authoritative. When no shutdown is underway, it starts
`reboot.target` directly from PID 1. That target has
`JobTimeoutSec=30min` and `JobTimeoutAction=reboot-force`, giving a bounded
forced fallback even on hardware without a watchdog. `reboot-force` avoids
dirty filesystems; `reboot-immediate`, which can lose data, is never used.

The upstream checker still executes first and keeps its current decisions:

| Upstream outcome | Existing upstream effect | Piccolo containment |
| --- | --- | --- |
| Health check passes | Save/bless working state; clear stale marker where applicable | No failure action |
| New/default snapshot needs retry or rollback | Select the upstream snapshot and request reboot | Existing shutdown proceeds; the failure action does not replace it |
| First failure of the stable/default snapshot | Write `health-check.rebooted` and request reboot | Existing shutdown proceeds; the failure action does not replace it |
| Marker already exists | Invoke stop hooks and start emergency mode | Piccolo hook is harmless; failed checker completion starts `reboot.target` |
| State is missing, rollback fails, or reboot request returns without recovery | Checker exits unsuccessfully | Failed checker completion starts `reboot.target` |
| Checker or plugin hangs | The oneshot start remains incomplete | The five-minute start deadline fails the unit and starts `reboot.target` |
| Checker is killed during the emergency transition | Unit enters failed state with a signal result | Failed checker completion starts `reboot.target` |
| Checker is killed by an intentional shutdown or poweroff | Unit may enter failed state while `shutdown.target` is active | The failure action is suppressed; the existing power action wins |
| Upstream reboot is disabled by kernel command line | Upstream selects its emergency path rather than requesting reboot | Failed checker completion still starts `reboot.target` under Piccolo policy |

The fallback may race with a normal reboot already requested by health-checker.
Both actions have the same terminal intent, and systemd keeps the active
shutdown transaction. If no reboot transaction became active, the failed unit
causes PID 1 to start one. A stalled reboot ultimately reaches the target's
forced timeout.

### Independent validation boundary

Piccolo OS does not wait for the `piccolod` readiness or multi-NIC fixes. Its VM
suite replaces the loopback readiness source with an OS-owned deterministic
fixture that can return HTTP 200, return HTTP 503 repeatedly, or remain
unavailable across reboots. The harness independently controls external
connectivity for the offline-boot scenario.

This proves every behavior owned by this repository: what the plugin does with
each local response, how health-checker handles repeated failure, and whether
the machine becomes reachable or autonomously starts another boot. It does not
claim to prove how `piccolod` classifies multiple interfaces. The support
package keeps its existing unversioned runtime dependency, but no particular
`piccolod` branch, fix, build, or release is an input to OS acceptance.

The VM validator accepts the final VDI artifact, creates a disposable working
copy, and injects test-only units into that copy before boot. A loopback proxy
fixture controls only health-checker's HTTP requests through a test-only
service environment, leaving the production URL and installed plugin unchanged.
The fixture persists a response schedule across boots so one scenario can move
from repeated 503 to 200 without guest interaction.

A test-only observer records boot IDs, health-checker results, marker state, and
relevant service state in the disposable disk's `/var`. After the scenario
deadline, the host powers off the VM, mounts the working disk, and evaluates
that evidence together with the journal. This keeps assertions available even
when guest networking and the Piccolo API are unavailable. The pristine input
artifact is never modified, and fixture injection is not part of the shipped
image.

### 3. Keep existing recovery state

No Piccolo-owned durable recovery state is added. In particular, package
installation or upgrade does not delete `health-check.rebooted`: `/var` is
shared across MicroOS snapshots, and deleting the marker outside a successful
health check would erase upstream evidence without proving the boot healthy.

If the failure was transient, the next successful check clears the marker via
the existing upstream implementation. If the failure persists and upstream
cannot find a working snapshot, the appliance continues bounded boot attempts
instead of remaining powered on in an inaccessible emergency state.

This RFC deliberately chooses a reboot loop over a terminal emergency shell as
the final failure mode. It does not add a maximum retry count because the only
available terminal mode would be unreachable through the appliance's sole
management plane.

## Temporal Composition

The upstream checker owns snapshot choice and health state; the Piccolo plugin
owns only the local readiness observation; PID 1 owns the last-resort reboot.
The following transitions must remain valid even when interrupted or replayed:

| Event | Current state and durable record | Next state / effect | Retry and cleanup rule |
| --- | --- | --- | --- |
| Activation | Selected snapshot is booting; marker may be absent or present | Run all checks after `piccolod.service` ordering | Re-execution on a later boot reads the same upstream state |
| Successful check | Local control plane returns HTTP 200 | Upstream records/blesses success and removes stale failure state | Success is the only event that clears `health-check.rebooted` |
| Failed check before recovery choice | One or more plugins failed; snapshot and marker are unchanged | Upstream chooses retry, rollback, first reboot, or emergency | Any non-zero checker completion activates the manager fallback |
| Marker write, then interruption | `health-check.rebooted` exists but the requested reboot may not complete | PID 1 ensures `reboot.target` is active after checker failure | Marker is replay-safe and remains until a healthy boot |
| Snapshot selection, then interruption | Upstream may have changed default/oneshot boot state | PID 1 ensures reboot into the state selected upstream | Piccolo code does not compensate or choose a different snapshot |
| Emergency transition | Marker already exists; stop hooks and target transaction begin | Piccolo stop hook has no side effect; checker becomes failed; PID 1 starts `reboot.target` | Emergency mode must not remain the stable state |
| Missing/corrupt prerequisite or rollback failure | Upstream operation cannot complete and checker exits non-zero | PID 1 starts `reboot.target` | Preserve upstream files for the next attempt and for diagnostics |
| Checker or plugin hangs | The start transaction exceeds its explicit deadline | PID 1 starts `reboot.target` after timeout | No plugin owns an unbounded boot-health lifetime |
| Concurrent normal reboot and failure action | Upstream reboot transaction is already active when the unit fails | The failure action detects active shutdown and does not replace it | The existing reboot proceeds; its target timeout remains the forced fallback |
| Intentional poweroff overlaps checker cancellation | `shutdown.target` is active and the checker exits by signal | The failure action is suppressed | The requested power action remains authoritative |
| Upstream reboot suppression | The checker enters emergency handling because `health-checker-reboot=disabled` is present | The failed unit starts `reboot.target` | Recovery media, not the inaccessible emergency shell, is the explicit bypass path |
| Package or snapshot supersession | A different snapshot becomes selected | If it contains this policy, its installed drop-in owns recovery; a pre-policy snapshot retains its older behavior | Final-image validation proves the tested root contains both policy files; no attempt is made to mutate older snapshots |

Pause and resume are not separate runtime states: a boot-health decision is a
oneshot transaction. Cancellation or service stop outside shutdown is treated
as abnormal completion. Cancellation caused by an already-active shutdown is
not recovery work; the existing shutdown remains authoritative.

## Alternatives Considered

### Replace or wrap upstream health-checker

A Piccolo-owned state machine could bypass emergency mode and control rollback
directly. It would duplicate legacy Btrfs, BLS, boot-counting, and marker logic
that evolves with MicroOS. This creates more authority and more failure modes
than needed for the scoped invariant.

### Add a networked recovery target

A Piccolo recovery target could keep NetworkManager and `piccolod` running
after retry exhaustion. Upstream health-checker currently hard-codes
`emergency.target`, so this requires replacing the checker or changing generic
emergency semantics. It also introduces a new boot lifecycle and operator
surface. Defer this unless product requirements later demand a stable,
network-reachable degraded mode rather than autonomous retries.

### Treat readiness failures as success in the OS plugin

Checking only process liveness or accepting HTTP 503 would avoid this incident
but would also bless images where the local control plane is genuinely broken.
The correct boundary is to fix `piccolod` readiness semantics while retaining a
strict OS check of that contract.

### Rely on `Restart=always` or the watchdog

Neither recovers a service explicitly stopped by `systemctl stop`, and PID 1
continues feeding the hardware watchdog in emergency mode. These mechanisms do
not cover the observed failure.

## Implementation Sites

| Site | Planned responsibility |
| --- | --- |
| `packages/piccolo-os-support/piccolo-health-check.sh` | Make `stop` non-destructive and document the recovery ownership boundary |
| `packages/piccolo-os-support/health-checker-piccolo.conf` | Preserve ordering and add the manager-owned failure action |
| Upstream `health-checker.service` and `reboot.target` in the selected image | Remain unmodified composition points; image validation proves the oneshot and bounded reboot assumptions used by the drop-in |
| `packages/piccolo-os-support/piccolo-os-support.spec` | Bump the policy package, record the change, and add build-time assertions for the plugin and drop-in |
| `scripts/validate-image-policy.sh` | Assert the installed plugin cannot explicitly stop `piccolod` and the installed drop-in contains the required failure action |
| `scripts/validate-obs-image.sh` | Reuse unchanged to apply the policy validator to the final OBS image rather than a staging tree |
| `scripts/validate-boot-health-vm.sh` (new) | Create and inject a disposable VDI, orchestrate deterministic guest-local health responses, observe multi-boot results from persistent disk evidence, and clean up the VM and mounts |

## Validation Plan

### Package and static policy

- Run shell syntax validation on the health plugin.
- Exercise `check` against HTTP 200, repeated HTTP 503, and an unavailable local
  endpoint; retain strict success-only-on-200 behavior.
- Exercise `stop` and prove it does not invoke `systemctl` or change
  `piccolod.service` state.
- Use systemd unit verification against the target unit/drop-in combination and
  assert `FailureAction=reboot` is effective and the target image retains
  `reboot.target`'s bounded forced action.
- Assert the effective health-checker start deadline is five minutes.
- Extend the RPM `%check` section with semantic assertions for the no-op hook
  and failure action.

### Final image policy

- Extend `validate-image-policy.sh` to verify exact installed source files,
  executable mode, effective recovery directive, absence of an explicit
  Piccolo stop action, health-checker enablement, and NetworkManager enablement.
- Run `validate-obs-image.sh` against the compressed final OBS artifact so the
  test crosses decompression, conversion, partition selection, and mounted
  read-only root inspection.

### MicroOS VM scenarios

Run against an image containing the exact packaged change. Recovery assertions
must be observed from the host or a persistent/serial evidence channel; the
test must not depend solely on SSH or the Piccolo HTTP API remaining available.

1. Force `/api/v1/health/ready` to return HTTP 503 for every retry and prove a
   new boot begins within a bounded deadline.
2. Start without `health-check.rebooted`; prove the first failure writes the
   marker and reboots, then prove the second failed boot also reboots rather
   than remaining in emergency mode.
3. Start with `health-check.rebooted` already present and prove the system does
   not remain on the same boot with `piccolod` and NetworkManager stopped.
4. Exercise the missing-state and failed-recovery seams, plus a deliberately
   hung test plugin, and prove non-zero or timed-out health-checker completion
   still causes a new boot.
5. Restore HTTP 200 after a failed boot and prove the appliance reaches the
   healthy boot state and the stale marker is cleared.
6. Disable external connectivity, have the OS-owned loopback fixture return
   HTTP 200, and prove boot validation succeeds. This demonstrates that the OS
   plugin itself has no external-connectivity prerequisite.
7. Request an intentional poweroff while health-checker is still active and
   prove the failure action does not convert it into a reboot.
8. Where the image supports the upstream reboot-suppression kernel option, boot
   with it enabled and prove Piccolo policy still leaves emergency mode through
   the manager-owned reboot path.

The invariant for every failed scenario is: either the appliance becomes
locally/remote reachable, or its boot identity changes again within the test
deadline. It must never remain indefinitely on one boot in emergency mode with
the management plane stopped.

Selecting one usable interface among multiple Ethernet interfaces is entirely
owned by `piccolod` and is not an OS acceptance scenario. OS recovery begins at
the resulting loopback health response.

## Rollout and Observability

1. Ship the change as a `piccolo-os-support` policy-package update and include
   it in a newly validated MicroOS image.
2. Canary the image on a VM and a representative appliance before fleet-wide
   release.
3. Preserve health-checker, snapshot, and boot journals across attempts so a
   reboot loop remains diagnosable from the disk or after recovery.
4. Treat repeated rebooting as a visible critical fault, but do not replace it
   with a terminal inaccessible state.

Existing appliances with `health-check.rebooted` require no migration. Once
they boot the updated snapshot, that marker can no longer strand the machine;
the next successful health check clears it normally. If upstream rollback
instead selects a snapshot created before this package version, the recovery
guarantee no longer applies until an updated snapshot boots again.

## Risks and Trade-offs

- **Recovery may take the normal shutdown path first:** The failure action uses
  `reboot` so intentional poweroff remains authoritative. A stuck reboot may
  take up to the target's 30-minute timeout before systemd escalates to
  `reboot-force`; `reboot-immediate` is not used.
- **Persistent fault becomes a reboot loop:** This is intentional for an
  appliance with no independent console or management plane. A reachable
  degraded target would be a larger product and architecture decision.
- **False readiness failure causes repeated rebooting:** The existing
  15-attempt Piccolo plugin retry loop remains. OS-owned VM responses cover
  restoration to HTTP 200 without depending on a `piccolod` change.
- **Systemd transaction behavior differs from expectation:** The target image
  uses systemd 260.3, but the effective directive and the signal-failure path
  must be proven in the real MicroOS VM before release.
- **Static validation cannot prove temporal recovery:** Image validators are
  necessary gates but do not replace the two-boot VM scenarios.
- **Rollback into a pre-policy snapshot restores the old unsafe behavior:** The
  recovery drop-in and non-destructive plugin are snapshot-local. This RFC does
  not mutate older snapshots or add a rollback-persistent bootstrap. The user
  explicitly accepted this initial-upgrade risk on 2026-07-16.
- **Upstream reboot suppression is intentionally overridden:**
  `health-checker-reboot=disabled` cannot provide a useful terminal recovery
  mode on this appliance. An operator who must suppress autonomous recovery
  uses external recovery media instead.

## Review and Delivery Gates

With this scope checkpoint accepted:

1. Run structured RFC review and adversarial temporal red-team review; revise
   until blocking findings converge.
2. Run subtractive RFC minimization and conditional final verification.
3. Implement only the accepted sites and validation contract.
4. Run implementation-quality and privileged-path security review, fix all
   blocking findings, and re-review.
5. Run dirty-tree gating review, package/static checks, final-image validation,
   MicroOS VM scenarios, and RFC-to-code closure before commit.

## Scope Checkpoint

Approval of this RFC accepts the following product-level failure policy:

> When MicroOS exhausts its normal boot-health decision and no healthy snapshot
> is available, Piccolo OS continues autonomous, bounded reboot attempts, with
> forced reboot as the timeout fallback. It does not settle into a local
> emergency shell that is inaccessible through the appliance's only management
> plane, provided the selected snapshot contains this recovery policy.
