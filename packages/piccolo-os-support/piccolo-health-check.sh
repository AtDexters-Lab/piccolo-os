#!/bin/bash
# Piccolo OS Health Check Plugin
# Verifies that the piccolod control plane is serving traffic.
#
# Robustness:
# 1. We retry for up to 30 seconds to handle startup latency.
# 2. We depend on systemd ordering (via drop-in), but this is a safety net.

run_checks() {
    local max_attempts=15
    local sleep_seconds=2
    local attempt=1

    echo "Starting piccolod health check..."

    while [ $attempt -le $max_attempts ]; do
        # Verify piccolod API is responsive
        if /usr/bin/curl --silent --fail --max-time 2 http://127.0.0.1:80/api/v1/health/live >/dev/null; then
            echo "piccolod is healthy."
            return 0
        fi

        echo "Attempt $attempt/$max_attempts: piccolod API not ready yet. Retrying in ${sleep_seconds}s..."
        sleep $sleep_seconds
        ((attempt++))
    done

    echo "ERROR: piccolod API failed to respond after $((max_attempts * sleep_seconds)) seconds."
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