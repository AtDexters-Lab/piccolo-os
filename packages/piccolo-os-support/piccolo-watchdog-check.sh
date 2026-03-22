#!/bin/bash
# piccolo-watchdog-check.sh — log which watchdog driver owns /dev/watchdog0
#
# Runs once at boot to give visibility into whether the correct hardware
# watchdog claimed the device.  Observability only — never blocks boot.

set -u
PATH=/usr/sbin:/usr/bin:/bin:/sbin
export PATH

TAG="piccolo-watchdog-check"
SYSFS="/sys/class/watchdog/watchdog0"

log() { logger -t "$TAG" "$@"; }

if [[ ! -d "$SYSFS" ]]; then
    log "WARNING: no watchdog0 found — no hardware watchdog on this platform"
    exit 0
fi

identity="unknown"
if [[ -r "${SYSFS}/identity" ]]; then
    identity=$(cat "${SYSFS}/identity" 2>/dev/null) || identity="unreadable"
fi

log "watchdog0 identity: ${identity}"

# Flag known-unreliable drivers so they stand out in log searches.
# If the modprobe blacklist is effective these should never appear.
case "$identity" in
    *intel_oc_wdt*|*"Intel OC WDT"*)
        log "WARNING: watchdog0 is claimed by intel_oc_wdt — this driver is unreliable for appliance use"
        ;;
    *softdog*|*"Software Watchdog"*)
        log "WARNING: watchdog0 is claimed by softdog — software watchdog cannot recover from kernel hangs"
        ;;
esac

exit 0
