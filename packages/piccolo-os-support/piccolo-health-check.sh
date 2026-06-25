#!/bin/bash
# Piccolo OS Health Check Plugin
# Verifies that the piccolod control plane is healthy (no fatal component errors).
#
# Robustness:
# 1. We retry for up to 30 seconds to handle startup latency.
# 2. We depend on systemd ordering (via drop-in), but this is a safety net.

run_checks() {
    local max_attempts=15
    local sleep_seconds=2
    local attempt=1
    local health_url="http://127.0.0.1:80/api/v1/health/ready"

    echo "Starting piccolod health check..."

    while [ $attempt -le $max_attempts ]; do
        local response curl_status http_status response_body

        response="$(/usr/bin/curl --silent --show-error --max-time 2 --write-out $'\n%{http_code}' "$health_url" 2>&1)"
        curl_status=$?
        http_status="${response##*$'\n'}"
        response_body="${response%$'\n'*}"

        # Verify piccolod API is responsive and reporting no fatal component errors.
        if [ "$curl_status" -eq 0 ] && [ "$http_status" = "200" ]; then
            echo "piccolod is healthy."
            return 0
        fi

        response_body="${response_body//$'\r'/ }"
        response_body="${response_body//$'\n'/ }"
        response_body="${response_body//$'\t'/ }"
        if [ ${#response_body} -gt 1000 ]; then
            response_body="${response_body:0:1000}...<truncated>"
        fi

        echo "Attempt $attempt/$max_attempts: piccolod API not ready yet (curl_exit=${curl_status}, http_status=${http_status}). Retrying in ${sleep_seconds}s..."
        if [ -n "$response_body" ]; then
            echo "Attempt $attempt/$max_attempts response: ${response_body}"
        fi
        sleep $sleep_seconds
        ((attempt++))
    done

    echo "ERROR: piccolod API did not become ready after $((max_attempts * sleep_seconds)) seconds."
    return 1
}

stop_services() {
    # If the check fails, health-checker calls this to stop the service
    # before potentially triggering a rollback.
    echo "Stopping piccolod service due to failed health check..."
    /usr/bin/systemctl stop piccolod.service
}

case "$1" in
    check)
        run_checks
        ;;
    stop)
        stop_services
        ;;
    *)
        echo "Usage: $0 {check|stop}"
        exit 1
        ;;
esac
