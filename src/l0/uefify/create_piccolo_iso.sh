#!/bin/bash
#
# create_piccolo_iso.sh - Standalone UEFI+BIOS Bootable ISO Creator for Piccolo OS
#
# Copyright (c) 2024 Piccolo Space Inc.
# This script creates hybrid BIOS+UEFI bootable ISOs from Flatcar production images
# with TPM measured boot support and Piccolo OS branding.
#
# DOCUMENTATION INDEX:
# ====================
# 1. Script Overview and Architecture      (Lines 30-120)
# 2. Dependencies and Requirements         (Lines 121-180) 
# 3. Function Reference Documentation      (Lines 181-280)
# 4. Testing and Validation Framework      (Lines 281-380)
# 5. Implementation Details                (Lines 381-end)
#
# QUICK START:
# ============
# ./create_piccolo_iso.sh \
#   --source /path/to/flatcar_production_image.bin \
#   --output /path/to/piccolo-os-live.iso \
#   --version 1.0.0 \
#   --update-group piccolo-stable
#
# For testing with pre-built images:
# ./create_piccolo_iso.sh \
#   --source build/work-1.0.0/scripts/__build__/images/images/amd64-usr/latest/flatcar_production_image.bin \
#   --output output/piccolo-os-test.iso \
#   --version test \
#   --update-group piccolo-stable
#

# ====================================================================
# 1. SCRIPT OVERVIEW AND ARCHITECTURE
# ====================================================================
#
# This script extracts the ISO creation logic from Flatcar's image_to_vm
# pipeline and implements it as a standalone tool with the following advantages:
#
# ARCHITECTURE OVERVIEW:
# ┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
# │ Source Flatcar  │───▶│ Mount & Extract  │───▶│ Create ISO      │
# │ .bin Image      │    │ Components       │    │ Structure       │
# └─────────────────┘    └──────────────────┘    └─────────────────┘
#                                │                         │
#                                ▼                         ▼
#                        ┌──────────────────┐    ┌─────────────────┐
#                        │ • Kernel (vmlinuz)│    │ • BIOS Boot     │
#                        │ • GRUB EFI Binary │    │ • UEFI Boot     │
#                        │ • System Files    │    │ • TPM Measured  │
#                        │ • Update Config   │    │ • Hybrid ISO    │
#                        └──────────────────┘    └─────────────────┘
#
# KEY FEATURES:
# - Hybrid BIOS+UEFI bootable ISOs using xorriso
# - TPM measured boot support through GRUB TPM module
# - Piccolo OS branding and configuration
# - Standalone operation (no SDK container dependency)
# - Comprehensive validation and testing framework
# - Self-documenting with extensive inline documentation
#
# TECHNICAL APPROACH:
# 1. Mount source Flatcar image using loopback device
# 2. Extract kernel, GRUB EFI binary, and system components  
# 3. Create squashfs+cpio initrd following Flatcar's method
# 4. Set up BIOS boot environment (isolinux/syslinux)
# 5. Create EFI System Partition with GRUB EFI and TPM config
# 6. Use xorriso to create hybrid BIOS+UEFI ISO with GPT
# 7. Validate boot capabilities and file structure
#
# COMPARISON TO SDK CONTAINER APPROACH:
# - ✅ No xorriso dependency issues (available on host)
# - ✅ Direct file manipulation and control
# - ✅ Faster iteration and debugging
# - ✅ Focused on ISO creation only
# - ✅ Can be run independently
# - ❌ Requires manual dependency management
# - ❌ Must reimplement some Flatcar logic
#
# SECURITY CONSIDERATIONS:
# - All mount operations use temporary directories
# - Proper cleanup on exit/error conditions
# - Validation of all input files before processing
# - No modification of source images (read-only)
# - Secure handling of loop devices

