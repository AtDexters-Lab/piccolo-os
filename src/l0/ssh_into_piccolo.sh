#!/bin/bash
#
# Piccolo OS - Interactive SSH Script v2.0
#
# This script boots the Piccolo MicroOS ISO with UEFI support in a QEMU virtual machine
# and provides an interactive SSH console for debugging and exploration.
#
# v2.0 Major Updates:
# - Updated for MicroOS-based artifacts (piccolo-os-x86_64-*.iso naming)
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
    local deps=("qemu-system-x86_64" "ssh" "ssh-keygen" "ss" "genisoimage")
    for dep in "${deps[@]}"; do
        if ! command -v "$dep" &> /dev/null; then
            log "Error: Required dependency '$dep' is not installed." >&2
            log "On Debian/Ubuntu, try: sudo apt-get install -y qemu-system-x86 openssh-client iproute2 genisoimage"
            exit 1
        fi
    done
    
    # Check for UEFI firmware files (secboot versions preferred, then 4MB versions)
    local uefi_firmware_paths=(
        "/usr/share/OVMF/OVMF_CODE_4M.secboot.fd"
        "/usr/share/OVMF/OVMF_CODE.secboot.fd"
        "/usr/share/OVMF/OVMF_CODE_4M.fd"
        "/usr/share/OVMF/OVMF_CODE.fd"
    )
    
    local uefi_vars_paths=(
        "/usr/share/OVMF/OVMF_VARS_4M.fd"
        "/usr/share/OVMF/OVMF_VARS.fd"
    )
    
    UEFI_CODE=""
    UEFI_VARS=""
    
    for path in "${uefi_firmware_paths[@]}"; do
        if [ -f "$path" ]; then
            UEFI_CODE="$path"
            break
        fi
    done
    
    for path in "${uefi_vars_paths[@]}"; do
        if [ -f "$path" ]; then
            UEFI_VARS="$path"
            break
        fi
    done
    
    if [ -z "$UEFI_CODE" ] || [ -z "$UEFI_VARS" ]; then
        log "Error: UEFI firmware files not found. Please install OVMF package." >&2
        log "On Debian/Ubuntu, try: sudo apt-get install -y ovmf"
        exit 1
    fi
    
    log "All dependencies are installed."
    log "Using UEFI firmware: $UEFI_CODE"
    log "Using UEFI vars: $UEFI_VARS"
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
    local iso_path="${BUILD_DIR}/piccolo-os-x86_64-${PICCOLO_VERSION}.iso"
    local ssh_key="${test_dir}/id_rsa_test"
    local uefi_code_copy="${test_dir}/$(basename "$UEFI_CODE")"
    local uefi_vars_copy="${test_dir}/$(basename "$UEFI_VARS")"
    local seed_iso="${test_dir}/seed.iso"
    local cloud_config_dir="${test_dir}/cloud-config"

    # Clean up previous run just in case
    rm -rf "$test_dir"
    mkdir -p "$test_dir" "$cloud_config_dir"
    if [ ! -f "$iso_path" ]; then log "Error: ISO file not found at ${iso_path}" >&2; exit 1; fi
    
    log "Creating local copies of UEFI firmware files..."
    cp "$UEFI_CODE" "$uefi_code_copy"
    cp "$UEFI_VARS" "$uefi_vars_copy"
    
    log "Generating temporary SSH key for this session..."
    ssh-keygen -t rsa -b 4096 -f "$ssh_key" -N "" -q

    log "Creating cloud-init configuration for SSH access..."
    cat > "${cloud_config_dir}/user-data" << EOF
#cloud-config
users:
  - name: root
    lock_passwd: false
    plain_text_passwd: 'piccolo123'
    ssh_authorized_keys:
      - $(cat "${ssh_key}.pub")

ssh_pwauth: true
disable_root: false

write_files:
  - path: /etc/ssh/sshd_config.d/99-cloud-init-root.conf
    content: |
      PermitRootLogin yes
      PasswordAuthentication yes
      PubkeyAuthentication yes
    permissions: '0600'

runcmd:
  - systemctl enable --now sshd
  - systemctl reload sshd
EOF

    cat > "${cloud_config_dir}/meta-data" << EOF
instance-id: piccolo-ssh-$(date +%s)
local-hostname: piccolo-ssh
EOF

    log "Creating cloud-init seed ISO with CIDATA label..."
    genisoimage -quiet -output "$seed_iso" -volid CIDATA -joliet -rational-rock "${cloud_config_dir}/user-data" "${cloud_config_dir}/meta-data" 2>/dev/null
    if [ $? -ne 0 ]; then
        log "Error: Failed to create cloud-init seed ISO." >&2
        exit 1
    fi

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
        -enable-kvm \
        -machine q35,smm=on,accel=kvm \
        -cpu host \
        -smp 4 \
        -m 4096 \
        -drive if=pflash,format=raw,readonly=on,file="$uefi_code_copy" \
        -drive if=pflash,format=raw,file="$uefi_vars_copy" \
        -cdrom "$iso_path" \
        -drive file="$seed_iso",media=cdrom,if=virtio \
        -boot d \
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
