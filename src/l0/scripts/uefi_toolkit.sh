#!/bin/bash
# UEFI ISO Testing and Validation Toolkit
# Provides comprehensive analysis without requiring sudo privileges

set -euo pipefail

# Color output for better readability
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
log_info() { echo -e "${BLUE}[INFO]${NC} $*"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $*"; }
log_warning() { echo -e "${YELLOW}[WARNING]${NC} $*"; }
log_error() { echo -e "${RED}[ERROR]${NC} $*"; }

# Check if file exists and is readable
check_file() {
    local file="$1"
    local description="$2"
    
    if [[ ! -f "$file" ]]; then
        log_error "$description not found: $file"
        return 1
    fi
    
    if [[ ! -r "$file" ]]; then
        log_error "$description not readable: $file"
        return 1
    fi
    
    log_success "$description found: $file ($(stat -c%s "$file" | numfmt --to=iec))"
    return 0
}

# Analyze ISO structure without mounting
analyze_iso() {
    local iso_file="$1"
    
    log_info "=== ISO Analysis: $iso_file ==="
    
    # Basic file information
    if ! check_file "$iso_file" "ISO file"; then
        return 1
    fi
    
    # File type
    local file_type
    file_type=$(file "$iso_file")
    log_info "File type: $file_type"
    
    # ISO 9660 information
    if command -v isoinfo >/dev/null 2>&1; then
        log_info "ISO 9660 details:"
        isoinfo -d -i "$iso_file" 2>/dev/null | while read -r line; do
            echo "  $line"
        done
        
        # Check for UEFI indicators
        log_info "Checking for UEFI boot indicators..."
        if isoinfo -l -i "$iso_file" 2>/dev/null | grep -qi "EFI\|UEFI\|bootx64"; then
            log_success "UEFI indicators found in ISO"
        else
            log_warning "No UEFI indicators found in ISO"
        fi
        
        # List root directory
        log_info "Root directory contents:"
        isoinfo -l -i "$iso_file" 2>/dev/null | head -20 | while read -r line; do
            echo "  $line"
        done
    else
        log_warning "isoinfo not available - install genisoimage for detailed ISO analysis"
    fi
}

# Analyze QEMU UEFI disk image
analyze_qemu_uefi() {
    local img_file="$1"
    
    log_info "=== QEMU UEFI Image Analysis: $img_file ==="
    
    if ! check_file "$img_file" "QEMU UEFI image"; then
        return 1
    fi
    
    # Check if compressed
    local file_type
    file_type=$(file "$img_file")
    log_info "File type: $file_type"
    
    if [[ "$file_type" == *"bzip2"* ]]; then
        log_info "Image is bzip2 compressed"
        
        # Test decompression without extracting
        if command -v lbzip2 >/dev/null 2>&1; then
            log_info "Testing decompression with lbzip2..."
            if lbzip2 -t "$img_file" 2>/dev/null; then
                log_success "Image passes bzip2 integrity check"
                
                # Get uncompressed size
                local uncompressed_size
                uncompressed_size=$(lbzip2 -dc "$img_file" | wc -c)
                log_info "Uncompressed size: $(echo "$uncompressed_size" | numfmt --to=iec)"
                
                # Check format of uncompressed data
                local uncompressed_type
                uncompressed_type=$(lbzip2 -dc "$img_file" | head -c 1024 | file -)
                log_info "Uncompressed format: $uncompressed_type"
            else
                log_error "Image fails bzip2 integrity check"
                return 1
            fi
        else
            log_warning "lbzip2 not available - install lbzip2 for detailed analysis"
        fi
    fi
}

# Check for required tools
check_dependencies() {
    log_info "=== Dependency Check ==="
    
    local tools=(
        "file:File type identification"
        "isoinfo:ISO 9660 analysis (genisoimage package)"
        "lbzip2:Fast bzip2 compression/decompression"
        "qemu-img:QEMU disk image tools"
        "xorriso:ISO creation and manipulation"
        "mksquashfs:SquashFS creation"
        "cpio:Archive creation"
        "dd:Low-level disk operations"
        "losetup:Loop device management (requires sudo)"
        "mount:Filesystem mounting (requires sudo)"
    )
    
    local missing_count=0
    
    for tool_desc in "${tools[@]}"; do
        local tool="${tool_desc%%:*}"
        local desc="${tool_desc#*:}"
        
        if command -v "$tool" >/dev/null 2>&1; then
            log_success "$tool available - $desc"
        else
            log_warning "$tool missing - $desc"
            ((missing_count++))
        fi
    done
    
    if [[ $missing_count -eq 0 ]]; then
        log_success "All dependencies available"
    else
        log_warning "$missing_count dependencies missing - install for full functionality"
    fi
}

# Validate QEMU UEFI components without extraction
validate_qemu_uefi_structure() {
    local img_file="$1"
    
    log_info "=== QEMU UEFI Structure Validation ==="
    
    if ! check_file "$img_file" "QEMU UEFI image"; then
        return 1
    fi
    
    # Convert to raw format in memory and analyze
    if command -v qemu-img >/dev/null 2>&1; then
        log_info "Converting QEMU image to analyze partition structure..."
        
        # Create temporary file for analysis
        local temp_raw="/tmp/uefi_analysis_raw.img"
        
        if [[ "$img_file" == *".bz2" ]]; then
            log_info "Decompressing and converting..."
            lbzip2 -dc "$img_file" | qemu-img convert -f qcow2 -O raw - "$temp_raw" 2>/dev/null
        else
            qemu-img convert -f qcow2 -O raw "$img_file" "$temp_raw" 2>/dev/null
        fi
        
        if [[ -f "$temp_raw" ]]; then
            # Analyze partition table
            log_info "Partition structure:"
            if command -v fdisk >/dev/null 2>&1; then
                fdisk -l "$temp_raw" 2>/dev/null | while read -r line; do
                    echo "  $line"
                done
            fi
            
            # Clean up
            rm -f "$temp_raw"
            log_success "QEMU UEFI structure analysis complete"
        else
            log_error "Failed to convert QEMU image for analysis"
            return 1
        fi
    else
        log_warning "qemu-img not available - install qemu-utils for structure analysis"
        return 1
    fi
}

# Test ISO bootability (basic checks without actual booting)
test_iso_bootability() {
    local iso_file="$1"
    
    log_info "=== ISO Bootability Test ==="
    
    if ! check_file "$iso_file" "ISO file"; then
        return 1
    fi
    
    # Check El Torito boot catalog
    if command -v isoinfo >/dev/null 2>&1; then
        log_info "Checking El Torito boot catalog..."
        
        local boot_info
        boot_info=$(isoinfo -d -i "$iso_file" 2>/dev/null | grep -i "boot\|catalog\|torito")
        
        if [[ -n "$boot_info" ]]; then
            log_success "El Torito boot catalog found:"
            echo "$boot_info" | while read -r line; do
                echo "  $line"
            done
        else
            log_warning "No El Torito boot catalog found"
        fi
        
        # Check for specific boot files
        log_info "Checking for boot files..."
        
        local boot_files
        boot_files=$(isoinfo -l -i "$iso_file" 2>/dev/null | grep -i "boot\|efi\|grub\|vmlinuz")
        
        if [[ -n "$boot_files" ]]; then
            log_success "Boot files found:"
            echo "$boot_files" | while read -r line; do
                echo "  $line"
            done
        else
            log_warning "No obvious boot files found"
        fi
    fi
}

# Main execution
main() {
    local command="$1"
    shift
    
    case "$command" in
        "check-deps")
            check_dependencies
            ;;
        "analyze-iso")
            if [[ $# -ne 1 ]]; then
                log_error "Usage: $0 analyze-iso <iso_file>"
                exit 1
            fi
            analyze_iso "$1"
            test_iso_bootability "$1"
            ;;
        "analyze-qemu")
            if [[ $# -ne 1 ]]; then
                log_error "Usage: $0 analyze-qemu <qemu_uefi_image>"
                exit 1
            fi
            analyze_qemu_uefi "$1"
            validate_qemu_uefi_structure "$1"
            ;;
        "full-analysis")
            if [[ $# -ne 2 ]]; then
                log_error "Usage: $0 full-analysis <qemu_uefi_image> <iso_file>"
                exit 1
            fi
            check_dependencies
            echo
            analyze_qemu_uefi "$1"
            echo
            analyze_iso "$2"
            ;;
        *)
            echo "UEFI ISO Testing and Validation Toolkit"
            echo
            echo "Usage: $0 <command> [args]"
            echo
            echo "Commands:"
            echo "  check-deps                     - Check for required dependencies"
            echo "  analyze-iso <iso_file>         - Analyze ISO structure and bootability"
            echo "  analyze-qemu <qemu_image>      - Analyze QEMU UEFI disk image"
            echo "  full-analysis <qemu> <iso>     - Complete analysis of both images"
            echo
            exit 1
            ;;
    esac
}

# Execute main function with all arguments
main "$@"