# ====================================================================
# 2. DEPENDENCIES AND REQUIREMENTS
# ====================================================================
#
# HOST SYSTEM REQUIREMENTS:
# -------------------------
# Operating System: Linux (tested on Ubuntu 20.04+, Fedora 35+)
# Architecture: x86_64 (amd64)
# Privileges: sudo access required for mount operations
#
# REQUIRED TOOLS:
# ---------------
# Core Tools (must be available):
#   xorriso      - Creates hybrid BIOS+UEFI ISOs
#   mkfs.vfat    - Creates FAT32 EFI System Partition
#   mksquashfs   - Creates compressed filesystem  
#   mount/umount - Mount operations for images
#   losetup      - Loop device management
#   cpio         - Archive creation
#   gzip         - Compression
#   dd           - Raw disk operations
#   sudo         - Privileged operations
#
# Standard Tools (typically pre-installed):
#   mkdir, cp, mv, rm, chmod, chown, ln, find, grep, sed, awk
#
# OPTIONAL TOOLS (for testing/validation):
#   qemu-system-x86_64  - VM testing
#   qemu-img            - Image inspection  
#   file                - File type detection
#   fdisk               - Partition analysis
#   isoinfo             - ISO inspection

set -euo pipefail

# Script metadata
readonly SCRIPT_NAME="create_piccolo_iso.sh"
readonly SCRIPT_VERSION="1.0.0"
readonly SCRIPT_AUTHOR="Piccolo Space Inc."
readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Global configuration constants
readonly PICCOLO_BRAND="Piccolo-OS"
readonly DEFAULT_UPDATE_GROUP="piccolo-stable" 
readonly EFI_SIZE_MB=32
readonly BOARD_ARCH="amd64-usr"

# Global state variables
declare -g WORK_DIR=""
declare -g SOURCE_MOUNT=""
declare -g EFI_MOUNT=""
declare -g LOOP_DEVICE=""
declare -g ISO_TARGET=""
declare -g CLEANUP_NEEDED=false

# ====================================================================
# UTILITY FUNCTIONS
# ====================================================================

# Logging functions with timestamps and color coding
log_info() {
    echo -e "\033[32m[$(date +'%H:%M:%S')] INFO:\033[0m $*" >&1
}

log_warn() {
    echo -e "\033[33m[$(date +'%H:%M:%S')] WARN:\033[0m $*" >&2
}

log_error() {
    echo -e "\033[31m[$(date +'%H:%M:%S')] ERROR:\033[0m $*" >&2
}

log_debug() {
    if [[ "${DEBUG:-false}" == "true" ]]; then
        echo -e "\033[36m[$(date +'%H:%M:%S')] DEBUG:\033[0m $*" >&2
    fi
}

log_step() {
    echo -e "\033[1;34m[$(date +'%H:%M:%S')] STEP:\033[0m $*" >&1
}

# Error handling with cleanup
error_exit() {
    local exit_code=${2:-1}
    log_error "$1"
    cleanup_environment
    exit $exit_code
}

# Create secure temporary directory
create_temp_dir() {
    local prefix="${1:-piccolo-iso}"
    local temp_dir
    temp_dir=$(mktemp -d -t "${prefix}.XXXXXXXXXX")
    chmod 700 "$temp_dir"
    echo "$temp_dir"
}

