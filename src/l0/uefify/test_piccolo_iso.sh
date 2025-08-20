#!/bin/bash
#
# test_piccolo_iso.sh - TDD Testing Framework for Piccolo OS ISO Creation
#
# This script implements comprehensive Test-Driven Development (TDD) validation
# for the create_piccolo_iso.sh script, including boot testing with QEMU.
#

set -euo pipefail

# Test configuration
readonly TEST_SCRIPT_VERSION="1.0.0"
readonly TEST_DIR="/tmp/piccolo-iso-tests"
readonly QEMU_MEMORY="2048"
readonly QEMU_TIMEOUT="120"

# Colors for output
readonly RED='\033[0;31m'
readonly GREEN='\033[0;32m'
readonly YELLOW='\033[1;33m'
readonly BLUE='\033[0;34m'
readonly NC='\033[0m' # No Color

# Test results tracking
declare -a TEST_RESULTS=()
declare -g TESTS_PASSED=0
declare -g TESTS_FAILED=0

# Logging functions
log_test() {
    echo -e "${BLUE}[TEST]${NC} $*"
}

log_pass() {
    echo -e "${GREEN}[PASS]${NC} $*"
    ((TESTS_PASSED++))
    TEST_RESULTS+=("✅ $*")
}

log_fail() {
    echo -e "${RED}[FAIL]${NC} $*"
    ((TESTS_FAILED++))
    TEST_RESULTS+=("❌ $*")
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $*"
}

log_info() {
    echo -e "${BLUE}[INFO]${NC} $*"
}

# Test utility functions
setup_test_environment() {
    log_info "Setting up test environment"
    mkdir -p "$TEST_DIR"/{input,output,work}
    
    # Create test source image (dummy)
    if [[ ! -f "$TEST_DIR/input/test_flatcar.bin" ]]; then
        log_info "Creating dummy test image"
        dd if=/dev/zero of="$TEST_DIR/input/test_flatcar.bin" bs=1M count=100 2>/dev/null
    fi
}

cleanup_test_environment() {
    log_info "Cleaning up test environment"
    rm -rf "$TEST_DIR"
}

