#!/bin/bash
# piccolo-net-watchdog.sh — Network health watchdog for Piccolo OS
#
# Detects gateway unreachability via ARP and escalates:
#   1. 3 consecutive ARP failures → bounce interface (nmcli)
#   2. 3 more failures after bounce → reboot
#
# Activated by piccolo-net-watchdog.timer (every 30s).
# State in /run/piccolo/ (volatile) and /var/lib/piccolo/ (persistent).

set -u
PATH=/usr/sbin:/usr/bin:/bin:/sbin
export PATH

# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------
VOLATILE_DIR="/run/piccolo"
PERSISTENT_DIR="/var/lib/piccolo"
FAILURES_FILE="${VOLATILE_DIR}/net-watchdog-failures"
RECOVERIES_FILE="${VOLATILE_DIR}/net-watchdog-recoveries"
LAST_ACTION_FILE="${VOLATILE_DIR}/net-watchdog-last-action"
LOCK_FILE="${VOLATILE_DIR}/net-watchdog.lock"
REBOOTS_FILE="${PERSISTENT_DIR}/net-watchdog-reboots"
ONBOARDING_FILE="/piccolo-core/network-bootstrap/onboarding.json"

MAX_BOUNCES=3
BOUNCE_WINDOW=3600   # 1 hour
MAX_REBOOTS=1
REBOOT_WINDOW=7200   # 2 hours
FAILURE_THRESHOLD=3

TAG="piccolo-net-watchdog"

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------
log() {
    logger -t "$TAG" "$@"
}

prune_timestamps() {
    local file="$1" max_age="$2"
    local cutoff=$(( $(date +%s) - max_age ))
    if [ -f "$file" ]; then
        awk -v cutoff="$cutoff" '$1 >= cutoff' "$file" > "${file}.tmp"
        mv "${file}.tmp" "$file"
    fi
}

count_entries() {
    local file="$1"
    if [ -f "$file" ]; then
        wc -l < "$file"
    else
        echo 0
    fi
}

read_failures() {
    local val
    val=$(cat "$FAILURES_FILE" 2>/dev/null) || val=0
    [[ "$val" =~ ^[0-9]+$ ]] || val=0
    echo "$val"
}

write_failures() {
    echo "$1" > "$FAILURES_FILE"
}

read_last_action() {
    cat "$LAST_ACTION_FILE" 2>/dev/null || echo ""
}

write_last_action() {
    echo "$1" > "$LAST_ACTION_FILE"
}

# ---------------------------------------------------------------------------
# Precondition checks
# ---------------------------------------------------------------------------
for cmd in arping nmcli ping ip flock; do
    if ! command -v "$cmd" >/dev/null 2>&1; then
        log "ERROR: ${cmd} not found — cannot perform network health checks"
        exit 1
    fi
done

# ---------------------------------------------------------------------------
# Setup
# ---------------------------------------------------------------------------
mkdir -p "$VOLATILE_DIR" "$PERSISTENT_DIR" || { log "ERROR: cannot create state dirs"; exit 1; }

# Concurrency guard — skip if another instance is running
exec 9>"$LOCK_FILE"
flock -n 9 || exit 0

# ---------------------------------------------------------------------------
# Onboarding guard — skip during first-run setup
# ---------------------------------------------------------------------------
if [ -f "$ONBOARDING_FILE" ]; then
    onboarding_state=$(awk -F'"' '/"state"/{print $4}' "$ONBOARDING_FILE" 2>/dev/null || echo "")
    case "$onboarding_state" in
        pending|install_disk)
            exit 0
            ;;
        "")
            # Unparseable — exit silently (assume setup in progress)
            exit 0
            ;;
    esac
fi
# File missing = post-onboarding or pre-piccolod with no setup file yet — proceed

# ---------------------------------------------------------------------------
# Gateway extraction
# ---------------------------------------------------------------------------
default_route=$(ip -4 route show default 2>/dev/null | head -1)
if [ -z "$default_route" ]; then
    # No default route — check for recoverable WiFi radio soft-block
    wifi_radio=$(nmcli radio wifi 2>/dev/null)
    if [ "$wifi_radio" = "disabled" ]; then
        prune_timestamps "$RECOVERIES_FILE" "$BOUNCE_WINDOW"
        bounce_count=$(count_entries "$RECOVERIES_FILE")
        if [ "$bounce_count" -lt "$MAX_BOUNCES" ]; then
            log "no default route and WiFi radio disabled — re-enabling"
            nmcli radio wifi on
            date +%s >> "$RECOVERIES_FILE"
        else
            log "CRIT: WiFi radio disabled but bounce limit reached, giving up"
        fi
    fi
    write_failures 0
    write_last_action ""
    exit 0
fi