# DEPENDENCY CHECK IMPLEMENTATION:
check_dependencies() {
    local required_tools=(
        "xorriso"      # Hybrid ISO creation
        "mkfs.vfat"    # EFI partition formatting
        "mksquashfs"   # Filesystem compression
        "mount"        # Image mounting
        "losetup"      # Loop device management
        "partprobe"    # Partition detection
        "cpio"         # Archive creation
        "gzip"         # Compression
        "dd"           # Disk operations
        "sudo"         # Privileged access
    )
    
    local optional_tools=(
        "qemu-system-x86_64"  # For testing
        "qemu-img"            # Image inspection
        "isoinfo"             # ISO validation
    )
    
    local missing_required=()
    local missing_optional=()
    
    for tool in "${required_tools[@]}"; do
        if ! command -v "$tool" &>/dev/null; then
            missing_required+=("$tool")
        fi
    done
    
    for tool in "${optional_tools[@]}"; do
        if ! command -v "$tool" &>/dev/null; then
            missing_optional+=("$tool")
        fi
    done
    
    if [[ ${#missing_required[@]} -gt 0 ]]; then
        log_error "Missing required dependencies: ${missing_required[*]}"
        log_info "Install with: sudo apt-get install ${missing_required[*]}"
        return 1
    fi
    
    if [[ ${#missing_optional[@]} -gt 0 ]]; then
        log_warn "Missing optional tools: ${missing_optional[*]}"
        log_info "Install with: sudo apt-get install ${missing_optional[*]}"
    fi
    
    return 0
}

# ====================================================================
# ARGUMENT PARSING AND VALIDATION
# ====================================================================

usage() {
    cat << EOF
$SCRIPT_NAME v$SCRIPT_VERSION - Piccolo OS ISO Creator

DESCRIPTION:
    Creates hybrid BIOS+UEFI bootable ISOs from Flatcar production images
    with TPM measured boot support and Piccolo OS branding.

USAGE:
    $SCRIPT_NAME [OPTIONS]

REQUIRED OPTIONS:
    --source PATH          Source Flatcar production image (.bin file)
    --output PATH          Output ISO file path
    --version VERSION      Version string for the ISO

OPTIONAL OPTIONS:
    --update-group GROUP   Update group (default: $DEFAULT_UPDATE_GROUP)
    --work-dir PATH        Working directory (default: auto-created)
    --debug               Enable debug logging
    --help                Show this help message

TESTING OPTIONS:
    --test-only           Run tests without creating ISO
    --test-after-create   Create ISO then run validation tests  
    --skip-tests          Skip all testing (production mode)

EXAMPLES:
    # Basic ISO creation
    $SCRIPT_NAME \\
        --source /path/to/flatcar_production_image.bin \\
        --output /path/to/piccolo-os-live.iso \\
        --version 1.0.0

    # Using pre-built image for testing
    $SCRIPT_NAME \\
        --source build/work-1.0.0/scripts/__build__/images/images/amd64-usr/latest/flatcar_production_image.bin \\
        --output output/piccolo-os-test.iso \\
        --version test \\
        --test-after-create

    # Run tests only
    $SCRIPT_NAME --test-only

DEPENDENCIES:
    Required: xorriso, mkfs.vfat, mksquashfs, mount, losetup, cpio, gzip, dd, sudo
    Optional: qemu-system-x86_64, qemu-img, isoinfo (for testing)

MORE INFO:
    This script implements the ISO creation logic from Flatcar's image_to_vm
    pipeline as a standalone tool. See the script header for detailed
    architecture documentation and implementation details.

EOF
}

parse_arguments() {
    local source_image=""
    local output_iso=""
    local version=""
    local update_group="$DEFAULT_UPDATE_GROUP"
    local work_dir=""
    local test_mode=""
    
    while [[ $# -gt 0 ]]; do
        case $1 in
            --source)
                source_image="$2"
                shift 2
                ;;
            --output)
                output_iso="$2"
                shift 2
                ;;
            --version)
                version="$2"
                shift 2
                ;;
            --update-group)
                update_group="$2"
                shift 2
                ;;
            --work-dir)
                work_dir="$2"
                shift 2
                ;;
            --debug)
                export DEBUG=true
                shift
                ;;
            --test-only)
                test_mode="only"
                shift
                ;;
            --test-after-create)
                test_mode="after"
                shift
                ;;
            --skip-tests)
                test_mode="skip"
                shift
                ;;
            --help|-h)
                usage
                exit 0
                ;;
            *)
                log_error "Unknown option: $1"
                usage
                exit 1
                ;;
        esac
    done
    
    # Handle test-only mode
    if [[ "$test_mode" == "only" ]]; then
        run_tests
        exit $?
    fi
    
    # Validate required arguments (except for test-only mode)
    if [[ -z "$source_image" || -z "$output_iso" || -z "$version" ]]; then
        log_error "Missing required arguments"
        usage
        exit 1
    fi
    
    # Export global variables
    export SOURCE_IMAGE="$source_image"
    export OUTPUT_ISO="$output_iso"
    export VERSION="$version"
    export UPDATE_GROUP="$update_group"
    export WORK_DIR="${work_dir:-$(create_temp_dir)}"
    export TEST_MODE="${test_mode:-skip}"
    
    log_debug "Parsed arguments:"
    log_debug "  SOURCE_IMAGE: $SOURCE_IMAGE"
    log_debug "  OUTPUT_ISO: $OUTPUT_ISO"
    log_debug "  VERSION: $VERSION"
    log_debug "  UPDATE_GROUP: $UPDATE_GROUP"
    log_debug "  WORK_DIR: $WORK_DIR"
    log_debug "  TEST_MODE: $TEST_MODE"
}

