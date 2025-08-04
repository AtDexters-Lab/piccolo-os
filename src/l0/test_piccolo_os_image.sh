#!/bin/bash
#
# Piccolo OS - Automated QA Smoke Test Script v2.6
#
# This script boots the Piccolo Live ISO in a VM using a direct QEMU command
# and runs a series of automated checks to verify its integrity and core functionality.
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

echo "--- CHECK 2: piccolod version ---"
if /usr/bin/piccolod --version | grep -q "${PICCOLO_VERSION_TO_TEST}"; then
    echo "PASS: piccolod version is correct."
else
    echo "FAIL: piccolod version does not match."
    /usr/bin/piccolod --version
    exit 1
fi

echo "--- CHECK 3: Container runtime ---"
# We need to start docker first in the live environment
sudo systemctl start docker
if docker run --rm hello-world; then
    echo "PASS: Container runtime is functional."
else
    echo "FAIL: Could not run hello-world container."
    exit 1
fi
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