GATEWAY=$(echo "$default_route" | awk '/via/{for(i=1;i<=NF;i++) if($i=="via") print $(i+1)}')
IFACE=$(echo "$default_route" | awk '/dev/{for(i=1;i<=NF;i++) if($i=="dev") print $(i+1)}')

if [ -z "$GATEWAY" ] || [ -z "$IFACE" ]; then
    write_failures 0
    write_last_action ""
    exit 0
fi

# Validate extracted values (defense-in-depth — these come from kernel, but we run as root)
if ! [[ "$GATEWAY" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    log "ERROR: invalid gateway address: ${GATEWAY}"
    exit 1
fi
if ! [[ "$IFACE" =~ ^[a-zA-Z0-9._-]+$ ]]; then
    log "ERROR: invalid interface name: ${IFACE}"
    exit 1
fi

# ---------------------------------------------------------------------------
# ARP check
# ---------------------------------------------------------------------------
if arping -c 1 -w 3 -I "$IFACE" "$GATEWAY" >/dev/null 2>&1; then
    # Gateway reachable — reset state, exit silently
    write_failures 0
    write_last_action ""
    exit 0
fi

# ---------------------------------------------------------------------------
# Gateway unreachable — increment failure counter
# ---------------------------------------------------------------------------
failures=$(read_failures)
failures=$(( failures + 1 ))
write_failures "$failures"

if [ "$failures" -lt "$FAILURE_THRESHOLD" ]; then
    exit 0
fi

# ---------------------------------------------------------------------------
# 3 consecutive failures reached — check if this is a false positive
# ---------------------------------------------------------------------------
if ping -c 1 -W 3 8.8.8.8 >/dev/null 2>&1; then
    log "WARNING: ARP to gateway ${GATEWAY} via ${IFACE} failed but internet reachable — skipping recovery"
    write_failures 0
    write_last_action ""
    exit 0
fi

# ---------------------------------------------------------------------------
# Network is genuinely broken — determine escalation level
# ---------------------------------------------------------------------------
last_action=$(read_last_action)

if [ "$last_action" = "bounce" ]; then
    # ---------------------------------------------------------------------------
    # Bounce already tried — escalate to reboot
    # ---------------------------------------------------------------------------
    prune_timestamps "$REBOOTS_FILE" "$REBOOT_WINDOW"
    reboot_count=$(count_entries "$REBOOTS_FILE")

    if [ "$reboot_count" -ge "$MAX_REBOOTS" ]; then
        log "CRIT: reboot limit reached (${reboot_count}/${MAX_REBOOTS} in ${REBOOT_WINDOW}s window), giving up"
        exit 0
    fi

    date +%s >> "$REBOOTS_FILE"
    log "ARP to ${GATEWAY} via ${IFACE} failed after bounce — escalating to reboot"
    systemctl reboot
else
    # ---------------------------------------------------------------------------
    # First escalation — bounce the interface
    # ---------------------------------------------------------------------------
    prune_timestamps "$RECOVERIES_FILE" "$BOUNCE_WINDOW"
    bounce_count=$(count_entries "$RECOVERIES_FILE")

    if [ "$bounce_count" -ge "$MAX_BOUNCES" ]; then
        log "CRIT: bounce limit reached (${bounce_count}/${MAX_BOUNCES} in ${BOUNCE_WINDOW}s window), giving up"
        exit 0
    fi

    # Detect interface type
    IFACE_TYPE=$(nmcli -t -f DEVICE,TYPE device status 2>/dev/null | grep "^${IFACE}:" | cut -d: -f2)

    log "ARP to ${GATEWAY} via ${IFACE} failed (${FAILURE_THRESHOLD}/${FAILURE_THRESHOLD}), bouncing interface (type=${IFACE_TYPE:-unknown})"

    if [ "$IFACE_TYPE" = "wifi" ]; then
        trap 'nmcli radio wifi on 2>/dev/null' EXIT
        nmcli radio wifi off
        sleep 2
        nmcli radio wifi on
        trap - EXIT
    else
        # Intentional: expand IFACE at trap registration time, not when signalled
        # shellcheck disable=SC2064
        trap "nmcli device connect '${IFACE}' 2>/dev/null" EXIT
        nmcli device disconnect "$IFACE" 2>/dev/null
        sleep 2
        nmcli device connect "$IFACE" 2>/dev/null
        trap - EXIT
    fi

    date +%s >> "$RECOVERIES_FILE"

    # Post-bounce verification (10s allows for DHCP renewal on slower networks)
    sleep 10
    if arping -c 1 -w 3 -I "$IFACE" "$GATEWAY" >/dev/null 2>&1; then
        log "post-bounce: gateway ${GATEWAY} reachable — recovery successful"
        write_last_action ""
    else
        log "post-bounce: gateway ${GATEWAY} still unreachable — will escalate if failures persist"
        write_last_action "bounce"
    fi

    write_failures 0
fi