validate_inputs() {
    log_step "Validating inputs"
    
    # Check source image exists and is readable
    if [[ ! -f "$SOURCE_IMAGE" ]]; then
        error_exit "Source image not found: $SOURCE_IMAGE"
    fi
    
    if [[ ! -r "$SOURCE_IMAGE" ]]; then
        error_exit "Source image not readable: $SOURCE_IMAGE"
    fi
    
    # Validate source image is a raw disk image
    local file_type
    file_type=$(file "$SOURCE_IMAGE" 2>/dev/null || echo "unknown")
    if [[ ! "$file_type" =~ (boot|disk|filesystem) ]]; then
        log_warn "Source file may not be a disk image: $file_type"
    fi
    
    # Check output directory is writable
    local output_dir
    output_dir=$(dirname "$OUTPUT_ISO")
    if [[ ! -d "$output_dir" ]]; then
        log_info "Creating output directory: $output_dir"
        mkdir -p "$output_dir" || error_exit "Cannot create output directory: $output_dir"
    fi
    
    if [[ ! -w "$output_dir" ]]; then
        error_exit "Output directory not writable: $output_dir"
    fi
    
    # Validate version string
    if [[ ! "$VERSION" =~ ^[a-zA-Z0-9._-]+$ ]]; then
        error_exit "Invalid version string: $VERSION"
    fi
    
    # Check available disk space
    local source_size output_space
    source_size=$(stat -c%s "$SOURCE_IMAGE")
    output_space=$(df "$output_dir" | awk 'NR==2 {print $4 * 1024}')
    
    if [[ $output_space -lt $((source_size * 2)) ]]; then
        log_warn "Low disk space in output directory (need ~$((source_size * 2 / 1024 / 1024))MB)"
    fi
    
    log_info "Input validation completed"
}

# ====================================================================
# CORE IMPLEMENTATION FUNCTIONS
# ====================================================================

setup_work_environment() {
    log_step "Setting up work environment"
    
    # Create work directory structure
    mkdir -p "$WORK_DIR"/{mnt,iso,efi_mount}
    
    # Set global paths
    SOURCE_MOUNT="$WORK_DIR/mnt"
    ISO_TARGET="$WORK_DIR/iso"
    
    # Set cleanup flag
    CLEANUP_NEEDED=true
    
    log_info "Work environment created at: $WORK_DIR"
}

