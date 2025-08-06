#!/bin/bash
#
# Piccolo OS - Automated QA Smoke Test Script v2.8
#
# This script boots the Piccolo Live ISO in a VM using a direct QEMU command
# and runs a series of automated checks to verify its integrity and core functionality.
#
# v2.8 Improvements:
# - Replaced external permission tests with piccolod's built-in ecosystem test (CHECK 6)
# - Uses /api/v1/ecosystem endpoint for comprehensive self-validation
# - Tests actual runtime permissions from piccolod's security context
# - Validates systemd hardening, device access, and manager components
# - Provides detailed pass/warn/fail analysis with actionable feedback
#
# v2.6 Improvements:
# - Fixes a variable scope bug in the cleanup trap by making key variables global.
#   This ensures cleanup runs correctly even when tests fail and the script exits early.
#

# ---
# Script Configuration and Safety
# ---
set -euo pipefail
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
SSH_PORT=2222 # The host port to forward to the VM's SSH port (22)

# ---
# Global variables for the trap handler
# ---
# These are defined globally so the cleanup function can access them
# even if the script exits unexpectedly from within a function.
qemu_pid=""
test_dir=""


# ---
# Helper Functions
# ---
log() {
    echo "[$(date +'%Y-%m-%d %H:%M:%S')] $*"
}

usage() {
    echo "Usage: $0 --build-dir <PATH_TO_BUILD_OUTPUT> --version <VERSION>"
    echo "  --build-dir    (Required) The path to the build output directory containing the ISO."
    echo "  --version      (Required) The version of the image being tested (e.g., 1.0.0)."
    exit 1
}

check_dependencies() {
    log "Checking for required dependencies..."
    # Add 'ss' for network socket checking
    local deps=("qemu-system-x86_64" "ssh" "ssh-keygen" "ss")
    for dep in "${deps[@]}"; do
        if ! command -v "$dep" &> /dev/null; then
            log "Error: Required dependency '$dep' is not installed." >&2
            log "On Debian/Ubuntu, try: sudo apt-get install -y qemu-system-x86 openssh-client iproute2"
            exit 1
        fi
    done
    log "All dependencies are installed."
}

# This function contains the robust cleanup logic.
cleanup() {
    log "### Cleaning up..."
    # Check if the QEMU PID was set and if the process still exists
    if [ -n "${qemu_pid:-}" ] && ps -p "$qemu_pid" > /dev/null; then
        log "Attempting graceful shutdown of QEMU PID: $qemu_pid..."
        kill "$qemu_pid" 2>/dev/null
        # Wait up to 3 seconds for it to terminate
        for _ in {1..3}; do
            if ! ps -p "$qemu_pid" > /dev/null; then
                break
            fi
            sleep 1
        done

        # If it's still alive, it's stuck. Force kill it.
        if ps -p "$qemu_pid" > /dev/null; then
            log "QEMU did not shut down gracefully. Forcing termination (kill -9)..."
            kill -9 "$qemu_pid" 2>/dev/null
        fi
        log "QEMU process terminated."
    fi
    
    # Now that test_dir is global, this will work reliably.
    if [ -d "${test_dir:-}" ]; then
        rm -rf "$test_dir"
        log "Test directory cleaned up."
    fi
    log "Cleanup complete."
}

# ---
# Main Script Logic
# ---
main() {
    # Set up the trap to call the cleanup function on exit, interrupt, or termination.
    trap cleanup EXIT INT TERM

    # ---
    # Step 0: Parse Arguments and Check Dependencies
    # ---
    if [ "$#" -eq 0 ]; then usage; fi
    local BUILD_DIR=""
    local PICCOLO_VERSION=""
    while [ "$#" -gt 0 ]; do
        case "$1" in
            --build-dir) BUILD_DIR="$2"; shift 2;;
            --version) PICCOLO_VERSION="$2"; shift 2;;
            *) usage;;
        esac
    done
    if [ -z "${BUILD_DIR:-}" ] || [ -z "${PICCOLO_VERSION:-}" ]; then usage; fi
    if [ ! -d "$BUILD_DIR" ]; then log "Error: Build directory not found at $BUILD_DIR" >&2; exit 1; fi
    check_dependencies

    # ---
    # Step 1: Prepare Test Environment
    # ---
    log "### Step 1: Preparing the test environment..."
    # Assign a value to the global test_dir variable
    test_dir="${SCRIPT_DIR}/build/test-${PICCOLO_VERSION}"
    local iso_path="${BUILD_DIR}/piccolo-os-live-${PICCOLO_VERSION}.iso"
    local ssh_key="${test_dir}/id_rsa_test"
    local config_drive_dir="${test_dir}/config-drive"

    # Clean up previous run just in case
    rm -rf "$test_dir"
    mkdir -p "$test_dir"
    mkdir -p "${config_drive_dir}/openstack/latest"
    if [ ! -f "$iso_path" ]; then log "Error: ISO file not found at ${iso_path}" >&2; exit 1; fi
    
    log "Generating temporary SSH key for this test run..."
    ssh-keygen -t rsa -b 4096 -f "$ssh_key" -N "" -q

    log "Creating cloud-config for SSH key injection..."
    cat > "${config_drive_dir}/openstack/latest/user_data" <<EOF
