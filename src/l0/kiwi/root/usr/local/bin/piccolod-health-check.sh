#!/bin/bash
#
# Piccolo Health Check Script for MicroOS Integration
# Simple boolean health check using dedicated readiness endpoint
#

set -euo pipefail

HEALTH_ENDPOINT="http://localhost/api/v1/health/ready"
ECOSYSTEM_ENDPOINT="http://localhost/api/v1/ecosystem"
MAX_ATTEMPTS=30
RETRY_DELAY=2

echo "Starting Piccolo health check (max $MAX_ATTEMPTS attempts)..."

for attempt in $(seq 1 $MAX_ATTEMPTS); do
    # Simple curl check using dedicated boolean endpoint
    if curl -f -s "$HEALTH_ENDPOINT" >/dev/null 2>&1; then
        # Get the actual response for logging
        response=$(curl -s "$HEALTH_ENDPOINT" 2>/dev/null || echo '{"ready": false}')
        echo "✅ Piccolo health check PASSED - service ready"
        echo "Response: $response"
        exit 0
    else
        # Health check failed - get detailed info for debugging
        status_code=$(curl -s -w "%{http_code}" -o /dev/null "$HEALTH_ENDPOINT" 2>/dev/null || echo "000")
        
        case "$status_code" in
            "503")
                echo "❌ Attempt $attempt/$MAX_ATTEMPTS: Service UNHEALTHY (HTTP $status_code)"
                # Get detailed ecosystem info for troubleshooting
                if detailed_info=$(curl -s "$ECOSYSTEM_ENDPOINT" 2>/dev/null); then
                    echo "   Issue details: $(echo "$detailed_info" | jq -r '.summary // "Unable to get details"' 2>/dev/null || echo "JSON parse failed")"
                fi
                ;;
            "000")
                echo "🔌 Attempt $attempt/$MAX_ATTEMPTS: Connection failed - service may not be started yet"
                ;;
            *)
                echo "⚠️  Attempt $attempt/$MAX_ATTEMPTS: Unexpected HTTP status $status_code"
                ;;
        esac
        
        if [[ $attempt -lt $MAX_ATTEMPTS ]]; then
            echo "   Retrying in ${RETRY_DELAY}s..."
            sleep $RETRY_DELAY
        fi
    fi
done

echo ""
echo "🚨 CRITICAL: Piccolo health check FAILED after $MAX_ATTEMPTS attempts"
echo "   This indicates piccolod is not functioning properly after OS update"
echo "   MicroOS health-checker will likely trigger automatic rollback"
echo ""
echo "Readiness endpoint: $HEALTH_ENDPOINT"
echo "For detailed diagnostics, check: $ECOSYSTEM_ENDPOINT"

exit 1