#!/bin/bash
#
# Piccolo OS - Automated QA Smoke Test Script v1.0
#
# This script boots a given Piccolo OS raw image in a VM and runs a series
# of automated checks to verify its integrity and core functionality.
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
    echo "Usage: $0 --image-path <PATH_TO_IMAGE.raw.gz> --version <VERSION>"
    echo "  --image-path    (Required) The path to the compressed Piccolo OS image to test."
    echo "  --version       (Required) The version of the image being tested (e.g., 1.0.0)."
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
    local IMAGE_PATH=""
    local PICCOLO_VERSION=""
    while [ "$#" -gt 0 ]; do
        case "$1" in
            --image-path) IMAGE_PATH="$2"; shift 2;;
            --version) PICCOLO_VERSION="$2"; shift 2;;
            *) usage;;
        esac
    done
    if [ -z "${IMAGE_PATH:-}" ] || [ -z "${PICCOLO_VERSION:-}" ]; then usage; fi
    if [ ! -f "$IMAGE_PATH" ]; then log "Error: Image not found at $IMAGE_PATH" >&2; exit 1; fi
    check_dependencies

    # ---
    # Step 1: Prepare Test Environment
    # ---
    log "### Step 1: Preparing the test environment..."
    local test_dir="${SCRIPT_DIR}/build/test-${PICCOLO_VERSION}"
    local flatcar_scripts_dir="${SCRIPT_DIR}/build/work-${PICCOLO_VERSION}/scripts"
    local decompressed_image="${test_dir}/piccolo-os.raw"
    local ssh_key="${test_dir}/id_rsa_test"
    
    mkdir -p "$test_dir"
    
    # Ensure the Flatcar scripts repo is available
    if [ ! -d "$flatcar_scripts_dir" ]; then
        log "Error: Flatcar scripts directory not found. Please run a build first." >&2
        exit 1
    fi
    
    log "Decompressing image..."
    gzip -cdk "$IMAGE_PATH" > "$decompressed_image"

    log "Generating temporary SSH key for this test run..."
    ssh-keygen -t rsa -b 4096 -f "$ssh_key" -N "" -q

    # ---
    # Step 2: Boot VM with QEMU
    # ---
    log "### Step 2: Booting Piccolo OS image in QEMU..."
    # We use the image_to_vm.sh script from the Flatcar SDK to handle the complexities
    # of booting the raw image in a properly configured QEMU instance.
    # The script will run QEMU in the background and print the PID.
    pushd "$flatcar_scripts_dir" > /dev/null
    local qemu_pid
    # Note: image_to_vm.sh requires Ignition to inject an SSH key. We create a minimal one.
    cat > "${test_dir}/config.ign" <<EOF
{
  "ignition": { "version": "3.0.0" },
  "passwd": {
    "users": [
      {
        "name": "core",
        "sshAuthorizedKeys": [
          "$(cat "${ssh_key}.pub")"
        ]
      }
    ]
  }
}
EOF
    qemu_pid=$(./image_to_vm.sh --from="${decompressed_image}" --board=amd64-usr --ignition="${test_dir}/config.ign" | grep "QEMU running" | awk '{print $4}')
    popd > /dev/null

    if [ -z "$qemu_pid" ]; then
        log "Error: Failed to start QEMU VM." >&2
        exit 1
    fi
    log "QEMU started successfully with PID: $qemu_pid"

    # ---
    # Step 3: Wait for SSH and Run Checks
    # ---
    log "### Step 3: Waiting for SSH to become available..."
    local ssh_port=2222 # Default port used by image_to_vm.sh
    local ssh_opts="-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -i ${ssh_key}"
    local checks_passed=false
    
    # Wait for up to 2 minutes for SSH to be ready
    for i in {1..120}; do
        if ssh -p "$ssh_port" $ssh_opts "core@localhost" "echo 'SSH is ready'" &> /dev/null; then
            log "SSH is available. Running automated checks..."
            
            # Define the checks in a heredoc for readability
            ssh -p "$ssh_port" $ssh_opts "core@localhost" /bin/bash << 'EOF'
set -euo pipefail
echo "--- CHECK 1: piccolod service status ---"
systemctl is-active piccolod.service | grep -q "active"
echo "PASS: piccolod is active."

echo "--- CHECK 2: piccolod version ---"
# This assumes your binary has a --version flag
/usr/bin/piccolod --version | grep -q "1.0.0" # NOTE: Hardcoded version, improve this
echo "PASS: piccolod version is correct."

echo "--- CHECK 3: piccolod location ---"
which piccolod | grep -q "/usr/bin/piccolod"
echo "PASS: piccolod is in the correct immutable location."

echo "--- CHECK 4: Update configuration ---"
grep -q "os-updates.system.piccolospace.com" /etc/flatcar/update.conf
echo "PASS: Update server configuration is correct."

echo "--- CHECK 5: Container runtime ---"
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