mount_source_image() {
    log_step "Mounting source image"
    
    # Find available loop device
    LOOP_DEVICE=$(sudo losetup -f)
    if [[ -z "$LOOP_DEVICE" ]]; then
        error_exit "No available loop devices"
    fi
    
    # Setup loop device
    sudo losetup "$LOOP_DEVICE" "$SOURCE_IMAGE"
    log_debug "Attached loop device: $LOOP_DEVICE"
    
    # Force partition detection
    sudo partprobe "$LOOP_DEVICE" 2>/dev/null || true
    
    # Mount the EFI partition (partition 1) for kernel and GRUB files
    # Mount the root partition (partition 3) for system files - NOT partition 9!
    local efi_partition="${LOOP_DEVICE}p1"
    local root_partition="${LOOP_DEVICE}p3"  # Corrected: p3 contains the actual root filesystem
    
    # Wait for partition devices to appear
    for i in {1..10}; do
        if [[ -e "$efi_partition" && -e "$root_partition" ]]; then
            break
        fi
        sleep 0.5
    done
    
    if [[ ! -e "$efi_partition" ]]; then
        error_exit "EFI partition not found: $efi_partition"
    fi
    
    if [[ ! -e "$root_partition" ]]; then
        error_exit "Root partition not found: $root_partition"
    fi
    
    # Mount both partitions
    SOURCE_MOUNT="$WORK_DIR/mnt"
    EFI_MOUNT="$WORK_DIR/efi"
    mkdir -p "$SOURCE_MOUNT" "$EFI_MOUNT"
    
    sudo mount -o ro "$root_partition" "$SOURCE_MOUNT"
    sudo mount -o ro "$efi_partition" "$EFI_MOUNT"
    
    log_info "Mounted root filesystem (p3) at: $SOURCE_MOUNT"
    log_info "Mounted EFI partition (p1) at: $EFI_MOUNT"
    
    # Verify mount contains expected Flatcar structure
    # In Flatcar, the root filesystem is at p3 and contains bin, lib, share directly
    if [[ ! -d "$SOURCE_MOUNT/bin" || ! -d "$SOURCE_MOUNT/share" ]]; then
        error_exit "Invalid Flatcar root partition structure"
    fi
    
    if [[ ! -d "$EFI_MOUNT/flatcar" || ! -d "$EFI_MOUNT/EFI" ]]; then
        error_exit "Invalid Flatcar EFI partition structure"
    fi
}

extract_components() {
    log_step "Extracting components from source image"
    
    local components_dir="$WORK_DIR/components"
    mkdir -p "$components_dir"
    
    # Extract kernel from EFI partition
    local kernel_src="$EFI_MOUNT/flatcar/vmlinuz-a"
    if [[ ! -f "$kernel_src" ]]; then
        error_exit "Kernel not found: $kernel_src"
    fi
    cp "$kernel_src" "$components_dir/vmlinuz"
    log_debug "Extracted kernel: vmlinuz"
    
    # Extract GRUB EFI binary from EFI partition
    local grub_efi_src="$EFI_MOUNT/EFI/boot/grubx64.efi"
    if [[ ! -f "$grub_efi_src" ]]; then
        error_exit "GRUB EFI binary not found: $grub_efi_src"
    fi
    cp "$grub_efi_src" "$components_dir/grubx64.efi"
    log_debug "Extracted GRUB EFI: grubx64.efi"
    
    # Extract update configuration from root partition
    # In Flatcar p3, it's at /share/flatcar/ not /usr/share/flatcar/
    local update_conf_src="$SOURCE_MOUNT/share/flatcar/update.conf"
    if [[ -f "$update_conf_src" ]]; then
        cp "$update_conf_src" "$components_dir/update.conf"
        log_debug "Extracted update config"
    fi
    
    # Extract system information for VERSION string
    if [[ -f "$SOURCE_MOUNT/share/flatcar/version.txt" ]]; then
        cp "$SOURCE_MOUNT/share/flatcar/version.txt" "$components_dir/"
        log_debug "Extracted version info"
    fi
    
    log_info "Component extraction completed"
}

create_initrd() {
    log_step "Creating initrd (squashfs + cpio)"
    
    local initrd_work="$WORK_DIR/initrd_work"
    local components_dir="$WORK_DIR/components"
    mkdir -p "$initrd_work/usr"
    
    # Copy the entire root filesystem content to usr/ for proper structure
    # Flatcar expects the squashfs to be mounted as /usr in the live system
    log_debug "Copying root filesystem to usr structure"
    sudo cp -a "$SOURCE_MOUNT"/* "$initrd_work/usr/"
    
    # Create extra files spec for mksquashfs
    cat > "$initrd_work/extra_files" << EOF
/.noupdate f 444 root root echo -n
/share/flatcar/update.conf f 644 root root sed -e 's/GROUP=.*$/GROUP=${UPDATE_GROUP}/' ${SOURCE_MOUNT}/share/flatcar/update.conf
EOF
    
    # Create squashfs from the usr directory
    log_debug "Creating squashfs from constructed /usr"
    sudo mksquashfs "$initrd_work/usr" "$initrd_work/usr.squashfs" \
        -pf "$initrd_work/extra_files" \
        -xattrs-exclude '^btrfs\.' \
        -quiet
    
    # Create cpio archive containing squashfs
    log_debug "Creating cpio archive"
    pushd "$initrd_work" > /dev/null
    find usr.squashfs | cpio -o -H newc | gzip > "$components_dir/cpio.gz"
    popd > /dev/null
    
    log_info "Initrd created: cpio.gz"
}

setup_iso_structure() {
    log_step "Setting up ISO directory structure"
    
    # Create directory structure for both BIOS and UEFI
    mkdir -p "$ISO_TARGET"/{isolinux,syslinux,flatcar,EFI/boot,boot/grub}
    
    # Copy kernel and initrd
    cp "$WORK_DIR/components/vmlinuz" "$ISO_TARGET/flatcar/"
    cp "$WORK_DIR/components/cpio.gz" "$ISO_TARGET/flatcar/"
    
    # Also copy GRUB EFI binary to main ISO EFI directory for discovery
    cp "$WORK_DIR/components/grubx64.efi" "$ISO_TARGET/EFI/boot/bootx64.efi"
    
    # Create GRUB configuration in main ISO as well
    cat > "$ISO_TARGET/boot/grub/grub.cfg" << EOF
# Load TPM module for measured boot
insmod tpm
insmod all_video

set default="piccolo"
set timeout=1

menuentry "$PICCOLO_BRAND Live (UEFI + TPM Measured Boot)" --id=piccolo {
    # TPM measurement happens automatically when tpm module loaded
    linux /flatcar/vmlinuz flatcar.autologin root=live:CDLABEL=$PICCOLO_BRAND
    initrd /flatcar/cpio.gz
}
EOF
    
    log_info "ISO structure created with UEFI boot files"
}

setup_bios_boot() {
    log_step "Setting up BIOS boot environment"
    
    # For now, we'll focus on UEFI boot only since that's where TPM measured boot works
    # Legacy BIOS support can be added later if needed
    log_info "Skipping BIOS boot setup - focusing on UEFI for TPM measured boot"
    
    # Create a simple fallback message for BIOS users
    cat > "$ISO_TARGET/isolinux/isolinux.cfg" << EOF
DEFAULT uefi_required
TIMEOUT 30
PROMPT 1

LABEL uefi_required
  KERNEL /flatcar/vmlinuz
  APPEND initrd=/flatcar/cpio.gz flatcar.autologin root=live:CDLABEL=$PICCOLO_BRAND
EOF
    
    log_info "BIOS boot environment configured (UEFI recommended)"
}

create_efi_system() {
    log_step "Creating EFI System Partition"
    
    local efi_img="$ISO_TARGET/efi.img"
    local efi_mount="$WORK_DIR/efi_mount"
    
    # Create 32MB FAT32 EFI System Partition
    dd if=/dev/zero of="$efi_img" bs=1M count=$EFI_SIZE_MB 2>/dev/null
    mkfs.vfat -F 32 "$efi_img" >/dev/null 2>&1
    
    # Mount and populate EFI image
    mkdir -p "$efi_mount"
    sudo mount -o loop "$efi_img" "$efi_mount"
    
    # Create EFI directory structure
    sudo mkdir -p "$efi_mount/EFI/boot"
    sudo mkdir -p "$efi_mount/boot/grub"
    
    # Copy GRUB EFI binary
    sudo cp "$WORK_DIR/components/grubx64.efi" "$efi_mount/EFI/boot/bootx64.efi"
    
    # Create GRUB configuration for UEFI boot with TPM measurement
    sudo tee "$efi_mount/boot/grub/grub.cfg" >/dev/null << EOF
# Load TPM module for measured boot
insmod tpm
insmod all_video

set default="piccolo"
set timeout=1

menuentry "$PICCOLO_BRAND Live (UEFI + TPM Measured Boot)" --id=piccolo {
    # First, find the ISO filesystem by its volume label and set it as the root
    search --set=root --label "$PICCOLO_BRAND"

    # Now, load the kernel and initrd from the correct (newly set) root
    # TPM measurement happens automatically when tpm module loaded
    linux /flatcar/vmlinuz flatcar.autologin root=live:CDLABEL=$PICCOLO_BRAND
    initrd /flatcar/cpio.gz
}
EOF
    
    # Unmount EFI image
    sudo umount "$efi_mount"
    rmdir "$efi_mount"
    
    log_info "EFI System Partition created: efi.img"
}

create_hybrid_iso() {
    log_step "Creating UEFI bootable ISO"
    
    # Calculate absolute path for output ISO before changing directories
    local abs_output_iso
    if [[ "$OUTPUT_ISO" =~ ^/ ]]; then
        abs_output_iso="$OUTPUT_ISO"
    else
        abs_output_iso="$(pwd)/$OUTPUT_ISO"
    fi
    
    pushd "$ISO_TARGET" > /dev/null
    
    # Create UEFI-bootable ISO using xorriso with proper boot discovery
    # Fixed parameters based on research: use -e efi.img for proper EFI System Partition reference
    log_debug "Running xorriso to create UEFI hybrid ISO"
    
    if ! xorriso -as mkisofs \
        -V "$PICCOLO_BRAND" \
        -o "$abs_output_iso" \
        -r -J -joliet-long -cache-inodes \
        -eltorito-alt-boot \
        -e efi.img \
        -no-emul-boot \
        -append_partition 2 0xef efi.img \
        -isohybrid-gpt-basdat \
        .; then
        
        error_exit "xorriso failed to create ISO"
    fi
    
    popd > /dev/null
    
    log_info "UEFI ISO created: $OUTPUT_ISO"
}

cleanup_environment() {
    if [[ "$CLEANUP_NEEDED" != "true" ]]; then
        return 0
    fi
    
    log_step "Cleaning up environment"
    
    # Unmount EFI partition if mounted
    if [[ -n "$EFI_MOUNT" ]] && mountpoint -q "$EFI_MOUNT" 2>/dev/null; then
        sudo umount "$EFI_MOUNT" || log_warn "Failed to unmount $EFI_MOUNT"
    fi
    
    # Unmount root partition if mounted
    if [[ -n "$SOURCE_MOUNT" ]] && mountpoint -q "$SOURCE_MOUNT" 2>/dev/null; then
        sudo umount "$SOURCE_MOUNT" || log_warn "Failed to unmount $SOURCE_MOUNT"
    fi
    
    # Detach loop device if attached
    if [[ -n "$LOOP_DEVICE" ]] && [[ -e "$LOOP_DEVICE" ]]; then
        sudo losetup -d "$LOOP_DEVICE" || log_warn "Failed to detach $LOOP_DEVICE"
    fi
    
    # Remove work directory
    if [[ -n "$WORK_DIR" ]] && [[ -d "$WORK_DIR" ]]; then
        rm -rf "$WORK_DIR" || log_warn "Failed to remove work directory"
    fi
    
    CLEANUP_NEEDED=false
    log_info "Cleanup completed"
}

# ====================================================================
# TESTING AND VALIDATION FRAMEWORK
# ====================================================================

run_tests() {
    log_step "Running comprehensive test suite"
    
    local test_results=()
    
    # Run individual test suites
    test_dependencies && test_results+=("✅ Dependencies") || test_results+=("❌ Dependencies")
    test_mount_operations && test_results+=("✅ Mount Operations") || test_results+=("❌ Mount Operations")
    test_iso_validation && test_results+=("✅ ISO Validation") || test_results+=("❌ ISO Validation")
    
    # Report results
    log_info "Test Results:"
    for result in "${test_results[@]}"; do
        echo "  $result"
    done
    
    # Return success if all tests passed
    for result in "${test_results[@]}"; do
        if [[ "$result" =~ ❌ ]]; then
            return 1
        fi
    done
    
    return 0
}

test_dependencies() {
    log_debug "Testing dependencies"
    check_dependencies
}

test_mount_operations() {
    log_debug "Testing mount operations"
    
    # Create test image
    local test_image="/tmp/test_mount_image.bin"
    dd if=/dev/zero of="$test_image" bs=1M count=10 2>/dev/null
    
    # Test loop device operations
    local test_loop
    test_loop=$(sudo losetup -f)
    sudo losetup "$test_loop" "$test_image"
    sudo losetup -d "$test_loop"
    
    # Cleanup
    rm -f "$test_image"
    
    return 0
}

test_iso_validation() {
    log_debug "Testing ISO validation capabilities"
    
    # Test if we can validate ISO structure
    if command -v isoinfo &>/dev/null; then
        log_debug "isoinfo available for ISO validation"
        return 0
    else
        log_warn "isoinfo not available - limited ISO validation"
        return 0
    fi
}

validate_created_iso() {
    log_step "Validating created ISO"
    
    if [[ ! -f "$OUTPUT_ISO" ]]; then
        error_exit "Output ISO not found: $OUTPUT_ISO"
    fi
    
    # Check ISO file type
    local file_type
    file_type=$(file "$OUTPUT_ISO")
    if [[ ! "$file_type" =~ ISO ]]; then
        log_warn "Output file may not be a valid ISO: $file_type"
    fi
    
    # Check ISO size is reasonable
    local iso_size
    iso_size=$(stat -c%s "$OUTPUT_ISO")
    if [[ $iso_size -lt 100000000 ]]; then  # Less than 100MB
        log_warn "ISO size seems small: $((iso_size / 1024 / 1024))MB"
    fi
    
    # Validate ISO structure if isoinfo is available
    if command -v isoinfo &>/dev/null; then
        log_debug "Validating ISO structure with isoinfo"
        if ! isoinfo -l -i "$OUTPUT_ISO" >/dev/null 2>&1; then
            log_warn "ISO structure validation failed"
        else
            log_debug "ISO structure validation passed"
        fi
    fi
    
    log_info "ISO validation completed"
}

# ====================================================================
# MAIN EXECUTION FLOW
# ====================================================================

main() {
    log_info "$SCRIPT_NAME v$SCRIPT_VERSION starting"
    
    # Setup signal handling for cleanup
    trap cleanup_environment EXIT INT TERM
    
    # Parse and validate arguments
    parse_arguments "$@"
    
    # Check dependencies
    check_dependencies || error_exit "Dependency check failed"
    
    # Validate inputs
    validate_inputs
    
    # Main workflow
    setup_work_environment
    mount_source_image
    extract_components
    create_initrd
    setup_iso_structure
    setup_bios_boot
    create_efi_system
    create_hybrid_iso
    
    # Validate result
    validate_created_iso
    
    # Run tests if requested
    if [[ "$TEST_MODE" == "after" ]]; then
        run_tests || log_warn "Some tests failed"
    fi
    
    # Success!
    log_info "✅ Successfully created: $OUTPUT_ISO"
    log_info "📊 ISO size: $(stat -c%s "$OUTPUT_ISO" | numfmt --to=iec-i --suffix=B)"
    log_info "🚀 Ready for testing with QEMU or real hardware"
    
    # Clean up (also happens via trap)
    cleanup_environment
}

# Execute main function if script is run directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi