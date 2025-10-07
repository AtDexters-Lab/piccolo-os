#!/bin/bash
#
# Piccolo Health Check Script for PRODUCTION MicroOS Integration
# - On success, clear pending staged sentinel and record success.
# - On failure, systemd will trigger automatic rollback.

set -euo pipefail

STATE_DIR="/var/lib/piccolo/os-update"
STATE_FILE="${STATE_DIR}/state"

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

mark_success() {
  mkdir -p "$STATE_DIR"
  if [ -f "$STATE_FILE" ]; then
    # shellcheck disable=SC1090
    . "$STATE_FILE" || true
  fi
  if [ "${PENDING_GEN:-0}" -gt 0 ]; then
    LAST_SUCCESS_GEN=${PENDING_GEN}
    LAST_SUCCESS_SUBVOL=$(get_current_subvol)
    PENDING_GEN=0
    STAGED_FROM_SUBVOL=""
    UPDATED_AT=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    cat > "$STATE_FILE" <<EOF
PENDING_GEN=${PENDING_GEN}
ROLLED_BACK_GEN=${ROLLED_BACK_GEN:-0}
LAST_SUCCESS_GEN=${LAST_SUCCESS_GEN}
STAGED_FROM_SUBVOL=${STAGED_FROM_SUBVOL}
LAST_SUCCESS_SUBVOL=${LAST_SUCCESS_SUBVOL}
UPDATED_AT=${UPDATED_AT}
EOF
  fi
}

HEALTH_ENDPOINT="http://localhost/api/v1/health/ready"
MAX_ATTEMPTS=10
RETRY_DELAY=1

echo "Piccolo PRODUCTION health check starting..."

for attempt in $(seq 1 $MAX_ATTEMPTS); do
  if curl -f -s --connect-timeout 2 --max-time 5 "$HEALTH_ENDPOINT" >/dev/null 2>&1; then
    echo "✅ Piccolo PRODUCTION health check PASSED"
    mark_success || true
    exit 0
  else
    status_code=$(curl -s --connect-timeout 2 --max-time 5 -w "%{http_code}" -o /dev/null "$HEALTH_ENDPOINT" 2>/dev/null || echo "000")
    case "$status_code" in
      "503") echo "❌ PRODUCTION health check FAILED: Service unhealthy (attempt $attempt/$MAX_ATTEMPTS)" ;;
      "000") echo "🔌 PRODUCTION health check FAILED: Connection failed (attempt $attempt/$MAX_ATTEMPTS)" ;;
      *)      echo "⚠️  PRODUCTION health check FAILED: HTTP $status_code (attempt $attempt/$MAX_ATTEMPTS)" ;;
    esac
    if [[ $attempt -lt $MAX_ATTEMPTS ]]; then
      sleep $RETRY_DELAY
    fi
  fi
done

echo ""
echo "🚨 CRITICAL: Piccolo PRODUCTION health check FAILED after $MAX_ATTEMPTS attempts"
echo "   MicroOS will trigger automatic rollback"
exit 1