#cloud-config
ssh_authorized_keys:
  - $(cat "${ssh_key}.pub")
EOF

    # ---
    # Step 2: Boot VM with Direct QEMU Command
    # ---
    log "### Step 2: Booting Piccolo Live ISO in QEMU..."
    
    # Check if the port is already in use
    if ss -Hltn "sport = :${SSH_PORT}" | grep -q "LISTEN"; then
        log "Error: Port ${SSH_PORT} is already in use. Please kill the process using it or change SSH_PORT in the script." >&2
        exit 1
    fi
    
    qemu-system-x86_64 \
        -name "Piccolo-QA-${PICCOLO_VERSION}" \
        -m 2048 \
        -machine q35,accel=kvm \
        -cpu host \
        -smp "$(getconf _NPROCESSORS_ONLN)" \
        -netdev user,id=eth0,hostfwd=tcp::${SSH_PORT}-:22 \
        -device virtio-net-pci,netdev=eth0 \
        -object rng-random,filename=/dev/urandom,id=rng0 \
        -device virtio-rng-pci,rng=rng0 \
        -drive file="$iso_path",media=cdrom,format=raw \
        -fsdev local,id=conf,security_model=none,readonly=on,path="${config_drive_dir}" \
        -device virtio-9p-pci,fsdev=conf,mount_tag=config-2 \
        -boot order=d \
        -nographic &
    # Assign a value to the global qemu_pid variable
    qemu_pid=$!
    log "QEMU started successfully with PID: $qemu_pid"

    # ---
    # Step 3: Wait for SSH and Run Checks
    # ---
    log "### Step 3: Waiting for SSH to become available on port ${SSH_PORT}..."
    local ssh_opts="-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o ConnectTimeout=5"
    local checks_passed=false
    
    # Wait for up to 2 minutes for SSH to be ready
    for i in {1..120}; do
        # Use -q to suppress banner, but allow errors to be seen
        if ssh -q -p "$SSH_PORT" -i "${ssh_key}" $ssh_opts "core@localhost" "echo 'SSH is ready'" 2>/dev/null; then
            log "SSH is available. Running automated checks..."
            
            # These checks run against the LIVE environment booted from our ISO.
            ssh -p "$SSH_PORT" -i "${ssh_key}" $ssh_opts "core@localhost" /bin/bash -s -- "$PICCOLO_VERSION" << 'EOF'
set -euo pipefail
PICCOLO_VERSION_TO_TEST="$1"

echo "--- CHECK 1: piccolod binary ---"
if [ -x "/usr/bin/piccolod" ]; then
    echo "PASS: piccolod binary is present and executable."
else
    echo "FAIL: piccolod binary not found or not executable."
    exit 1
fi

echo "--- CHECK 2: piccolod service status ---"
# The service should be enabled and running by default.
# Add retry logic since services may take time to start during boot
SERVICE_ACTIVE=false
for i in {1..10}; do
    if sudo systemctl is-active --quiet piccolod.service; then
        SERVICE_ACTIVE=true
        break
    fi
    echo "Attempt $i: Service not yet active. Retrying in 2 seconds..."
    sleep 2
done

if [ "$SERVICE_ACTIVE" = true ]; then
    echo "PASS: piccolod service is active."
else
    echo "FAIL: piccolod service is not active after multiple attempts."
    sudo systemctl status piccolod.service
    exit 1
fi

echo "--- CHECK 3: piccolod process runs as root ---"
# Verify that the piccolod process is running as root user
PICCOLOD_USER=$(ps -eo user,comm | grep "^[[:space:]]*root[[:space:]]*piccolod$" | awk '{print $1}')
if [ "$PICCOLOD_USER" = "root" ]; then
    echo "PASS: piccolod process is running as root user."
else
    echo "FAIL: piccolod process is not running as root user."
    echo "Current process info:"
    ps -eo user,pid,comm | grep piccolod
    exit 1
fi

