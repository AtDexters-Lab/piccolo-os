#!/bin/bash
#
# Stage OS updates in the background using transactional-update (no reboot).
# - Uses a sentinel in /var/lib/piccolo/os-update/state to track a pending staged boot.
# - Safe to run repeatedly; uses flock to avoid concurrent runs.

set -euo pipefail

STATE_DIR="/var/lib/piccolo/os-update"
STATE_FILE="${STATE_DIR}/state"
LOCK_FILE="/run/piccolo/update-stage.lock"
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
  echo "piccolo-update-stage: another run is in progress; exiting" >&2
  exit 0
fi

echo "piccolo-update-stage: starting background stage"

# Stage new snapshot without reboot.
if transactional-update -n dup; then
  # Update sentinel: create a new pending generation and record source subvolume.
  read_state
  base_gen=$LAST_SUCCESS_GEN
  if [ "$ROLLED_BACK_GEN" -gt "$base_gen" ]; then base_gen=$ROLLED_BACK_GEN; fi
  PENDING_GEN=$((base_gen + 1))
  STAGED_FROM_SUBVOL=$(get_current_subvol)
  write_state
  echo "piccolo-update-stage: stage complete; reboot may be required"
  logger -t piccolo-update -p daemon.notice "Background stage complete; reboot required when user confirms"
  exit 0
else
  rc=$?
  echo "piccolo-update-stage: stage failed with RC=${rc}" >&2
  logger -t piccolo-update -p daemon.err "Background stage failed (rc=${rc}); will retry on next timer"
  exit ${rc}
fi
