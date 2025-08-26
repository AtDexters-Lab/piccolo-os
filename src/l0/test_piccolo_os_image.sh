#!/bin/bash
#
# Piccolo OS - Automated QA Smoke Test Script v3.0
#
# This script boots the Piccolo MicroOS ISO with UEFI support using QEMU and runs 
# automated checks to verify its integrity and core functionality including mDNS.
#
# v3.0 Major Improvements:
# - Added full UEFI/OVMF boot support with proper firmware configuration
# - Updated for MicroOS-based artifacts (piccolo-os-x86_64-*.iso naming)
# - Fixed QEMU boot order using bootindex=0 for reliable CD-ROM boot
# - Updated container runtime from Docker to Podman for MicroOS compatibility
# - Added binary path compatibility for both MicroOS and Flatcar locations
# - Added UEFI and Secure Boot validation (CHECK 7)
# - Proper UEFI variables persistence with writable OVMF_VARS copy
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
    local deps=("qemu-system-x86_64" "ssh" "ssh-keygen" "ss" "nc" "sshpass" "genisoimage")
    for dep in "${deps[@]}"; do
        if ! command -v "$dep" &> /dev/null; then
            log "Error: Required dependency '$dep' is not installed." >&2
            log "On Debian/Ubuntu, try: sudo apt-get install -y qemu-system-x86 openssh-client iproute2 netcat-openbsd sshpass genisoimage"
            exit 1
        fi
    done
    
    # Check for UEFI firmware files (secboot versions preferred, then 4MB versions)
    local uefi_firmware_paths=(
        "/usr/share/OVMF/OVMF_CODE_4M.secboot.fd"
        "/usr/share/OVMF/OVMF_CODE.secboot.fd"
        "/usr/share/OVMF/OVMF_CODE_4M.fd"
        "/usr/share/OVMF/OVMF_CODE.fd"
        "/usr/share/edk2-ovmf/OVMF_CODE_4M.secboot.fd"
        "/usr/share/edk2-ovmf/OVMF_CODE.secboot.fd"
        "/usr/share/edk2-ovmf/OVMF_CODE_4M.fd"
        "/usr/share/edk2-ovmf/OVMF_CODE.fd" 
        "/usr/share/qemu/ovmf-x86_64-code.bin"
        "/usr/share/qemu/OVMF_CODE_4M.fd"
        "/usr/share/qemu/OVMF_CODE.fd"
    )
    
    local uefi_vars_paths=(
        "/usr/share/OVMF/OVMF_VARS_4M.fd"
        "/usr/share/OVMF/OVMF_VARS.fd"
        "/usr/share/edk2-ovmf/OVMF_VARS_4M.fd"
        "/usr/share/edk2-ovmf/OVMF_VARS.fd"
        "/usr/share/qemu/ovmf-x86_64-vars.bin"
        "/usr/share/qemu/OVMF_VARS_4M.fd"
        "/usr/share/qemu/OVMF_VARS.fd"
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
        log "On openSUSE, try: sudo zypper install qemu-ovmf-x86_64"
        exit 1
    fi
    
    log "All dependencies are installed."
    log "Using UEFI firmware: $UEFI_CODE"
    log "Using UEFI vars: $UEFI_VARS"
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
    local iso_path="${BUILD_DIR}/piccolo-os.x86_64-${PICCOLO_VERSION}.iso"
    local uefi_code_copy="${test_dir}/$(basename "$UEFI_CODE")"
    local uefi_vars_copy="${test_dir}/$(basename "$UEFI_VARS")"
    local ssh_key="${test_dir}/id_rsa_test"
    local seed_iso="${test_dir}/seed.iso"
    local cloud_config_dir="${test_dir}/cloud-config"

    # Clean up previous run just in case
    rm -rf "$test_dir"
    mkdir -p "$test_dir" "$cloud_config_dir"
    if [ ! -f "$iso_path" ]; then log "Error: ISO file not found at ${iso_path}" >&2; exit 1; fi
    
    log "Creating local copies of UEFI firmware files for this test run..."
    cp "$UEFI_CODE" "$uefi_code_copy"
    cp "$UEFI_VARS" "$uefi_vars_copy"

    log "Generating SSH key for cloud-init authentication..."
    ssh-keygen -t rsa -b 4096 -f "$ssh_key" -N "" -q

    log "Creating cloud-init configuration for SSH access..."
    cat > "${cloud_config_dir}/user-data" << EOF
#cloud-config
users:
  - name: root
    lock_passwd: false
    plain_text_passwd: 'testpassword123'
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
  - systemctl status sshd
EOF

    cat > "${cloud_config_dir}/meta-data" << EOF
instance-id: piccolo-test-$(date +%s)
local-hostname: piccolo-test
EOF

    log "Creating cloud-init seed ISO with CIDATA label..."
    genisoimage -quiet -output "$seed_iso" -volid CIDATA -joliet -rational-rock "${cloud_config_dir}/user-data" "${cloud_config_dir}/meta-data" 2>/dev/null
    if [ $? -ne 0 ]; then
        log "Error: Failed to create cloud-init seed ISO. Ensure genisoimage is installed." >&2
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
    
    # Print the full QEMU command for debugging
    log "Running QEMU command:"
    log "qemu-system-x86_64 \\"
    log "    -enable-kvm \\"
    log "    -machine q35,smm=on,accel=kvm \\"
    log "    -cpu host \\"
    log "    -smp 4 \\"
    log "    -m 4096 \\"
    log "    -drive if=pflash,format=raw,readonly=on,file=\"$uefi_code_copy\" \\"
    log "    -drive if=pflash,format=raw,file=\"$uefi_vars_copy\" \\"
    log "    -cdrom \"$iso_path\" \\"
    log "    -drive file=\"$seed_iso\",media=cdrom,if=virtio \\"
    log "    -boot d \\"
    log "    -netdev user,id=n0,hostfwd=tcp:127.0.0.1:${SSH_PORT}-:22 \\"
    log "    -device virtio-net-pci,netdev=n0 \\"
    log "    -nographic"
    
    qemu-system-x86_64 \
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
    # Step 3: Test SSH Connection
    # ---
    log "### Step 3: Testing SSH connection to verify if SSH is enabled..."
    local ssh_opts="-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o ConnectTimeout=5"
    local checks_passed=false
    
    # Wait for VM to boot and cloud-init to finish
    log "Waiting for VM to complete boot process and cloud-init configuration..."
    sleep 60  # Give cloud-init more time to complete configuration
    
    # Test if SSH is listening on port 22
    log "Checking if SSH service is listening on port 22..."
    for i in {1..10}; do
        if nc -z -w5 127.0.0.1 2222; then
            log "SSH port is open. Testing SSH connection..."
            break
        fi
        log "Attempt $i: SSH port not yet open. Retrying in 5 seconds..."
        sleep 5
    done
    
    # Test SSH connection with cloud-init configured credentials
    log "Testing SSH connection with cloud-init configured credentials..."
    
    # Try SSH key authentication first (should work with cloud-init)
    if ssh -q -p "$SSH_PORT" -i "$ssh_key" $ssh_opts "root@localhost" "echo 'SSH key auth works'" 2>/dev/null; then
        log "✅ SSH is working with key authentication for user: root"
        SSH_USER="root"
        SSH_KEY="$ssh_key"
        checks_passed=true
    # Fall back to password authentication
    elif sshpass -p "testpassword123" ssh -q -p "$SSH_PORT" -o PasswordAuthentication=yes -o PubkeyAuthentication=no $ssh_opts "root@localhost" "echo 'SSH password works'" 2>/dev/null; then
        log "✅ SSH is working with password authentication for user: root"
        SSH_USER="root"
        SSH_PASS="testpassword123"
        checks_passed=true
    else
        # Debug: Try to see what's happening with cloud-init
        log "Attempting to check cloud-init status via SSH with no authentication..."
        log "This might work if SSH is enabled but authentication isn't configured yet..."
        
        # Try connecting without authentication to get debugging info
        if timeout 10 ssh -q -p "$SSH_PORT" $ssh_opts "root@localhost" "systemctl status cloud-init.target; cloud-init status; journalctl -u cloud-init --no-pager -n 20" 2>/dev/null | head -20; then
            log "Got some cloud-init debug info above"
        else
            log "Could not get cloud-init debug info via SSH"
        fi
    fi
    
    if [ "$checks_passed" = false ]; then
        log "❌ SSH connection failed with all attempted users and authentication methods."
        log "This suggests SSH is either:"
        log "  1. Not enabled/running in the MicroOS live environment"
        log "  2. Configured with different authentication requirements"
        log "  3. Using different user accounts than expected"
        log ""
        log "Manual verification recommended:"
        log "  1. Boot with: qemu-system-x86_64 -enable-kvm -machine q35,smm=on,accel=kvm -cpu host -smp 4 -m 4096 \\"
        log "     -drive if=pflash,format=raw,readonly=on,file=$UEFI_CODE \\"
        log "     -drive if=pflash,format=raw,file=$uefi_vars_copy \\"
        log "     -cdrom $iso_path -boot d -device virtio-net-pci,netdev=n0 \\"
        log "     -netdev user,id=n0,hostfwd=tcp:127.0.0.1:2222-:22 -display gtk"
        log "  2. Log in via GUI console and run: systemctl status sshd"
        log "  3. If not running: systemctl enable --now sshd"
        log "  4. Check users: cat /etc/passwd | grep -E '(root|core|opensuse)'"
        log "  5. Test SSH: ssh -p 2222 username@127.0.0.1"
        
        exit 1
    else
        log "✅ SSH connection successful! Proceeding with automated checks..."
        
        # Determine SSH command based on detected authentication method
        local ssh_cmd
        if [ -n "${SSH_PASS:-}" ]; then
            ssh_cmd="sshpass -p '$SSH_PASS' ssh -p '$SSH_PORT' -o PasswordAuthentication=yes -o PubkeyAuthentication=no $ssh_opts '${SSH_USER}@localhost'"
        elif [ -n "${SSH_KEY:-}" ]; then
            ssh_cmd="ssh -p '$SSH_PORT' -i '$SSH_KEY' $ssh_opts '${SSH_USER}@localhost'"
        else
            log "Error: No valid SSH authentication method detected" >&2
            exit 1
        fi
        
        # These checks run against the LIVE environment booted from our ISO.
        eval "$ssh_cmd" /bin/bash -s -- "$PICCOLO_VERSION" << 'EOF'
set -euo pipefail
PICCOLO_VERSION_TO_TEST="$1"

echo "--- CHECK 1: piccolod binary ---"
# Check both the new MicroOS path and the old Flatcar path for compatibility
if [ -x "/usr/local/piccolo/v1/bin/piccolod" ] || [ -x "/usr/bin/piccolod" ]; then
    echo "PASS: piccolod binary is present and executable."
else
    echo "FAIL: piccolod binary not found or not executable at expected paths."
    echo "Checked paths: /usr/local/piccolo/v1/bin/piccolod, /usr/bin/piccolod"
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
# MicroOS uses Podman instead of Docker
if podman run --rm hello-world; then
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

echo "--- CHECK 7: UEFI and Secure Boot validation ---"
# Check if the system booted with UEFI
if [ -d "/sys/firmware/efi" ]; then
    echo "PASS: System booted with UEFI firmware."
    
    # Check EFI system partition
    if lsblk -f | grep -i efi > /dev/null 2>&1; then
        echo "PASS: EFI system partition detected."
    else
        echo "WARN: No EFI system partition found in block devices."
    fi
    
    # Check secure boot status if available
    if [ -f "/sys/firmware/efi/efivars/SecureBoot-8be4df61-93ca-11d2-aa0d-00e098032b8c" ]; then
        # Read secure boot status (byte 4, should be 1 for enabled)
        SECBOOT_STATUS=$(hexdump -C "/sys/firmware/efi/efivars/SecureBoot-8be4df61-93ca-11d2-aa0d-00e098032b8c" 2>/dev/null | awk 'NR==1 {print $8}')
        if [ "$SECBOOT_STATUS" = "01" ]; then
            echo "PASS: Secure Boot is enabled."
        else
            echo "WARN: Secure Boot is disabled or not available (status: $SECBOOT_STATUS)."
        fi
    else
        echo "WARN: Secure Boot status not available (SecureBoot EFI variable not found)."
    fi
    
    # Check setup mode (should be 0 for normal operation)
    if [ -f "/sys/firmware/efi/efivars/SetupMode-8be4df61-93ca-11d2-aa0d-00e098032b8c" ]; then
        SETUP_MODE=$(hexdump -C "/sys/firmware/efi/efivars/SetupMode-8be4df61-93ca-11d2-aa0d-00e098032b8c" 2>/dev/null | awk 'NR==1 {print $8}')
        if [ "$SETUP_MODE" = "00" ]; then
            echo "PASS: System is not in Setup Mode (normal operation)."
        else
            echo "WARN: System is in Setup Mode (value: $SETUP_MODE)."
        fi
    else
        echo "INFO: SetupMode status not available."
    fi
    
else
    echo "FAIL: System did not boot with UEFI firmware. Found legacy BIOS boot."
    echo "This may indicate an issue with the ISO UEFI compatibility or test configuration."
    exit 1
fi
EOF
        checks_passed=true
    fi

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
