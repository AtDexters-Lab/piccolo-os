#!/bin/bash
#
# Automatic rollback on fatal boot failure:
# - Invoked via OnFailure= from the health-check service.
# - Rolls back to previous snapshot and reboots immediately.
# - Uses flock to avoid repeated concurrent attempts.

set -euo pipefail

LOCK_FILE="/run/piccolo/rollback.lock"
mkdir -p /run/piccolo

exec 9>"${LOCK_FILE}"
if ! flock -n 9; then
  echo "piccolo-rollback: another rollback run is in progress; exiting" >&2
  exit 0
fi

echo "piccolo-rollback: initiating automatic rollback" | tee /dev/kmsg || true
logger -t piccolo-rollback -p daemon.crit "Initiating automatic rollback due to failed health-check"

# Stage rollback to previous snapshot; then reboot to activate it.
if transactional-update -n rollback; then
  echo "piccolo-rollback: rollback staged; rebooting now" | tee /dev/kmsg || true
  logger -t piccolo-rollback -p daemon.crit "Rollback staged; rebooting"
  systemctl reboot
else
  rc=$?
  echo "piccolo-rollback: rollback failed (rc=${rc})" | tee /dev/kmsg || true
  logger -t piccolo-rollback -p daemon.alert "Rollback failed (rc=${rc})"
  exit ${rc}
fi

