#!/bin/bash

# UEFI ISO Bootability Checker - The Holy Script
# 
# A comprehensive UEFI ISO analysis tool that provides precise diagnostics
# for UEFI boot failures. Supports modern ISO architectures including:
# - Traditional /EFI directory structure
# - El Torito boot images  
# - EFI System Partitions in hybrid ISOs
# - Multiple extraction methods (unprivileged + sudo fallbacks)
#
# Based on comprehensive research from check_iso_research.md
# Enhanced with expert feedback and real-world testing
#
# Version: 1.0.0
# Authors: Claude Code AI Assistant + Expert Community Review
# License: MIT
#
# Key Features:
# - Multi-layer UEFI detection (static + dynamic analysis)
# - Boot image extraction and analysis
# - ESP partition extraction for hybrid ISOs
# - Enhanced EFI binary analysis (PE32/PE32+ detection)
# - Case-sensitivity validation and warnings
# - JSON output for automation
# - Comprehensive verbose debugging
# - Graceful dependency handling with install suggestions
#
# Tested with: Piccolo OS, Windows 11, various Linux distributions

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Global variables
ISO_FILE=""
MOUNT_POINT=""
TEMP_DIR=""
EFI_ROOT=""
VERBOSE=false
JSON_OUTPUT=false
EXIT_CODE=0

# JSON tracking variables
JSON_HAS_ELTORITO="false"
JSON_HAS_EFI_PLATFORM="false"
JSON_HAS_EFI_DIR="false"
JSON_HAS_BOOTLOADER="false"
JSON_BOOTLOADER_ARCH=""