echo "--- CHECK 4: piccolod version via HTTP ---"
# Use curl to get the version from the API endpoint
# Add a retry loop in case the service is slow to start accepting connections
for i in {1..5}; do
    # Use --fail to make curl exit with an error if the HTTP request fails (e.g., 404, 500)
    # Use -s for silent mode
    VERSION_JSON=$(curl -s --fail http://localhost:8080/version 2>/dev/null)
    if [ $? -eq 0 ]; then
        break
    fi
    echo "Attempt $i: Failed to contact version endpoint. Retrying in 2 seconds..."
    sleep 2
done

if [ -z "${VERSION_JSON:-}" ]; then
    echo "FAIL: Could not retrieve version from endpoint after multiple attempts."
    exit 1
fi

# Extract version from JSON using basic tools to avoid jq dependency
EXTRACTED_VERSION=$(echo "$VERSION_JSON" | sed -n 's/.*"version":"\([^"]*\)".*/\1/p')

if [ -z "${EXTRACTED_VERSION}" ]; then
    echo "FAIL: Could not parse version from JSON response: ${VERSION_JSON}"
    exit 1
fi

echo "Found version '${EXTRACTED_VERSION}' from endpoint."

if [ "${EXTRACTED_VERSION}" == "${PICCOLO_VERSION_TO_TEST}" ]; then
    echo "PASS: piccolod version is correct."
else
    echo "FAIL: piccolod version does not match. Expected ${PICCOLO_VERSION_TO_TEST}, got ${EXTRACTED_VERSION}."
    exit 1
fi

echo "--- CHECK 5: Container runtime ---"
# We need to start docker first in the live environment
sudo systemctl start docker
if docker run --rm hello-world; then
    echo "PASS: Container runtime is functional."
else
    echo "FAIL: Could not run hello-world container."
    exit 1
fi

echo "--- CHECK 6: Ecosystem and environment validation ---"
# Use piccolod's built-in ecosystem test to validate its own environment
# This tests what piccolod can actually access from within its systemd security context
ECOSYSTEM_JSON=""
for i in {1..3}; do
    ECOSYSTEM_JSON=$(curl -s --fail http://localhost:8080/api/v1/ecosystem 2>/dev/null)
    if [ $? -eq 0 ] && [ -n "$ECOSYSTEM_JSON" ]; then
        break
    fi
    echo "Attempt $i: Failed to contact ecosystem endpoint. Retrying in 2 seconds..."
    sleep 2
done

if [ -z "$ECOSYSTEM_JSON" ]; then
    echo "FAIL: Could not retrieve ecosystem test results after multiple attempts."
    exit 1
fi

# Parse the overall status using basic shell tools
OVERALL_STATUS=$(echo "$ECOSYSTEM_JSON" | sed -n 's/.*"overall":"\([^"]*\)".*/\1/p')
SUMMARY=$(echo "$ECOSYSTEM_JSON" | sed -n 's/.*"summary":"\([^"]*\)".*/\1/p')

echo "Ecosystem Status: $OVERALL_STATUS"
echo "Summary: $SUMMARY"

# Display individual check results
echo "Individual Check Results:"
echo "$ECOSYSTEM_JSON" | grep -o '"name":"[^"]*","status":"[^"]*","description":"[^"]*"' | while read -r line; do
    CHECK_NAME=$(echo "$line" | sed -n 's/.*"name":"\([^"]*\)".*/\1/p')
    CHECK_STATUS=$(echo "$line" | sed -n 's/.*"status":"\([^"]*\)".*/\1/p') 
    CHECK_DESC=$(echo "$line" | sed -n 's/.*"description":"\([^"]*\)".*/\1/p')
    
    case "$CHECK_STATUS" in
        "pass") echo "  ✅ $CHECK_NAME: $CHECK_DESC" ;;
        "warn") echo "  ⚠️  $CHECK_NAME: $CHECK_DESC" ;;
        "fail") echo "  ❌ $CHECK_NAME: $CHECK_DESC" ;;
        "info") echo "  ℹ️  $CHECK_NAME: $CHECK_DESC" ;;
        *) echo "  ❓ $CHECK_NAME: $CHECK_DESC (status: $CHECK_STATUS)" ;;
    esac
done

# Evaluate overall result
case "$OVERALL_STATUS" in
    "healthy")
        echo "PASS: Ecosystem is healthy - all checks passed."
        ;;
    "degraded") 
        echo "PASS: Ecosystem is functional but degraded - some features may be limited."
        echo "WARN: $SUMMARY"
        ;;
    "unhealthy")
        echo "FAIL: Ecosystem is unhealthy - critical issues detected."
        echo "ERROR: $SUMMARY"
        exit 1
        ;;
    *)
        echo "FAIL: Unknown ecosystem status: $OVERALL_STATUS"
        exit 1
        ;;
esac
EOF
            checks_passed=true
            break
        fi
        echo -n "."
        sleep 1
    done

    # ---
    # Step 4: Report Results
    # ---
    log "" # Newline for cleaner output
    if [ "$checks_passed" = true ]; then
        log "✅ ✅ ✅ ALL CHECKS PASSED ✅ ✅ ✅"
        exit 0
    else
        log "❌ ❌ ❌ TEST FAILED: SSH connection timed out or a check failed. ❌ ❌ ❌"
        log "Last SSH attempt details:"
        # Run with verbose output to diagnose the failure
        ssh -v -p "$SSH_PORT" -i "${ssh_key}" $ssh_opts "core@localhost" "echo 'Final connection attempt'"
        exit 1
    fi
}

main "$@"