# Test 1: Dependency validation
test_dependencies() {
    log_test "Testing dependency validation"
    
    local required_deps=(xorriso mkfs.vfat mksquashfs mount losetup cpio gzip dd sudo)
    local missing_deps=()
    
    for dep in "${required_deps[@]}"; do
        if ! command -v "$dep" &>/dev/null; then
            missing_deps+=("$dep")
        fi
    done
    
    if [[ ${#missing_deps[@]} -eq 0 ]]; then
        log_pass "All required dependencies available"
        return 0
    else
        log_fail "Missing dependencies: ${missing_deps[*]}"
        return 1
    fi
}

# Test 2: Script argument parsing
test_argument_parsing() {
    log_test "Testing argument parsing"
    
    # Test help option
    if ./create_piccolo_iso.sh --help >/dev/null 2>&1; then
        log_pass "Help option works"
    else
        log_fail "Help option failed"
        return 1
    fi
    
    # Test missing required arguments
    if ! ./create_piccolo_iso.sh 2>/dev/null; then
        log_pass "Missing arguments properly rejected"
    else
        log_fail "Missing arguments not detected"
        return 1
    fi
    
    return 0
}

# Test 3: Input validation
test_input_validation() {
    log_test "Testing input validation"
    
    # Test non-existent source file
    if ! ./create_piccolo_iso.sh --source /nonexistent --output test.iso --version test 2>/dev/null; then
        log_pass "Non-existent source file properly rejected"
    else
        log_fail "Non-existent source file not detected"
        return 1
    fi
    
    # Test invalid version string
    if ! ./create_piccolo_iso.sh --source "$TEST_DIR/input/test_flatcar.bin" --output test.iso --version "invalid version!" 2>/dev/null; then
        log_pass "Invalid version string properly rejected"
    else
        log_fail "Invalid version string not detected"
        return 1
    fi
    
    return 0
}

# Test 4: Test-only mode
test_only_mode() {
    log_test "Testing test-only mode"
    
    if ./create_piccolo_iso.sh --test-only >/dev/null 2>&1; then
        log_pass "Test-only mode executed successfully"
        return 0
    else
        log_fail "Test-only mode failed"
        return 1
    fi
}

# Test 5: ISO creation with real Flatcar image (if available)
test_iso_creation_with_real_image() {
    log_test "Testing ISO creation with real Flatcar image"
    
    # Look for pre-built Flatcar images
    local source_image=""
    local search_paths=(
        "/home/abhishek-borar/projects/piccolo/piccolo-os/src/l0/build/work-1.0.0/scripts/__build__/images/images/amd64-usr/latest/flatcar_production_image.bin"
        "/home/abhishek-borar/projects/piccolo/piccolo-os/src/l0/build/work-1.0.0/scripts/__build__/images/images/amd64-usr/*/flatcar_production_image.bin"
    )
    
    for path in "${search_paths[@]}"; do
        if [[ -f "$path" ]]; then
            source_image="$path"
            break
        fi
    done
    
    if [[ -z "$source_image" ]]; then
        log_warn "No real Flatcar image found, skipping ISO creation test"
        return 0
    fi
    
    local output_iso="$TEST_DIR/output/test-piccolo.iso"
    
    log_info "Creating ISO from: $source_image"
    if ./create_piccolo_iso.sh \
        --source "$source_image" \
        --output "$output_iso" \
        --version "test-$(date +%s)" \
        --update-group piccolo-test >/dev/null 2>&1; then
        
        if [[ -f "$output_iso" ]]; then
            local iso_size
            iso_size=$(stat -c%s "$output_iso" 2>/dev/null || echo 0)
            if [[ $iso_size -gt 100000000 ]]; then  # > 100MB
                # Additional content validation
                log_info "Validating ISO content integrity..."
                
                # Check if we can examine the ISO structure
                local validation_failed=false
                if command -v isoinfo &>/dev/null; then
                    # Check for essential files
                    if ! isoinfo -l -i "$output_iso" | grep -q "CPIO.GZ"; then
                        log_warn "cpio.gz not found in ISO"
                        validation_failed=true
                    fi
                    
                    if ! isoinfo -l -i "$output_iso" | grep -q "EFI.IMG"; then
                        log_warn "efi.img not found in ISO"
                        validation_failed=true
                    fi
                    
                    if ! isoinfo -l -i "$output_iso" | grep -q "VMLINUZ"; then
                        log_warn "vmlinuz not found in ISO"
                        validation_failed=true
                    fi
                fi
                
                if [[ "$validation_failed" == "false" ]]; then
                    log_pass "ISO creation successful (size: $((iso_size / 1024 / 1024))MB, content validated)"
                    
                    # Store ISO path for boot testing
                    echo "$output_iso" > "$TEST_DIR/test_iso_path"
                    return 0
                else
                    log_fail "ISO created but content validation failed"
                    return 1
                fi
            else
                log_fail "ISO created but size too small: $((iso_size / 1024 / 1024))MB"
                return 1
            fi
        else
            log_fail "ISO creation completed but file not found"
            return 1
        fi
    else
        log_fail "ISO creation failed"
        return 1
    fi
}

# Test 6: ISO structure validation
test_iso_structure() {
    log_test "Testing ISO structure validation"
    
    if [[ ! -f "$TEST_DIR/test_iso_path" ]]; then
        log_warn "No test ISO available, skipping structure validation"
        return 0
    fi
    
    local test_iso
    test_iso=$(cat "$TEST_DIR/test_iso_path")
    
    if [[ ! -f "$test_iso" ]]; then
        log_warn "Test ISO not found, skipping structure validation"
        return 0
    fi
    
    # Test ISO file type
    local file_type
    file_type=$(file "$test_iso" 2>/dev/null || echo "unknown")
    if [[ "$file_type" =~ ISO ]]; then
        log_pass "ISO file type validation passed"
    else
        log_fail "ISO file type validation failed: $file_type"
        return 1
    fi
    
    # Test ISO content if isoinfo is available
    if command -v isoinfo &>/dev/null; then
        if isoinfo -l -i "$test_iso" | grep -q "flatcar"; then
            log_pass "ISO contains expected Flatcar content"
        else
            log_fail "ISO missing expected Flatcar content"
            return 1
        fi
    else
        log_warn "isoinfo not available, skipping detailed structure validation"
    fi
    
    return 0
}

# Test 7: BIOS boot test with QEMU
test_bios_boot() {
    log_test "Testing BIOS boot with QEMU"
    
    if ! command -v qemu-system-x86_64 &>/dev/null; then
        log_warn "QEMU not available, skipping BIOS boot test"
        return 0
    fi
    
    if [[ ! -f "$TEST_DIR/test_iso_path" ]]; then
        log_warn "No test ISO available, skipping BIOS boot test"
        return 0
    fi
    
    local test_iso
    test_iso=$(cat "$TEST_DIR/test_iso_path")
    
    if [[ ! -f "$test_iso" ]]; then
        log_warn "Test ISO not found, skipping BIOS boot test"
        return 0
    fi
    
    log_info "Starting QEMU BIOS boot test (timeout: ${QEMU_TIMEOUT}s)"
    
    # Start QEMU in background and capture output
    local qemu_log="$TEST_DIR/qemu_bios.log"
    timeout $QEMU_TIMEOUT qemu-system-x86_64 \
        -m $QEMU_MEMORY \
        -cdrom "$test_iso" \
        -boot d \
        -nographic \
        -serial stdio \
        -monitor none \
        -no-reboot \
        > "$qemu_log" 2>&1 &
    
    local qemu_pid=$!
    
    # Wait for boot or timeout
    local boot_detected=false
    local start_time=$(date +%s)
    
    while [[ $(($(date +%s) - start_time)) -lt $QEMU_TIMEOUT ]]; do
        if kill -0 $qemu_pid 2>/dev/null; then
            if grep -q "Piccolo\|flatcar\|login:" "$qemu_log" 2>/dev/null; then
                boot_detected=true
                break
            fi
            sleep 2
        else
            break
        fi
    done
    
    # Cleanup QEMU
    if kill -0 $qemu_pid 2>/dev/null; then
        kill $qemu_pid 2>/dev/null || true
        wait $qemu_pid 2>/dev/null || true
    fi
    
    if [[ "$boot_detected" == "true" ]]; then
        log_pass "BIOS boot test successful"
        return 0
    else
        log_fail "BIOS boot test failed or timed out"
        log_info "QEMU log excerpt:"
        tail -20 "$qemu_log" 2>/dev/null | sed 's/^/  /' || true
        return 1
    fi
}

# Test 8: UEFI boot test with QEMU
test_uefi_boot() {
    log_test "Testing UEFI boot with QEMU"
    
    if ! command -v qemu-system-x86_64 &>/dev/null; then
        log_warn "QEMU not available, skipping UEFI boot test"
        return 0
    fi
    
    # Check for OVMF (UEFI firmware)
    local ovmf_code=""
    local ovmf_vars=""
    
    # Common OVMF locations
    for path in "/usr/share/OVMF/OVMF_CODE.fd" "/usr/share/ovmf/OVMF.fd"; do
        if [[ -f "$path" ]]; then
            ovmf_code="$path"
            break
        fi
    done
    
    for path in "/usr/share/OVMF/OVMF_VARS.fd" "/usr/share/ovmf/OVMF_VARS.fd"; do
        if [[ -f "$path" ]]; then
            ovmf_vars="$path"
            break
        fi
    done
    
    if [[ -z "$ovmf_code" || -z "$ovmf_vars" ]]; then
        log_warn "OVMF UEFI firmware not found, skipping UEFI boot test"
        return 0
    fi
    
    if [[ ! -f "$TEST_DIR/test_iso_path" ]]; then
        log_warn "No test ISO available, skipping UEFI boot test"
        return 0
    fi
    
    local test_iso
    test_iso=$(cat "$TEST_DIR/test_iso_path")
    
    if [[ ! -f "$test_iso" ]]; then
        log_warn "Test ISO not found, skipping UEFI boot test"
        return 0
    fi
    
    # Create temporary VARS file
    local temp_vars="$TEST_DIR/OVMF_VARS_temp.fd"
    cp "$ovmf_vars" "$temp_vars"
    
    log_info "Starting QEMU UEFI boot test (timeout: ${QEMU_TIMEOUT}s)"
    
    # Start QEMU in UEFI mode
    local qemu_log="$TEST_DIR/qemu_uefi.log"
    timeout $QEMU_TIMEOUT qemu-system-x86_64 \
        -m $QEMU_MEMORY \
        -cdrom "$test_iso" \
        -drive if=pflash,format=raw,readonly=on,file="$ovmf_code" \
        -drive if=pflash,format=raw,file="$temp_vars" \
        -boot d \
        -nographic \
        -serial stdio \
        -monitor none \
        -no-reboot \
        > "$qemu_log" 2>&1 &
    
    local qemu_pid=$!
    
    # Wait for boot or timeout
    local boot_detected=false
    local start_time=$(date +%s)
    
    while [[ $(($(date +%s) - start_time)) -lt $QEMU_TIMEOUT ]]; do
        if kill -0 $qemu_pid 2>/dev/null; then
            if grep -q "Piccolo\|flatcar\|login:" "$qemu_log" 2>/dev/null; then
                boot_detected=true
                break
            fi
            sleep 2
        else
            break
        fi
    done
    
    # Cleanup QEMU
    if kill -0 $qemu_pid 2>/dev/null; then
        kill $qemu_pid 2>/dev/null || true
        wait $qemu_pid 2>/dev/null || true
    fi
    
    # Cleanup temp files
    rm -f "$temp_vars"
    
    if [[ "$boot_detected" == "true" ]]; then
        log_pass "UEFI boot test successful"
        return 0
    else
        log_fail "UEFI boot test failed or timed out"
        log_info "QEMU log excerpt:"
        tail -20 "$qemu_log" 2>/dev/null | sed 's/^/  /' || true
        return 1
    fi
}

# Test 9: TPM measurement capability test
test_tpm_measurement() {
    log_test "Testing TPM measurement capability"
    
    if [[ ! -f "$TEST_DIR/test_iso_path" ]]; then
        log_warn "No test ISO available, skipping TPM test"
        return 0
    fi
    
    local test_iso
    test_iso=$(cat "$TEST_DIR/test_iso_path")
    
    # Extract and check GRUB configuration for TPM module
    if command -v isoinfo &>/dev/null; then
        local grub_config
        grub_config=$(isoinfo -i "$test_iso" -x /EFI/BOOT/GRUB.CFG 2>/dev/null || true)
        
        if [[ "$grub_config" =~ "insmod tpm" ]]; then
            log_pass "TPM module loading found in GRUB config"
            return 0
        else
            log_fail "TPM module loading not found in GRUB config"
            return 1
        fi
    else
        log_warn "Cannot extract GRUB config without isoinfo, assuming TPM support present"
        return 0
    fi
}

# Test 10: Cleanup verification
test_cleanup() {
    log_test "Testing cleanup functionality"
    
    # Test that create_piccolo_iso.sh cleans up properly after failure
    local temp_work_dir="/tmp/test_cleanup_work"
    mkdir -p "$temp_work_dir"
    
    # Simulate failure by providing invalid input and check cleanup
    if ! ./create_piccolo_iso.sh \
        --source /nonexistent \
        --output /tmp/test.iso \
        --version test \
        --work-dir "$temp_work_dir" 2>/dev/null; then
        
        # Check if work directory was cleaned up
        if [[ ! -d "$temp_work_dir" ]]; then
            log_pass "Cleanup verification successful"
            return 0
        else
            log_fail "Work directory not cleaned up after failure"
            rm -rf "$temp_work_dir" 2>/dev/null || true
            return 1
        fi
    else
        log_fail "Expected failure did not occur"
        rm -rf "$temp_work_dir" 2>/dev/null || true
        return 1
    fi
}

# Main test execution
run_all_tests() {
    log_info "Starting TDD test suite for Piccolo OS ISO creation"
    log_info "Test script version: $TEST_SCRIPT_VERSION"
    
    # Setup
    setup_test_environment
    trap cleanup_test_environment EXIT
    
    # Execute tests
    test_dependencies || true
    test_argument_parsing || true
    test_input_validation || true
    test_only_mode || true
    test_iso_creation_with_real_image || true
    test_iso_structure || true
    test_bios_boot || true
    test_uefi_boot || true
    test_tpm_measurement || true
    test_cleanup || true
    
    # Report results
    echo
    log_info "Test Results Summary:"
    echo "=================="
    for result in "${TEST_RESULTS[@]}"; do
        echo "  $result"
    done
    echo
    log_info "Tests Passed: $TESTS_PASSED"
    log_info "Tests Failed: $TESTS_FAILED"
    log_info "Total Tests: $((TESTS_PASSED + TESTS_FAILED))"
    
    if [[ $TESTS_FAILED -eq 0 ]]; then
        log_pass "All tests passed! 🎉"
        return 0
    else
        log_fail "Some tests failed"
        return 1
    fi
}

# Usage information
usage() {
    cat << EOF
test_piccolo_iso.sh - TDD Testing Framework for Piccolo OS ISO Creation

USAGE:
    $0 [OPTIONS]

OPTIONS:
    --help              Show this help message
    --quick             Run only quick tests (no boot testing)
    --boot-only         Run only boot tests
    --clean             Clean test environment and exit

EXAMPLES:
    # Run all tests
    ./test_piccolo_iso.sh

    # Run quick tests only
    ./test_piccolo_iso.sh --quick

    # Run boot tests only
    ./test_piccolo_iso.sh --boot-only

DEPENDENCIES:
    Required: Same as create_piccolo_iso.sh
    Optional: qemu-system-x86_64, isoinfo, OVMF UEFI firmware

EOF
}

# Argument parsing
main() {
    case "${1:-}" in
        --help|-h)
            usage
            exit 0
            ;;
        --quick)
            log_info "Running quick tests only"
            setup_test_environment
            trap cleanup_test_environment EXIT
            test_dependencies || true
            test_argument_parsing || true
            test_input_validation || true
            test_only_mode || true
            ;;
        --boot-only)
            log_info "Running boot tests only"
            setup_test_environment
            trap cleanup_test_environment EXIT
            test_bios_boot || true
            test_uefi_boot || true
            ;;
        --clean)
            cleanup_test_environment
            log_info "Test environment cleaned"
            exit 0
            ;;
        "")
            run_all_tests
            ;;
        *)
            log_fail "Unknown option: $1"
            usage
            exit 1
            ;;
    esac
}

# Execute main function if script is run directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi