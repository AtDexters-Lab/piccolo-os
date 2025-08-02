#!/bin/bash
#
# Piccolo OS - Automated QA Smoke Test Script v2.3
#
# This script boots the Piccolo Live ISO in a VM using a direct QEMU command
# and runs a series of automated checks to verify its integrity and core functionality.
# v2.3 uses cloud-init for SSH key injection, which is more reliable for live environments.
#

# ---
# Script Configuration and Safety
# ---
set -euo pipefail
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)

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
    local deps=("qemu-system-x86_64" "ssh" "ssh-keygen")
    for dep in "${deps[@]}"; do
        if ! command -v "$dep" &> /dev/null; then
            log "Error: Required dependency '$dep' is not installed." >&2
            exit 1
        fi
    done
    log "All dependencies are installed."
}

# ---
# Main Script Logic
# ---
main() {
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
    local test_dir="${SCRIPT_DIR}/build/test-${PICCOLO_VERSION}"
    local iso_path="${BUILD_DIR}/piccolo-os-live-${PICCOLO_VERSION}.iso"
    local ssh_key="${test_dir}/id_rsa_test"
    local config_drive_dir="${test_dir}/config-drive"
    
    mkdir -p "$test_dir"
    mkdir -p "${config_drive_dir}/openstack/latest"
    if [ ! -f "$iso_path" ]; then log "Error: ISO file not found at ${iso_path}" >&2; exit 1; fi
    
    log "Generating temporary SSH key for this test run..."
    # CORRECTED: Use 'echo y' for more reliable non-interactive overwrite.
    echo y | ssh-keygen -t rsa -b 4096 -f "$ssh_key" -N "" -q

    # Create a cloud-config file to inject the SSH key. This is the most
    # reliable method for live environments.
    cat > "${config_drive_dir}/openstack/latest/user_data" <<EOF
#cloud-config
ssh_authorized_keys:
  - $(cat "${ssh_key}.pub")
EOF

    # ---
    # Step 2: Boot VM with Direct QEMU Command
    # ---
    log "### Step 2: Booting Piccolo Live ISO in QEMU..."
    
    # We construct our own QEMU command for full control and robustness.
    qemu-system-x86_64 \
        -name "Piccolo-QA-${PICCOLO_VERSION}" \
        -m 2048 \
        -machine q35,accel=kvm \
        -cpu host \
        -smp "$(getconf _NPROCESSORS_ONLN)" \
        -netdev user,id=eth0,hostfwd=tcp::2222-:22 \
        -device virtio-net-pci,netdev=eth0 \
        -object rng-random,filename=/dev/urandom,id=rng0 \
        -device virtio-rng-pci,rng=rng0 \
        -drive file="$iso_path",media=cdrom,format=raw \
        -fsdev local,id=conf,security_model=none,readonly=on,path="${config_drive_dir}" \
        -device virtio-9p-pci,fsdev=conf,mount_tag=config-2 \
        -boot order=d \
        -nographic &
    local qemu_pid=$!
    log "QEMU started successfully with PID: $qemu_pid"

    # ---
    # Step 3: Wait for SSH and Run Checks
    # ---
    log "### Step 3: Waiting for SSH to become available..."
    local ssh_port=2222
    local ssh_opts="-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -i ${ssh_key}"
    local checks_passed=false
    
    # Wait for up to 2 minutes for SSH to be ready
    for i in {1..120}; do
        if ssh -p "$ssh_port" $ssh_opts "core@localhost" "echo 'SSH is ready'" &> /dev/null; then
            log "SSH is available. Running automated checks..."
            
            # These checks run against the LIVE environment booted from our ISO.
            ssh -p "$ssh_port" $ssh_opts "core@localhost" /bin/bash -s -- "$PICCOLO_VERSION" << 'EOF'
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
/usr/bin/piccolod --version | grep -q "${PICCOLO_VERSION_TO_TEST}"
echo "PASS: piccolod version is correct."

echo "--- CHECK 3: Container runtime ---"
# We need to start docker first in the live environment
sudo systemctl start docker
docker run --rm hello-world
echo "PASS: Container runtime is functional."
EOF
            checks_passed=true
            break
        fi
        echo -n "."
        sleep 1
    done

    # ---
    # Step 4: Report Results and Cleanup
    # ---
    log "" # Newline for cleaner output
    log "### Step 4: Cleaning up..."
    kill "$qemu_pid"
    log "QEMU process ($qemu_pid) terminated."
    
    rm -rf "$test_dir"
    log "Test directory cleaned up."

    if [ "$checks_passed" = true ]; then
        log "✅ ✅ ✅ ALL CHECKS PASSED ✅ ✅ ✅"
        exit 0
    else
        log "❌ ❌ ❌ TEST FAILED: SSH connection timed out or a check failed. ❌ ❌ ❌"
        exit 1
    fi
}

main "$@"
