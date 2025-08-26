#!/bin/bash
#
# Piccolo Common Utilities v1.0
#
# Shared utility functions for Piccolo OS VM testing and debugging
# Used by: test_piccolo_os_image.sh, ssh_into_piccolo.sh
#

# ---
# Logging Functions
# ---
log() {
    echo "[$(date +'%Y-%m-%d %H:%M:%S')] $*"
}

# ---
# UEFI Firmware Detection
# ---
detect_uefi_firmware() {
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
        return 1
    fi
    
    log "Using UEFI firmware: $UEFI_CODE"
    log "Using UEFI vars: $UEFI_VARS"
    return 0
}

# ---
# Common Dependency Checking
# ---
check_vm_dependencies() {
    log "Checking for required dependencies..."
    local deps=("qemu-system-x86_64" "ssh" "ssh-keygen" "ss" "genisoimage")
    for dep in "${deps[@]}"; do
        if ! command -v "$dep" &> /dev/null; then
            log "Error: Required dependency '$dep' is not installed." >&2
            log "On Debian/Ubuntu, try: sudo apt-get install -y qemu-system-x86 openssh-client iproute2 genisoimage"
            return 1
        fi
    done
    
    if ! detect_uefi_firmware; then
        return 1
    fi
    
    log "All dependencies are installed."
    return 0
}

check_test_dependencies() {
    log "Checking for required dependencies..."
    local deps=("qemu-system-x86_64" "ssh" "ssh-keygen" "ss" "nc" "sshpass" "genisoimage")
    for dep in "${deps[@]}"; do
        if ! command -v "$dep" &> /dev/null; then
            log "Error: Required dependency '$dep' is not installed." >&2
            log "On Debian/Ubuntu, try: sudo apt-get install -y qemu-system-x86 openssh-client iproute2 netcat-openbsd sshpass genisoimage"
            return 1
        fi
    done
    
    if ! detect_uefi_firmware; then
        return 1
    fi
    
    log "All dependencies are installed."
    return 0
}

# ---
# Cloud-init Configuration Generation
# ---
generate_cloud_init_user_data() {
    local ssh_public_key="$1"
    local cloud_config_dir="$2"
    local password="${3:-piccolo123}"
    local extra_commands="${4:-}"
    
    cat > "${cloud_config_dir}/user-data" << EOF
#cloud-config
users:
  - name: root
    lock_passwd: false
    plain_text_passwd: '$password'
    ssh_authorized_keys:
      - $ssh_public_key

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
  - systemctl reload sshd${extra_commands:+
  - $extra_commands}
EOF
}

generate_cloud_init_meta_data() {
    local cloud_config_dir="$1"
    local script_name="$2"
    
    cat > "${cloud_config_dir}/meta-data" << EOF
instance-id: piccolo-$(basename "$script_name")-$(date +%s)
local-hostname: piccolo-test
EOF
}

# ---
# SSH Key Management
# ---
generate_temp_ssh_key() {
    local ssh_key_path="$1"
    
    log "Generating temporary SSH key for this session..."
    ssh-keygen -t rsa -b 4096 -f "$ssh_key_path" -N "" -q
}

# ---
# Common Cleanup Functions
# ---
cleanup_qemu_process() {
    local qemu_pid="$1"
    
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

        # If it's still alive, force kill it
        if ps -p "$qemu_pid" > /dev/null; then
            log "QEMU did not shut down gracefully. Forcing termination (kill -9)..."
            kill -9 "$qemu_pid" 2>/dev/null
        fi
        log "QEMU process terminated."
    fi
}

cleanup_test_directory() {
    local test_dir="$1"
    
    if [ -d "${test_dir:-}" ]; then
        rm -rf "$test_dir"
        log "Test directory cleaned up."
    fi
}

# ---
# ISO Path Resolution
# ---
resolve_piccolo_iso_path() {
    local build_dir="$1"
    local version="$2"
    
    echo "${build_dir}/piccolo-os.x86_64-${version}.iso"
}

# ---
# Common validation functions
# ---
validate_build_directory() {
    local build_dir="$1"
    
    if [ ! -d "$build_dir" ]; then
        log "Error: Build directory not found at $build_dir" >&2
        return 1
    fi
}

validate_iso_file() {
    local iso_path="$1"
    
    if [ ! -f "$iso_path" ]; then
        log "Error: ISO file not found at ${iso_path}" >&2
        return 1
    fi
}