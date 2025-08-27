#!/bin/bash
#
# Piccolo OS - Interactive SSH Script v2.1
#
# This script boots the Piccolo MicroOS disk image with UEFI support in a QEMU virtual machine
# and provides an interactive SSH console for debugging and exploration.
#
# v2.1 Refactor:
# - Extract shared utilities to lib/piccolo_common.sh for maintainability
# - Reduce code duplication with test_piccolo_os_image.sh
#
# v2.0 Major Updates:
# - Updated for MicroOS-based artifacts (piccolo-os-dev.x86_64-*.raw naming)
# - Added UEFI/OVMF boot support with proper firmware configuration
# - Updated for cloud-init with seed ISO approach
# - Added UEFI firmware detection and configuration
# - Fixed SSH authentication for root user with proper permissions
#

# ---
# Script Configuration and Safety
# ---
set -euo pipefail
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
SSH_PORT=2222 # The host port to forward to the VM's SSH port (22)

# Source shared utilities
source "${SCRIPT_DIR}/lib/piccolo_common.sh"

# ---
# Global variables for the trap handler
# ---
# These are defined globally so the cleanup function can access them
# even if the script exits unexpectedly from within a function.
qemu_pid=""
test_dir=""


# ---
# Helper Functions (log function now sourced from common utilities)
# ---

usage() {
    echo "Usage: $0 --build-dir <PATH_TO_BUILD_OUTPUT> --version <VERSION>"
    echo "  --build-dir    (Required) The path to the build output directory containing the disk image."
    echo "  --version      (Required) The version of the image being debugged (e.g., 1.0.0)."
    echo ""
    echo "The script will automatically detect dev or prod variant .raw disk images."
    exit 1
}

check_dependencies() {
    check_vm_dependencies
}

# This function contains the robust cleanup logic.
cleanup() {
    # FIX: Disable the trap to prevent it from running multiple times.
    trap - EXIT INT TERM
    log "### Cleaning up..."
    
    cleanup_qemu_process "$qemu_pid"
    cleanup_test_directory "$test_dir"
    
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
    
    validate_build_directory "$BUILD_DIR"
    check_dependencies

    # ---
    # Step 1: Prepare Test Environment
    # ---
    log "### Step 1: Preparing the environment..."
    # Assign a value to the global test_dir variable
    test_dir="${SCRIPT_DIR}/build/ssh-session-${PICCOLO_VERSION}"
    local image_path=$(resolve_piccolo_image_path "$BUILD_DIR" "$PICCOLO_VERSION")
    local ssh_key="${test_dir}/id_rsa_test"
    local uefi_code_copy="${test_dir}/$(basename "$UEFI_CODE")"
    local uefi_vars_copy="${test_dir}/$(basename "$UEFI_VARS")"
    local seed_iso="${test_dir}/seed.iso"
    local cloud_config_dir="${test_dir}/cloud-config"

    # Clean up previous run just in case
    rm -rf "$test_dir"
    mkdir -p "$test_dir" "$cloud_config_dir"
    validate_disk_image "$image_path"
    
    log "Creating local copies of UEFI firmware files..."
    cp "$UEFI_CODE" "$uefi_code_copy"
    cp "$UEFI_VARS" "$uefi_vars_copy"
    
    generate_temp_ssh_key "$ssh_key"

    log "Creating cloud-init configuration for SSH access..."
    generate_cloud_init_user_data "$(cat "${ssh_key}.pub")" "$cloud_config_dir"
    generate_cloud_init_meta_data "$cloud_config_dir" "$0"

    log "Creating cloud-init seed ISO with CIDATA label..."
    genisoimage -quiet -output "$seed_iso" -volid CIDATA -joliet -rational-rock "${cloud_config_dir}/user-data" "${cloud_config_dir}/meta-data" 2>/dev/null
    if [ $? -ne 0 ]; then
        log "Error: Failed to create cloud-init seed ISO." >&2
        exit 1
    fi

    # ---
    # Step 2: Boot VM with Direct QEMU Command
    # ---
    log "### Step 2: Booting Piccolo disk image in QEMU..."
    
    # Check if the port is already in use
    if ss -Hltn "sport = :${SSH_PORT}" | grep -q "LISTEN"; then
        log "Error: Port ${SSH_PORT} is already in use. Please kill the process using it or change SSH_PORT in the script." >&2
        exit 1
    fi
    
    qemu-system-x86_64 \
        -name "Piccolo-SSH-${PICCOLO_VERSION}" \
        -enable-kvm \
        -machine q35,smm=on,accel=kvm \
        -cpu host \
        -smp 4 \
        -m 4096 \
        -drive if=pflash,format=raw,readonly=on,file="$uefi_code_copy" \
        -drive if=pflash,format=raw,file="$uefi_vars_copy" \
        -drive file="$image_path",format=raw \
        -drive file="$seed_iso",media=cdrom,if=virtio \
        -netdev user,id=n0,hostfwd=tcp:127.0.0.1:${SSH_PORT}-:22 \
        -device virtio-net-pci,netdev=n0 \
        -nographic &
    # Assign a value to the global qemu_pid variable
    qemu_pid=$!
    log "QEMU started successfully with PID: $qemu_pid"

    # ---
    # Step 3: Wait for SSH to become available
    # ---
    log "### Step 3: Waiting for SSH to become available on port ${SSH_PORT}..."
    
    # Wait for cloud-init to complete and SSH to be ready
    log "Waiting for cloud-init and SSH setup..."
    sleep 60  # Give cloud-init time to complete
    
    # Wait for up to 2 minutes for SSH to be ready
    for i in {1..120}; do
        if ssh -q -p "$SSH_PORT" -i "${ssh_key}" -o ConnectTimeout=1 -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null "root@localhost" "echo 'SSH is ready'" 2>/dev/null; then
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
    # Retry the connection to give cloud-init time to apply the key.
    for i in {1..5}; do
        if ssh -p "$SSH_PORT" -i "${ssh_key}" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o BatchMode=yes "root@localhost" "echo 'Connection successful'" 2>/dev/null; then
            # Now connect for real for the interactive session
            ssh -p "$SSH_PORT" \
                -i "${ssh_key}" \
                -o StrictHostKeyChecking=no \
                -o UserKnownHostsFile=/dev/null \
                "root@localhost"
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
