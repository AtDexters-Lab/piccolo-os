#!/bin/bash
# piccolo-clock-epoch.sh — persist/restore system clock for RTC-less devices
#
# Devices without battery-backed RTCs (RPi, Rock64) boot with a stale clock.
# This script saves the current wall-clock time to disk periodically and at
# shutdown, then restores it at boot — before NTP starts — so the NTP step
# shrinks from days/weeks to seconds/minutes.
#
# Prior art: Debian/Ubuntu fake-hwclock, adapted for systemd on MicroOS.
#
# Subcommands: restore, save, status
# Error policy: restore/save always exit 0 (must never block boot/shutdown).
#               status exits non-zero on invalid data (diagnostic tool).

set -u
PATH=/usr/sbin:/usr/bin:/bin:/sbin
export PATH

EPOCH_FILE="/var/lib/piccolo/clock-epoch"
EPOCH_DIR="/var/lib/piccolo"

log() { echo "clock-epoch: $*" >&2; }

cmd_restore() {
    if [[ ! -f "$EPOCH_FILE" ]]; then
        log "no epoch file, skipping restore"
        return 0
    fi

    saved=$(cat "$EPOCH_FILE" 2>/dev/null) || { log "cannot read epoch file"; return 0; }

    # Validate: must be a positive integer > 1000000000 (Sep 2001)
    if ! [[ "$saved" =~ ^[0-9]+$ ]] || [[ "$saved" -le 1000000000 ]]; then
        log "warning: epoch file contains invalid data: '${saved:0:40}'"
        return 0
    fi

    now=$(date +%s)

    if [[ "$now" -ge "$saved" ]]; then
        log "clock already ahead (now=$now, saved=$saved), no adjustment needed"
        return 0
    fi

    if ! date -s "@${saved}" >/dev/null 2>&1; then
        log "failed to set clock to $saved"
        return 0
    fi
    log "restored clock from $now to $saved (advanced $(( saved - now ))s)"
}

cmd_save() {
    local tmp=""

    mkdir -p "$EPOCH_DIR" || { log "cannot create $EPOCH_DIR"; return 0; }

    now=$(date +%s)
    tmp=$(mktemp "${EPOCH_FILE}.XXXXXX") || { log "cannot create temp file"; return 0; }
    echo "$now" > "$tmp" || { rm -f "$tmp"; log "cannot write temp file"; return 0; }
    mv "$tmp" "$EPOCH_FILE" || { rm -f "$tmp"; log "cannot rename temp file"; return 0; }

    log "saved epoch $now"
}

cmd_status() {
    echo "Epoch file: $EPOCH_FILE"

    if [[ ! -f "$EPOCH_FILE" ]]; then
        echo "Status: no epoch file"
        return 0
    fi

    saved=$(cat "$EPOCH_FILE" 2>/dev/null) || { echo "Status: cannot read file"; return 1; }

    if ! [[ "$saved" =~ ^[0-9]+$ ]] || [[ "$saved" -le 1000000000 ]]; then
        echo "Status: invalid data in epoch file"
        return 1
    fi

    now=$(date +%s)
    delta=$(( now - saved ))

    echo "Saved epoch: $saved ($(date -d "@${saved}" '+%Y-%m-%d %H:%M:%S %Z' 2>/dev/null || echo 'unknown'))"
    echo "Current time: $now ($(date '+%Y-%m-%d %H:%M:%S %Z'))"
    echo "Delta: ${delta}s"
}

case "${1:-}" in
    restore) cmd_restore || { log "restore failed unexpectedly, continuing boot"; }; exit 0 ;;
    save)    cmd_save || { log "save failed unexpectedly"; }; exit 0 ;;
    status)  cmd_status; exit $? ;;
    *)
        echo "Usage: $0 {restore|save|status}" >&2
        exit 1
        ;;
esac