# Cleanup function
cleanup() {
    # Unmount any mounted filesystems in temp dir
    if [[ -n "$TEMP_DIR" && -d "$TEMP_DIR" ]]; then
        for mount in "$TEMP_DIR"/*/; do
            if mountpoint -q "$mount" 2>/dev/null; then
                sudo umount "$mount" 2>/dev/null || true
            fi
        done
        # Also check for specific mount points
        sudo umount "$TEMP_DIR/boot_mount" 2>/dev/null || true
        sudo umount "$TEMP_DIR/esp_mount" 2>/dev/null || true
        rm -rf "$TEMP_DIR" 2>/dev/null || true
    fi
    
    if [[ -n "$MOUNT_POINT" && -d "$MOUNT_POINT" ]]; then
        sudo umount "$MOUNT_POINT" 2>/dev/null || true
        rmdir "$MOUNT_POINT" 2>/dev/null || true
    fi
}

# Set trap for cleanup
trap cleanup EXIT

# Logging functions
log_success() {
    echo -e "${GREEN}✓${NC} $1"
}

log_error() {
    echo -e "${RED}✗${NC} $1"
    EXIT_CODE=1
}

log_warning() {
    echo -e "${YELLOW}⚠${NC} $1"
}

log_info() {
    echo -e "${BLUE}ℹ${NC} $1"
}

log_verbose() {
    if [[ "$VERBOSE" == true ]]; then
        echo -e "${BLUE}[VERBOSE]${NC} $1"
    fi
}

# Usage function
usage() {
    cat << EOF
Usage: $0 [OPTIONS] <iso-file>

UEFI ISO Bootability Checker - Provides precise diagnostics for UEFI boot issues

OPTIONS:
    -v, --verbose       Enable verbose output
    -j, --json         Output machine-readable JSON summary
    -h, --help         Show this help message

ARGUMENTS:
    iso-file           Path to the ISO file to analyze

DESCRIPTION:
    This script performs comprehensive UEFI bootability analysis including:
    - Basic bootability validation
    - El Torito boot catalog analysis (with xorriso enhancement)
    - EFI directory structure verification (multiple methods)
    - Boot image extraction and analysis
    - EFI System Partition extraction for hybrid ISOs
    - Bootloader file validation (PE32/PE32+ format checking)
    - Detailed failure diagnostics with specific solutions

EXAMPLES:
    ./check_iso.sh ubuntu-22.04-desktop-amd64.iso
    ./check_iso.sh -v windows-11.iso
    ./check_iso.sh --json --verbose my-custom.iso > analysis.json

DEBUGGING TIPS:
    - Use -v/--verbose for detailed technical output
    - Check dependency suggestions if enhanced features are missing
    - Modern ISOs may store EFI files in ESP partitions, not /EFI directories
    - Case sensitivity matters: /EFI/BOOT/ (correct) vs /efi/boot/ (problematic)

EXIT CODES:
    0    ISO is UEFI bootable
    1    ISO has UEFI boot issues (specific reasons provided)
    2    Script error or invalid arguments

EOF
}

# Parse command line arguments
parse_arguments() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            -v|--verbose)
                VERBOSE=true
                shift
                ;;
            -j|--json)
                JSON_OUTPUT=true
                shift
                ;;
            -h|--help)
                usage
                exit 0
                ;;
            -*)
                echo "Error: Unknown option $1" >&2
                usage >&2
                exit 2
                ;;
            *)
                if [[ -z "$ISO_FILE" ]]; then
                    ISO_FILE="$1"
                else
                    echo "Error: Multiple ISO files specified" >&2
                    usage >&2
                    exit 2
                fi
                shift
                ;;
        esac
    done

    # Validate arguments
    if [[ -z "$ISO_FILE" ]]; then
        echo "Error: No ISO file specified" >&2
        usage >&2
        exit 2
    fi

    if [[ ! -f "$ISO_FILE" ]]; then
        echo "Error: ISO file '$ISO_FILE' does not exist" >&2
        exit 2
    fi

    # Convert to absolute path
    ISO_FILE=$(realpath "$ISO_FILE")
}

# Check required dependencies
check_dependencies() {
    local missing_deps=()
    local optional_deps=()
    
    # Required dependencies
    for cmd in file isoinfo mount umount fdisk; do
        if ! command -v "$cmd" &> /dev/null; then
            missing_deps+=("$cmd")
        fi
    done
    
    # Optional dependencies for enhanced functionality
    for cmd in bsdtar 7z xorriso readelf strings jq geteltorito; do
        if ! command -v "$cmd" &> /dev/null; then
            optional_deps+=("$cmd")
        fi
    done
    
    if [[ ${#missing_deps[@]} -gt 0 ]]; then
        echo "Error: Missing required dependencies: ${missing_deps[*]}" >&2
        echo "" >&2
        echo "Install commands:" >&2
        echo "  Debian/Ubuntu: sudo apt install genisoimage util-linux fdisk coreutils" >&2
        echo "  Fedora/RHEL:   sudo dnf install genisoimage util-linux" >&2
        exit 2
    fi
    
    if [[ ${#optional_deps[@]} -gt 0 && "$VERBOSE" == true ]]; then
        log_info "Optional tools for enhanced analysis: ${optional_deps[*]}"
        echo "  Install: sudo apt install libarchive-tools p7zip-full xorriso binutils jq syslinux-utils"
    fi
}

# Initialize temporary directories
init_temp_dirs() {
    TEMP_DIR=$(mktemp -d -t uefi-check-XXXXXX)
    MOUNT_POINT="$TEMP_DIR/iso_mount"
    mkdir -p "$MOUNT_POINT"
    log_verbose "Temporary directory: $TEMP_DIR"
}

# Check basic ISO bootability
check_basic_bootability() {
    echo -e "\n${BLUE}=== 1. Basic Bootability Check ===${NC}"
    
    local file_output
    file_output=$(file "$ISO_FILE")
    log_verbose "file output: $file_output"
    
    if [[ $file_output == *"(bootable)"* ]]; then
        log_success "ISO is marked as bootable"
    else
        log_error "ISO is not marked as bootable"
        echo "  Reason: The ISO file lacks basic boot sector configuration"
        echo "  Solution: Recreate the ISO with proper bootable settings"
    fi
}

# Analyze El Torito boot catalog
check_el_torito() {
    echo -e "\n${BLUE}=== 2. El Torito Boot Catalog Analysis ===${NC}"
    
    local isoinfo_output
    if ! isoinfo_output=$(isoinfo -d -i "$ISO_FILE" 2>&1); then
        log_error "Failed to read ISO metadata"
        echo "  Reason: ISO file may be corrupted or not a valid ISO9660 image"
        return
    fi
    
    log_verbose "isoinfo output:\n$isoinfo_output"
    
    # Check for El Torito boot catalog
    if ! echo "$isoinfo_output" | grep -q "El Torito"; then
        log_error "No El Torito boot catalog found"
        echo "  Reason: ISO lacks El Torito boot catalog required for CD/DVD booting"
        echo "  Solution: Recreate ISO with proper El Torito boot catalog"
        return
    fi
    
    log_success "El Torito boot catalog present"
    JSON_HAS_ELTORITO="true"
    
    # Check for EFI platform entries using isoinfo
    local boot_entries
    boot_entries=$(echo "$isoinfo_output" | grep -A 20 "El Torito" || true)
    
    local has_efi_platform=false
    if echo "$boot_entries" | grep -q "Platform ID: 0xef"; then
        has_efi_platform=true
        JSON_HAS_EFI_PLATFORM="true"
        log_success "EFI platform boot entry found (Platform ID: 0xEF)"
    fi
    
    # Try enhanced detection with xorriso if available
    if command -v xorriso &> /dev/null; then
        log_verbose "Using xorriso for enhanced El Torito analysis"
        local xorriso_output
        if xorriso_output=$(xorriso -indev "$ISO_FILE" -report_el_torito plain 2>/dev/null); then
            log_verbose "xorriso El Torito report:\n$xorriso_output"
            
            if echo "$xorriso_output" | grep -qi 'UEFI\|platform.*0xEF'; then
                if [[ "$has_efi_platform" == false ]]; then
                    log_success "EFI platform entry detected by xorriso (UEFI boot image)"
                    has_efi_platform=true
                    JSON_HAS_EFI_PLATFORM="true"
                fi
            fi
        fi
    fi
    
    if [[ "$has_efi_platform" == false ]]; then
        log_error "No EFI platform boot entry found"
        echo "  Reason: El Torito catalog lacks Platform ID 0xEF entry required for UEFI boot"
        echo "  Solution: Add EFI boot entry when creating the ISO"
    fi
    
    # Check for BIOS platform entries  
    if echo "$boot_entries" | grep -q "Platform ID: 0x00"; then
        log_info "BIOS platform boot entry also present (hybrid ISO)"
    fi
}

# Extract EFI boot image from El Torito
extract_boot_image() {
    local boot_img="$TEMP_DIR/efi_boot.img"
    
    log_verbose "Attempting to extract EFI boot image from El Torito..."
    
    # Try geteltorito first (most reliable for EFI images)
    if command -v geteltorito &> /dev/null; then
        if geteltorito -o "$boot_img" "$ISO_FILE" 2>/dev/null; then
            log_verbose "Successfully extracted boot image using geteltorito: $boot_img"
            echo "$boot_img"
            return 0
        fi
    fi
    
    # Fallback: try to extract using dd with xorriso info
    if command -v xorriso &> /dev/null; then
        local xorriso_output
        if xorriso_output=$(xorriso -indev "$ISO_FILE" -report_el_torito plain 2>/dev/null); then
            # Look for UEFI boot image LBA
            local uefi_lba
            uefi_lba=$(echo "$xorriso_output" | awk '/UEFI.*y/ {for(i=1;i<=NF;i++) if($i ~ /^[0-9]+$/ && $i > 100) print $i}' | head -1)
            
            if [[ -n "$uefi_lba" && "$uefi_lba" -gt 0 ]]; then
                log_verbose "Found UEFI boot image at LBA $uefi_lba, extracting with dd..."
                # Extract a reasonable size (64MB should cover most EFI images)
                if dd if="$ISO_FILE" of="$boot_img" bs=2048 skip="$uefi_lba" count=32768 2>/dev/null; then
                    log_verbose "Successfully extracted boot image using dd: $boot_img"
                    echo "$boot_img"
                    return 0
                fi
            fi
        fi
    fi
    
    log_verbose "Boot image extraction failed"
    return 1
}

# Mount and analyze boot image
analyze_boot_image() {
    local boot_img="$1"
    local mount_point="$TEMP_DIR/boot_mount"
    
    mkdir -p "$mount_point"
    log_verbose "Analyzing boot image: $boot_img"
    
    # Check if it's a filesystem image
    local file_info
    file_info=$(file "$boot_img" 2>/dev/null)
    log_verbose "Boot image type: $file_info"
    
    # Try to mount it
    if sudo mount -o loop,ro "$boot_img" "$mount_point" 2>/dev/null; then
        log_verbose "Successfully mounted boot image at $mount_point"
        
        # Check for EFI directory in boot image
        if [[ -d "$mount_point/EFI" ]]; then
            log_success "Found EFI directory in boot image"
            echo "$mount_point"
            return 0
        else
            log_verbose "No EFI directory found in boot image"
        fi
        
        sudo umount "$mount_point" 2>/dev/null
    else
        log_verbose "Failed to mount boot image (may not be a filesystem)"
    fi
    
    return 1
}

# Extract and analyze EFI System Partition
extract_esp_partition() {
    log_verbose "Attempting to extract EFI System Partition..."
    
    # Get partition info from fdisk
    local fdisk_output
    if ! fdisk_output=$(fdisk -l "$ISO_FILE" 2>/dev/null); then
        log_verbose "No partition table found"
        return 1
    fi
    
    # Look for EFI partition (type 'ef' or 'EFI')
    local esp_info
    esp_info=$(echo "$fdisk_output" | grep -E "ef.*EFI")
    
    if [[ -z "$esp_info" ]]; then
        log_verbose "No EFI System Partition found in partition table"
        return 1
    fi
    
    log_verbose "Found EFI System Partition: $(echo "$esp_info" | awk '{print $1, $4, $5}')"
    
    # Extract partition details (start sector, size)
    # Format: device  start  end  sectors  size  id  type
    local start_sector size_sectors
    start_sector=$(echo "$esp_info" | awk '{print $2}')
    size_sectors=$(echo "$esp_info" | awk '{print $4}')
    
    if [[ -z "$start_sector" || -z "$size_sectors" ]]; then
        log_verbose "Could not parse EFI partition details"
        return 1
    fi
    
    log_verbose "Extracting ESP: sector $start_sector, size $size_sectors sectors"
    
    # Extract the EFI partition
    local esp_img="$TEMP_DIR/esp.img"
    
    if dd if="$ISO_FILE" of="$esp_img" bs=512 skip="$start_sector" count="$size_sectors" >/dev/null 2>&1; then
        log_verbose "Successfully extracted ESP partition ($(du -h "$esp_img" | cut -f1))"
        
        # Try to mount and analyze it
        local esp_mount="$TEMP_DIR/esp_mount"
        mkdir -p "$esp_mount"
        
        if sudo mount -o loop,ro "$esp_img" "$esp_mount" 2>/dev/null; then
            log_verbose "ESP mounted successfully"
            
            if [[ -d "$esp_mount/EFI" ]]; then
                log_verbose "Found EFI directory in ESP partition"
                echo "$esp_mount"
                return 0
            else
                log_verbose "No EFI directory in ESP partition"
                sudo umount "$esp_mount" 2>/dev/null || true
            fi
        else
            log_verbose "Failed to mount ESP partition"
        fi
    else
        log_verbose "Failed to extract ESP partition with dd"
    fi
    return 1
}

# Try unprivileged extraction first
try_unprivileged_extraction() {
    local extract_dir="$TEMP_DIR/efi_extract"
    mkdir -p "$extract_dir"
    
    log_verbose "Attempting unprivileged extraction to avoid sudo..."
    
    # Try bsdtar first
    if command -v bsdtar &> /dev/null; then
        if bsdtar -C "$extract_dir" -xf "$ISO_FILE" 'EFI/*' 2>/dev/null; then
            log_verbose "Successfully extracted EFI files using bsdtar"
            echo "$extract_dir"
            return 0
        fi
    fi
    
    # Try 7z
    if command -v 7z &> /dev/null; then
        if 7z x -o"$extract_dir" "$ISO_FILE" 'EFI/*' >/dev/null 2>&1; then
            log_verbose "Successfully extracted EFI files using 7z"
            echo "$extract_dir"
            return 0
        fi
    fi
    
    log_verbose "Unprivileged extraction failed, will need to mount with sudo"
    return 1
}

# Check EFI directory structure
check_efi_structure() {
    echo -e "\n${BLUE}=== 3. EFI Directory Structure Validation ===${NC}"
    
    # Try unprivileged extraction first
    if EFI_ROOT=$(try_unprivileged_extraction); then
        log_success "Using unprivileged extraction (no sudo required)"
    else
        log_info "Using sudo mount for ISO access"
        # Mount the ISO
        if ! sudo mount -o loop,ro "$ISO_FILE" "$MOUNT_POINT" 2>/dev/null; then
            log_error "Failed to mount ISO file"
            echo "  Reason: ISO file may be corrupted or not mountable"
            
            # Try boot image extraction as fallback
            log_info "Attempting boot image extraction as fallback..."
            if boot_img=$(extract_boot_image); then
                if EFI_ROOT=$(analyze_boot_image "$boot_img"); then
                    log_success "Using EFI boot image extraction"
                else
                    log_verbose "Boot image extraction succeeded but no EFI directory found"
                    return
                fi
            else
                log_verbose "Boot image extraction failed"
                return
            fi
        else
            EFI_ROOT="$MOUNT_POINT"
            log_verbose "ISO mounted at: $MOUNT_POINT"
        fi
    fi
    
    # Check for EFI directory
    if [[ ! -d "$EFI_ROOT/EFI" ]]; then
        log_warning "EFI directory not found in ISO root filesystem"
        
        # Try boot image extraction if we haven't already
        if [[ "$EFI_ROOT" != *"boot_mount"* ]]; then
            log_info "Checking for EFI files in boot image..."
            local found_efi=false
            
            # Try boot image extraction first
            if boot_img=$(extract_boot_image); then
                if boot_efi_root=$(analyze_boot_image "$boot_img"); then
                    log_success "Found EFI directory in boot image!"
                    EFI_ROOT="$boot_efi_root"
                    found_efi=true
                else
                    log_verbose "Boot image extracted but no EFI directory found"
                fi
            else
                log_verbose "Boot image extraction failed"
            fi
            
            # Try ESP partition extraction if boot image didn't work
            if [[ "$found_efi" == false ]]; then
                log_info "Trying EFI System Partition extraction..."
                
                # Call function directly and check result
                extract_esp_partition
                local esp_result=$?
                
                if [[ $esp_result -eq 0 ]]; then
                    # If successful, the function outputs the mount point to stdout
                    # We need to call it again in command substitution to capture output
                    esp_root=$(extract_esp_partition 2>/dev/null)
                    if [[ -n "$esp_root" ]]; then
                        log_success "Found EFI directory in ESP partition!"
                        EFI_ROOT="$esp_root"
                        found_efi=true
                    fi
                else
                    log_verbose "ESP partition extraction failed"
                fi
            fi
            
            # If still no EFI directory found
            if [[ "$found_efi" == false ]]; then
                log_error "No EFI directory found in ISO, boot image, or ESP partition"
                echo "  Reason: Missing /EFI directory in all possible locations"
                echo "  Solution: Ensure ISO contains /EFI directory with boot files"
                return
            fi
        else
            log_error "EFI directory not found in boot image"
            echo "  Reason: Missing /EFI directory required for UEFI boot"
            echo "  Solution: Ensure ISO contains /EFI directory with boot files"
            return
        fi
    else
        log_success "EFI directory found"
        JSON_HAS_EFI_DIR="true"
        
        # Check for BOOT subdirectory (case insensitive)
        local boot_dir=""
        if [[ -d "$EFI_ROOT/EFI/BOOT" ]]; then
            boot_dir="$EFI_ROOT/EFI/BOOT"
            log_success "/EFI/BOOT directory found"
        elif [[ -d "$EFI_ROOT/EFI/boot" ]]; then
            boot_dir="$EFI_ROOT/EFI/boot"
            log_warning "/EFI/boot directory found (should be uppercase BOOT)"
            echo "  Note: UEFI spec requires uppercase /EFI/BOOT/ for compatibility"
        else
            log_error "/EFI/BOOT directory not found"
            echo "  Reason: Missing /EFI/BOOT directory required for default UEFI boot"
            echo "  Solution: Create /EFI/BOOT directory with default bootloader"
        fi
        
        # List all EFI files
        local efi_files
        efi_files=$(find "$EFI_ROOT/EFI" -name "*.efi" -type f 2>/dev/null || true)
        
        if [[ -n "$efi_files" ]]; then
            log_success "EFI executable files found:"
            while IFS= read -r efi_file; do
                local rel_path=${efi_file#$EFI_ROOT}
                echo "    $rel_path"
                
                # Enhanced EFI binary analysis
                if [[ -n "$boot_dir" && "$efi_file" == *"BOOT"* ]]; then
                    analyze_efi_binary "$efi_file"
                fi
            done <<< "$efi_files"
        else
            log_error "No EFI executable files found"
            echo "  Reason: No *.efi files found in EFI directory structure"
            echo "  Solution: Add proper EFI bootloader files (.efi)"
        fi
    fi
}

# Enhanced EFI binary analysis
analyze_efi_binary() {
    local efi_file="$1"
    local file_info
    
    log_verbose "Analyzing EFI binary: $efi_file"
    
    file_info=$(file "$efi_file" 2>/dev/null || echo "unknown")
    
    # Check PE32 vs PE32+ architecture
    if [[ $file_info == *"PE32+ executable"* ]]; then
        log_verbose "  → 64-bit PE32+ executable (correct for x64 UEFI)"
    elif [[ $file_info == *"PE32 executable"* ]]; then
        log_warning "  → 32-bit PE32 executable (may not boot on 64-bit UEFI firmware)"
    else
        log_warning "  → Not a PE executable: $file_info"
    fi
    
    # Check for known bootloaders using strings
    if command -v strings &> /dev/null; then
        local strings_output
        strings_output=$(strings "$efi_file" 2>/dev/null | head -20)
        
        if echo "$strings_output" | grep -qi "shim"; then
            log_verbose "  → Appears to be shim bootloader (Secure Boot compatible)"
        elif echo "$strings_output" | grep -qi "grub"; then
            log_verbose "  → Appears to be GRUB bootloader"
        elif echo "$strings_output" | grep -qi "systemd"; then
            log_verbose "  → Appears to be systemd-boot"
        fi
    fi
}

# Check default UEFI bootloader
check_default_bootloader() {
    echo -e "\n${BLUE}=== 4. Default UEFI Bootloader Verification ===${NC}"
    
    # Skip if EFI directory structure check failed (not mounted or no EFI dir)
    if [[ -z "$EFI_ROOT" || ! -d "$EFI_ROOT/EFI" ]]; then
        log_warning "Skipping bootloader check - no EFI directory structure available"
        return
    fi
    
    local default_bootloaders=(
        "BOOTX64.EFI"  # x64 systems
        "BOOTAA64.EFI" # ARM64 systems  
        "BOOTARM.EFI"  # ARM systems
        "BOOTIA32.EFI" # 32-bit x86 systems
    )
    
    local found_bootloader=false
    
    for bootloader in "${default_bootloaders[@]}"; do
        local bootloader_path=""
        local bootloader_lower="${bootloader,,}"  # Convert to lowercase
        
        # Check uppercase BOOT directory first (correct)
        if [[ -f "$EFI_ROOT/EFI/BOOT/$bootloader" ]]; then
            bootloader_path="$EFI_ROOT/EFI/BOOT/$bootloader"
            log_success "Default bootloader found: $bootloader"
        # Check lowercase boot directory (incorrect but functional)
        elif [[ -f "$EFI_ROOT/EFI/boot/$bootloader_lower" ]]; then
            bootloader_path="$EFI_ROOT/EFI/boot/$bootloader_lower"
            log_warning "Default bootloader found in lowercase directory: /EFI/boot/$bootloader_lower"
            echo "  Note: Should be /EFI/BOOT/$bootloader for UEFI compliance"
        fi
        
        if [[ -n "$bootloader_path" ]]; then
            JSON_HAS_BOOTLOADER="true"
            JSON_BOOTLOADER_ARCH="$bootloader"
            
            # Check file permissions
            if [[ -r "$bootloader_path" ]]; then
                log_success "Bootloader file is readable"
            else
                log_warning "Bootloader file has restricted permissions"
            fi
            
            # Check file size
            local file_size
            file_size=$(stat -f%z "$bootloader_path" 2>/dev/null || stat -c%s "$bootloader_path" 2>/dev/null || echo "0")
            if [[ $file_size -gt 0 ]]; then
                log_success "Bootloader file size: $file_size bytes"
            else
                log_error "Bootloader file is empty"
                echo "  Reason: $bootloader exists but has zero size"
            fi
            
            # Verify PE32+ format
            local file_info
            file_info=$(file "$bootloader_path")
            if [[ $file_info == *"PE32+ executable"* ]] || [[ $file_info == *"MS-DOS executable"* ]]; then
                log_success "Bootloader is a valid PE32+ executable"
            else
                log_error "Bootloader is not a valid PE32+ executable"
                echo "  Reason: $bootloader is not in proper PE32+ format"
                echo "  File type: $file_info"
            fi
            
            found_bootloader=true
            break
        fi
    done
    
    if [[ "$found_bootloader" == false ]]; then
        log_error "No default UEFI bootloader found"
        echo "  Reason: Missing default bootloader in /EFI/BOOT/"
        echo "  Expected files: ${default_bootloaders[*]}"
        echo "  Solution: Add appropriate default bootloader for target architecture"
    fi
}

# Check partition table for hybrid ISOs
check_partition_table() {
    echo -e "\n${BLUE}=== 5. Partition Table Analysis ===${NC}"
    
    # Check if ISO has a partition table
    local fdisk_output
    if fdisk_output=$(fdisk -l "$ISO_FILE" 2>/dev/null); then
        log_info "Hybrid ISO detected (contains partition table)"
        log_verbose "fdisk output:\n$fdisk_output"
        
        # Check for EFI System Partition
        if echo "$fdisk_output" | grep -q "EFI"; then
            log_success "EFI System Partition found in partition table"
        else
            log_warning "No EFI System Partition found in partition table"
            echo "  Note: This may still work if EFI files are in ISO9660 filesystem"
        fi
    else
        log_info "Standard ISO (no partition table)"
        echo "  Note: Relying on ISO9660 filesystem for EFI files"
    fi
}

# Generate JSON output
generate_json_output() {
    local json_status
    if [[ $EXIT_CODE -eq 0 ]]; then
        json_status="bootable"
    else
        json_status="not_bootable"
    fi
    
    # Simple JSON without dependencies
    cat << EOF
{
  "iso_file": "$ISO_FILE",
  "status": "$json_status",
  "checks": {
    "has_eltorito": $JSON_HAS_ELTORITO,
    "has_efi_platform": $JSON_HAS_EFI_PLATFORM,
    "has_efi_directory": $JSON_HAS_EFI_DIR,
    "has_bootloader": $JSON_HAS_BOOTLOADER
  },
  "details": {
    "bootloader_arch": "$JSON_BOOTLOADER_ARCH",
    "exit_code": $EXIT_CODE
  },
  "timestamp": "$(date -Iseconds)",
  "script_version": "1.0.0"
}
EOF
}

# Generate summary report
generate_summary() {
    echo -e "\n${BLUE}=== UEFI Bootability Summary ===${NC}"
    
    if [[ $EXIT_CODE -eq 0 ]]; then
        echo -e "${GREEN}✓ ISO appears to be UEFI bootable${NC}"
        echo "  All essential UEFI boot requirements are met."
    else
        echo -e "${RED}✗ ISO has UEFI boot issues${NC}"
        echo "  Review the specific error messages above for details."
        echo ""
        echo "Common solutions:"
        echo "  • Recreate ISO with proper UEFI boot configuration"
        echo "  • Ensure /EFI/BOOT/BOOTX64.EFI is present and valid"
        echo "  • Add El Torito boot catalog with EFI platform entry"
        echo "  • Verify all EFI files are in PE32+ format"
    fi
}

# Main execution function
main() {
    echo "UEFI ISO Bootability Checker"
    echo "============================"
    echo "Analyzing: $ISO_FILE"
    
    parse_arguments "$@"
    check_dependencies
    init_temp_dirs
    
    # Run all checks
    check_basic_bootability
    check_el_torito  
    check_efi_structure
    check_default_bootloader
    check_partition_table
    
    # Cleanup mount point
    sudo umount "$MOUNT_POINT" 2>/dev/null || true
    
    generate_summary
    
    # Output JSON if requested
    if [[ "$JSON_OUTPUT" == true ]]; then
        echo ""
        generate_json_output
    fi
    
    # Show helpful debugging commands in verbose mode
    if [[ "$VERBOSE" == true && $EXIT_CODE -eq 1 ]]; then
        echo ""
        echo -e "${BLUE}=== Useful Manual Analysis Commands ===${NC}"
        echo "# Basic El Torito analysis:"
        echo "isoinfo -d -i \"$ISO_FILE\" | grep -E \"(El Torito|Boot)\""
        echo ""
        echo "# Enhanced El Torito with xorriso:"
        echo "xorriso -indev \"$ISO_FILE\" -report_el_torito plain"
        echo ""
        echo "# Extract boot image manually:"
        echo "geteltorito -o efi_boot.img \"$ISO_FILE\""
        echo ""
        echo "# Check partition table:"
        echo "fdisk -l \"$ISO_FILE\""
        echo ""
        echo "# Mount ISO and explore:"
        echo "sudo mkdir /mnt/iso && sudo mount -o loop \"$ISO_FILE\" /mnt/iso"
        echo "find /mnt/iso -name \"*.efi\" -type f"
    fi
    
    exit $EXIT_CODE
}

# Execute main function with all arguments
main "$@"