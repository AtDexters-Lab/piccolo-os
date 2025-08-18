#!/bin/bash
# Test UEFI ISO Creation Readiness
# Validates all prerequisites without actually building

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TOOLKIT_SCRIPT="$SCRIPT_DIR/uefi_toolkit.sh"
BUILD_SCRIPT="$SCRIPT_DIR/create_uefi_iso.sh"

# Color output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() { echo -e "${BLUE}[INFO]${NC} $*"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $*"; }
log_warning() { echo -e "${YELLOW}[WARNING]${NC} $*"; }
log_error() { echo -e "${RED}[ERROR]${NC} $*"; }

echo "Piccolo OS UEFI ISO Readiness Test"
echo "==================================="
echo

# Test 1: Dependencies
log_info "1. Checking dependencies..."
if "$TOOLKIT_SCRIPT" check-deps | grep -q "All dependencies available"; then
    log_success "All dependencies available"
else
    log_error "Dependencies missing"
    exit 1
fi

# Test 2: Source images
log_info "2. Checking source images..."
BASE_DIR="$(dirname "$SCRIPT_DIR")"
BUILD_DIR="$BASE_DIR/build"
QEMU_PATTERN="$BUILD_DIR/work-*/scripts/__build__/images/images/*/piccolo-stable-*/flatcar_production_qemu_uefi_image.img.bz2"
QEMU_IMAGE=$(find $QEMU_PATTERN 2>/dev/null | head -1)

if [[ -n "$QEMU_IMAGE" ]]; then
    log_success "QEMU UEFI image found: $QEMU_IMAGE"
    
    # Basic validation - just check if it's a valid bzip2 file
    log_info "Validating QEMU UEFI image integrity..."
    if lbzip2 -t "$QEMU_IMAGE" >/dev/null 2>&1; then
        log_success "QEMU UEFI image integrity check passed"
    else
        log_error "QEMU UEFI image is corrupted or invalid"
        exit 1
    fi
else
    log_error "QEMU UEFI image not found"
    log_info "Run ./build.sh first to generate required images"
    exit 1
fi

# Test 3: Sudo capability
log_info "3. Checking sudo capability..."
if sudo -n true 2>/dev/null; then
    log_success "Sudo access available"
else
    log_warning "Sudo access required - run 'sudo -v' before building"
fi

# Test 4: Disk space
log_info "4. Checking disk space..."
WORK_DIR="/tmp"
AVAILABLE_SPACE=$(df "$WORK_DIR" | awk 'NR==2 {print $4}')
REQUIRED_SPACE=$((2 * 1024 * 1024))  # 2GB in KB

if [[ $AVAILABLE_SPACE -gt $REQUIRED_SPACE ]]; then
    log_success "Sufficient disk space available: $(echo $AVAILABLE_SPACE | numfmt --from-unit=1024 --to=iec)B"
else
    log_warning "Low disk space: $(echo $AVAILABLE_SPACE | numfmt --from-unit=1024 --to=iec)B (need ~2GB)"
fi

# Test 5: Script validation
log_info "5. Validating build script..."
if [[ -x "$BUILD_SCRIPT" ]]; then
    log_success "Build script is executable"
    
    # Check script syntax
    if bash -n "$BUILD_SCRIPT"; then
        log_success "Build script syntax is valid"
    else
        log_error "Build script syntax errors found"
        exit 1
    fi
else
    log_error "Build script not found or not executable: $BUILD_SCRIPT"
    exit 1
fi

echo
log_success "=== READINESS CHECK COMPLETE ==="
log_info "Ready to build UEFI ISO with:"
log_info "  ./scripts/create_uefi_iso.sh [version]"
echo
log_info "Estimated build time: 5-10 minutes"
log_info "Estimated ISO size: ~400-500MB"
log_info "Output location: build/output/{version}/piccolo-os-uefi-{version}.iso"