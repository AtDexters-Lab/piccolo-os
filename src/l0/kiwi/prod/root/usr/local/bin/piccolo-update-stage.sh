#!/bin/bash
#
# Stage OS updates in the background using transactional-update (no reboot).
# - Safe to run repeatedly; uses flock to avoid concurrent runs.
# - Minimal logging suitable for production images.

set -euo pipefail

LOCK_FILE="/run/piccolo/update-stage.lock"
mkdir -p /run/piccolo

exec 9>"${LOCK_FILE}"
if ! flock -n 9; then
  echo "piccolo-update-stage: another run is in progress; exiting" >&2
  exit 0
fi

echo "piccolo-update-stage: starting background stage"

# Use non-interactive dup to stage new snapshot without reboot.
# Note: We deliberately omit any automatic reboot flags; reboot remains an explicit action.
if transactional-update -n dup; then
  echo "piccolo-update-stage: stage complete; reboot may be required"
  logger -t piccolo-update -p daemon.notice "Background stage complete; reboot required when user confirms"
  exit 0
else
  rc=$?
  echo "piccolo-update-stage: stage failed with RC=${rc}" >&2
  logger -t piccolo-update -p daemon.err "Background stage failed (rc=${rc}); will retry on next timer"
  exit ${rc}
fi

