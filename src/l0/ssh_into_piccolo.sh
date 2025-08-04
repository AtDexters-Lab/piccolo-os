#!/bin/bash
#
# Piccolo OS - Interactive SSH Script v1.1
#
# This script boots the Piccolo Live ISO in a temporary QEMU virtual machine
# and provides an interactive SSH console for debugging and exploration.
#
# v1.1 Improvements:
# - Fixes a race condition by retrying the SSH connection, allowing cloud-init
#   time to apply the SSH key before attempting to log in.
# - Adds a trap guard to the cleanup function to prevent it from ever running twice.
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
    echo "  --version      (Required) The version of the image being debugged (e.g., 1.0.0)."
    exit 1
}

check_dependencies() {
    log "Checking for required dependencies..."
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
    # FIX: Disable the trap to prevent it from running multiple times.
    trap - EXIT INT TERM
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
    log "### Step 1: Preparing the environment..."
    # Assign a value to the global test_dir variable
    test_dir="${SCRIPT_DIR}/build/ssh-session-${PICCOLO_VERSION}"
    local iso_path="${BUILD_DIR}/piccolo-os-live-${PICCOLO_VERSION}.iso"
    local ssh_key="${test_dir}/id_rsa_test"
    local config_drive_dir="${test_dir}/config-drive"

    # Clean up previous run just in case
    rm -rf "$test_dir"
    mkdir -p "$test_dir"
    mkdir -p "${config_drive_dir}/openstack/latest"
    if [ ! -f "$iso_path" ]; then log "Error: ISO file not found at ${iso_path}" >&2; exit 1; fi
    
    log "Generating temporary SSH key for this session..."
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
        -name "Piccolo-SSH-${PICCOLO_VERSION}" \
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
    # Step 3: Wait for SSH to become available
    # ---
    log "### Step 3: Waiting for SSH to become available on port ${SSH_PORT}..."
    
    # Wait for up to 2 minutes for SSH to be ready
    for i in {1..120}; do
        if ssh -q -p "$SSH_PORT" -i "${ssh_key}" -o ConnectTimeout=1 -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null "core@localhost" "echo 'SSH is ready'" 2>/dev/null; then
            log "SSH port is open."
            break
        fi
        
        if [ "$i" -eq 120 ]; then
            log "Error: Timed out waiting for SSH port." >&2
            exit 1
        fi
        echo -n "."
        sleep 1
    done

    # ---
    # Step 4: Connect to the interactive session
    # ---
    log "### Step 4: Connecting to interactive session (will retry a few times)..."
    log "Type 'exit' or press Ctrl+D to close the session and shut down the VM."
    
    local connected=false
    # FIX: Retry the connection to give cloud-init time to apply the key.
    for i in {1..5}; do
        if ssh -p "$SSH_PORT" -i "${ssh_key}" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o BatchMode=yes "core@localhost" "echo 'Connection successful'" 2>/dev/null; then
            # Now connect for real for the interactive session
            ssh -p "$SSH_PORT" \
                -i "${ssh_key}" \
                -o StrictHostKeyChecking=no \
                -o UserKnownHostsFile=/dev/null \
                "core@localhost"
            connected=true
            break
        else
            log "Login not yet ready, retrying in ${i}s..."
            sleep "$i"
        fi
    done

    if [ "$connected" = false ]; then
        log "Error: Could not establish SSH connection after multiple retries. The VM may not have provisioned correctly." >&2
        exit 1
    fi

    log "SSH session closed."
    # The script will now exit, and the 'trap' will trigger the cleanup function.
}

main "$@"
