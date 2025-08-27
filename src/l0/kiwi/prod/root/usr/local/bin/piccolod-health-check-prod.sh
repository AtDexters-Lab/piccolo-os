#!/bin/bash
#
# Piccolo Health Check Script for PRODUCTION MicroOS Integration
# PRODUCTION MODE: Minimal output, fast failure detection only
#

set -euo pipefail

HEALTH_ENDPOINT="http://localhost/api/v1/health/ready"
MAX_ATTEMPTS=10  # Reduced for production - fail fast
RETRY_DELAY=1    # Faster retry for production

# PRODUCTION: Minimal logging - only critical status
echo "Piccolo PRODUCTION health check starting..."

for attempt in $(seq 1 $MAX_ATTEMPTS); do
    # Simple curl check using dedicated boolean endpoint
    if curl -f -s --connect-timeout 2 --max-time 5 "$HEALTH_ENDPOINT" >/dev/null 2>&1; then
        # PRODUCTION: Minimal success logging
        echo "✅ Piccolo PRODUCTION health check PASSED"
        exit 0
    else
        # PRODUCTION: Minimal failure logging - no detailed diagnostics
        status_code=$(curl -s --connect-timeout 2 --max-time 5 -w "%{http_code}" -o /dev/null "$HEALTH_ENDPOINT" 2>/dev/null || echo "000")
        
        case "$status_code" in
            "503")
                echo "❌ PRODUCTION health check FAILED: Service unhealthy (attempt $attempt/$MAX_ATTEMPTS)"
                ;;
            "000")
                echo "🔌 PRODUCTION health check FAILED: Connection failed (attempt $attempt/$MAX_ATTEMPTS)"
                ;;
            *)
                echo "⚠️  PRODUCTION health check FAILED: HTTP $status_code (attempt $attempt/$MAX_ATTEMPTS)"
                ;;
        esac
        
        if [[ $attempt -lt $MAX_ATTEMPTS ]]; then
            sleep $RETRY_DELAY
        fi
    fi
done

echo ""
echo "🚨 CRITICAL: Piccolo PRODUCTION health check FAILED after $MAX_ATTEMPTS attempts"
echo "   MicroOS will trigger automatic rollback"

# PRODUCTION: No detailed diagnostics exposed - security hardening
exit 1