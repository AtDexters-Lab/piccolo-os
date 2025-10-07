#!/bin/bash
#
# Automatic rollback on fatal boot failure, gated by staged-snapshot sentinel.
# - Triggered by OnFailure= from the health-check service.
# - Rolls back to previous snapshot and reboots exactly once per staged generation.

set -euo pipefail

STATE_DIR="/var/lib/piccolo/os-update"
STATE_FILE="${STATE_DIR}/state"
LOCK_FILE="/run/piccolo/rollback.lock"
mkdir -p /run/piccolo "$STATE_DIR"

get_current_subvol() {
  local opts subvol id
  opts=$(findmnt -no FS-OPTIONS / 2>/dev/null || true)
  subvol=$(printf "%s" "$opts" | tr ',' '\n' | sed -n 's/^subvol=\(.*\)$/\1/p' | head -n1)
  if [ -z "$subvol" ]; then
    id=$(printf "%s" "$opts" | tr ',' '\n' | sed -n 's/^subvolid=\(.*\)$/\1/p' | head -n1)
    [ -n "$id" ] && subvol="subvolid=$id"
  fi
  echo "$subvol"
}

read_state() {
  PENDING_GEN=0
  ROLLED_BACK_GEN=0
  LAST_SUCCESS_GEN=0
  STAGED_FROM_SUBVOL=""
  LAST_SUCCESS_SUBVOL=""
  UPDATED_AT=""
  if [ -f "$STATE_FILE" ]; then
    # shellcheck disable=SC1090
    . "$STATE_FILE" || true
  fi
}

write_state() {
  UPDATED_AT=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
  cat > "$STATE_FILE" <<EOF
PENDING_GEN=${PENDING_GEN}
ROLLED_BACK_GEN=${ROLLED_BACK_GEN}
LAST_SUCCESS_GEN=${LAST_SUCCESS_GEN}
STAGED_FROM_SUBVOL=${STAGED_FROM_SUBVOL}
LAST_SUCCESS_SUBVOL=${LAST_SUCCESS_SUBVOL}
UPDATED_AT=${UPDATED_AT}
EOF
}

exec 9>"${LOCK_FILE}"
if ! flock -n 9; then
  echo "piccolo-rollback: another rollback run is in progress; exiting" >&2
  exit 0
fi

read_state
CUR_SUBVOL=$(get_current_subvol)

# Gate: only roll back if we have a pending staged boot and we are no longer on the subvol we staged from,
# and we have not already rolled back for this generation.
if [ "${PENDING_GEN}" -le 0 ] || [ -z "${STAGED_FROM_SUBVOL}" ] || [ "${CUR_SUBVOL}" = "${STAGED_FROM_SUBVOL}" ] || [ "${ROLLED_BACK_GEN}" -ge "${PENDING_GEN}" ]; then
  echo "piccolo-rollback: conditions not met (pending=${PENDING_GEN} cur=${CUR_SUBVOL} from=${STAGED_FROM_SUBVOL} rolled_back=${ROLLED_BACK_GEN}); skipping" >&2
  exit 0
fi

echo "piccolo-rollback: initiating automatic rollback (gen=${PENDING_GEN})" | tee /dev/kmsg || true
logger -t piccolo-rollback -p daemon.crit "Initiating automatic rollback due to failed health-check (gen=${PENDING_GEN})"

if transactional-update -n rollback; then
  # Mark rollback consumed for this generation and clear pending.
  ROLLED_BACK_GEN=${PENDING_GEN}
  PENDING_GEN=0
  STAGED_FROM_SUBVOL=""
  write_state
  echo "piccolo-rollback: rollback staged; rebooting now" | tee /dev/kmsg || true
  logger -t piccolo-rollback -p daemon.crit "Rollback staged (gen=${ROLLED_BACK_GEN}); rebooting"
  systemctl reboot
else
  rc=$?
  echo "piccolo-rollback: rollback failed (rc=${rc})" | tee /dev/kmsg || true
  logger -t piccolo-rollback -p daemon.alert "Rollback failed (rc=${rc})"
  exit ${rc}
fi
