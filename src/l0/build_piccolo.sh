#!/bin/bash
#
# Piccolo OS - Production Build Script v17.0 (Refactored for Maintainability)
#
# This script builds a custom Flatcar Linux OS image with the piccolod daemon
# integrated as a system service. The build process follows these steps:
# 1. Prepare build environment and clone Flatcar scripts
# 2. Create custom ebuild package for piccolod
# 3. Build OS image inside Flatcar SDK container
# 4. Package and sign final artifacts
#

# ---
# Script Configuration and Safety
# ---
set -euo pipefail # Exit on error, unset var, or pipe failure
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)

# ---
# Build Configuration Constants
# ---
readonly FLATCAR_REPO_URL="https://github.com/flatcar/scripts.git"
readonly BOARD_NAME="amd64-usr"
readonly EBUILD_CATEGORY="app-misc"
readonly EBUILD_PKG_NAME="piccolod-bin"
readonly HTTP_PORT="8080"

# ---
# Load and Validate Build Configuration
# ---
load_build_config() {
    local config_file="${SCRIPT_DIR}/piccolo.env"
    if [ ! -f "$config_file" ]; then
        log "ERROR: Build environment file 'piccolo.env' not found" >&2
        exit 1
    fi
    # shellcheck source=piccolo.env
    source "$config_file"

    # Validate required environment variables
    local required_vars=("GPG_SIGNING_KEY_ID" "PICCOLO_UPDATE_SERVER" "PICCOLO_UPDATE_GROUP")
    for var in "${required_vars[@]}"; do
        if [ -z "${!var:-}" ]; then
            log "ERROR: Required environment variable $var not set in piccolo.env" >&2
            exit 1
        fi
    done
    log "Build configuration loaded and validated"
}

# ---
# Utility Functions
# ---
log() {
    echo "[$(date +'%Y-%m-%d %H:%M:%S')] $*"
}

log_step() {
    log "### $*"
}

usage() {
    cat << EOF
Usage: $0 --version <VERSION> --binary-path <PATH_TO_PICCOLOD>

Arguments:
  --version       Version string for this build (e.g., '1.0.0')
  --binary-path   Absolute path to the compiled piccolod binary

Example:
  $0 --version 1.0.0 --binary-path /path/to/piccolod
EOF
    exit 1
}

check_dependencies() {
    log "Checking for required dependencies..."
    local deps=("git" "docker" "gpg" "numfmt" "stat")
    local missing_deps=()
    
    for dep in "${deps[@]}"; do
        if ! command -v "$dep" &> /dev/null; then
            missing_deps+=("$dep")
        fi
    done
    
    if [ ${#missing_deps[@]} -gt 0 ]; then
        log "ERROR: Missing required dependencies: ${missing_deps[*]}" >&2
        log "Please install them and try again" >&2
        exit 1
    fi
    
    log "All dependencies are installed"
}

validate_arguments() {
    local version="$1"
    local binary_path="$2"
    
    if [ -z "$version" ] || [ -z "$binary_path" ]; then
        log "ERROR: Both --version and --binary-path are required" >&2
        usage
    fi
    
    if [ ! -f "$binary_path" ]; then
        log "ERROR: piccolod binary not found at $binary_path" >&2
        exit 1
    fi
    
    if [ ! -x "$binary_path" ]; then
        log "ERROR: piccolod binary at $binary_path is not executable" >&2
        exit 1
    fi
    
    log "Arguments validated successfully" >&2
}

# ---
# Template Generation Functions
# ---
generate_systemd_service() {
    cat << 'EOF'
[Unit]
Description=Piccolo Daemon
After=network-online.target
Wants=network-online.target
StartLimitIntervalSec=0

[Service]
Type=simple
ExecStart=/usr/bin/piccolod
Restart=always
RestartSec=5s
User=root
Group=root
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF
}

generate_systemd_preset() {
    echo "enable piccolod.service"
}

generate_update_config() {
    cat << EOF
GROUP=${PICCOLO_UPDATE_GROUP}
SERVER=${PICCOLO_UPDATE_SERVER}
EOF
}

generate_ebuild_file() {
    local version="$1"
    cat << EOF
# Copyright 2024 Piccolo Space Inc.
# Distributed under the terms of the Piccolo EULA

EAPI=8

inherit systemd

DESCRIPTION="The core service for the Piccolo OS ecosystem (pre-compiled)"
HOMEPAGE="https://piccolospace.com"
SRC_URI=""
LICENSE="Piccolo-EULA"
SLOT="0"
KEYWORDS="~amd64 ~arm64"
RESTRICT="strip"

# Disable QA checks for prebuilt binaries
QA_PREBUILT="usr/bin/piccolod"

# Dependencies
RDEPEND="
    sys-apps/systemd
"

S="\${WORKDIR}"

src_install() {
    # Install the binary
    dobin "\${FILESDIR}/piccolod"
    
    # Install systemd unit file
    systemd_dounit "\${FILESDIR}/piccolod.service"
    
    # Install systemd preset file to enable the service
    insinto /usr/lib/systemd/system-preset
    doins "\${FILESDIR}/90-piccolod.preset"
    
    # Install configuration file
    insinto /etc/flatcar
    doins "\${FILESDIR}/update.conf"
    
    # Create log directory
    keepdir /var/log/piccolod
    fowners root:root /var/log/piccolod
    fperms 0755 /var/log/piccolod
}

pkg_postinst() {
    # Enable the service by default
    systemctl enable piccolod.service 
    
    elog "Piccolo daemon has been installed and enabled."
    elog "Configuration file: /etc/flatcar/update.conf"
    elog "Service will start automatically on next boot."
    elog "To start now: systemctl start piccolod.service"
}

pkg_prerm() {
    # Stop and disable service before removal
    systemctl stop piccolod.service || true
    systemctl disable piccolod.service || true
}
EOF
}

generate_license_file() {
    cat << 'EOF'
Piccolo Space Inc. End User License Agreement
Copyright (c) 2024 Piccolo Space Inc. All rights reserved.
This software is proprietary and confidential.
EOF
}

# ---
# Build Environment Functions
# ---
parse_arguments() {
    local version=""
    local binary_path=""
    
    if [ "$#" -eq 0 ]; then usage; fi
    
    while [ "$#" -gt 0 ]; do
        case "$1" in
            --version) version="$2"; shift 2;;
            --binary-path) binary_path="$2"; shift 2;;
            --help|-h) usage;;
            *) log "ERROR: Unknown argument: $1" >&2; usage;;
        esac
    done
    
    validate_arguments "$version" "$binary_path"
    echo "$version|$binary_path"
}

setup_build_directories() {
    local version="$1"
    local top_build_dir="${SCRIPT_DIR}/build"
    local work_dir="${top_build_dir}/work-${version}"
    local output_dir="${top_build_dir}/output/${version}"
    
    mkdir -p "$work_dir" "$output_dir"
    echo "$work_dir|$output_dir"
}

clone_flatcar_scripts() {
    local scripts_repo_dir="$1"
    
    if [ ! -d "$scripts_repo_dir" ]; then
        log "Cloning Flatcar scripts repository..."
        git clone "$FLATCAR_REPO_URL" "$scripts_repo_dir"
    fi
    
    pushd "$scripts_repo_dir" > /dev/null
    log "Resetting scripts repository to a clean state..."
    git reset --hard && git clean -fd
    log "Fetching latest tags from Flatcar repository..."
    git fetch --prune --prune-tags --tags --force origin
    
    local latest_stable_tag
    latest_stable_tag=$(git tag -l | grep -E 'stable-[0-9.]+$' | sort -V | tail -n 1)
    if [ -z "$latest_stable_tag" ]; then 
        log "ERROR: Could not find any stable release tags" >&2
        exit 1
    fi
    
    log "Checking out latest stable release: $latest_stable_tag"
    git checkout "$latest_stable_tag"
    popd > /dev/null
}

create_ebuild_package() {
    local scripts_repo_dir="$1"
    local version="$2"
    local binary_path="$3"
    
    local overlay_dir="${scripts_repo_dir}/sdk_container/src/third_party/coreos-overlay"
    local ebuild_dir="${overlay_dir}/${EBUILD_CATEGORY}/${EBUILD_PKG_NAME}"
    
    log "Creating ebuild directory structure..."
    mkdir -p "${ebuild_dir}/files"
    
    # Copy binary and make it executable
    log "Installing piccolod binary..."
    cp "$(realpath "$binary_path")" "${ebuild_dir}/files/piccolod"
    chmod +x "${ebuild_dir}/files/piccolod"
    
    # Generate configuration files
    log "Generating systemd service file..."
    generate_systemd_service > "${ebuild_dir}/files/piccolod.service"
    
    log "Generating systemd preset file..."
    generate_systemd_preset > "${ebuild_dir}/files/90-piccolod.preset"
    
    log "Generating update configuration..."
    generate_update_config > "${ebuild_dir}/files/update.conf"
    
    # Create the ebuild file
    log "Generating ebuild file..."
    generate_ebuild_file "$version" > "${ebuild_dir}/${EBUILD_PKG_NAME}-${version}.ebuild"
    
    # Add custom license to the overlay
    log "Installing custom license..."
    mkdir -p "${overlay_dir}/licenses"
    generate_license_file > "${overlay_dir}/licenses/Piccolo-EULA"
    
    log "Custom ebuild created successfully in ${ebuild_dir}"
}

generate_sdk_build_script() {
    local version="$1"
    local update_group="$2"
    
    cat << EOF
set -euxo pipefail

PICCOLO_VERSION="$version"
EBUILD_CATEGORY="$EBUILD_CATEGORY"
EBUILD_PKG_NAME="$EBUILD_PKG_NAME"
PICCOLO_UPDATE_GROUP="$update_group"
BOARD_NAME="$BOARD_NAME"

echo "=== Building Piccolo OS v\${PICCOLO_VERSION} ==="

# Set up portage license configuration
echo "Setting up custom license configuration..."
setup_license_config() {
    local license_entry="=\${EBUILD_CATEGORY}/\${EBUILD_PKG_NAME}-\${PICCOLO_VERSION} Piccolo-EULA"
    
    # Host license configuration
    if [ -d "/etc/portage/package.license" ]; then
        echo "\$license_entry" | sudo tee /etc/portage/package.license/01-piccolo > /dev/null
    else
        echo "\$license_entry" | sudo tee -a /etc/portage/package.license > /dev/null
    fi
    
    # Board-specific license configuration
    local board_license_dir="/build/\${BOARD_NAME}/etc/portage/package.license"
    sudo mkdir -p "/build/\${BOARD_NAME}/etc/portage"
    if [ -d "\${board_license_dir}" ]; then
        echo "\$license_entry" | sudo tee "\${board_license_dir}/01-piccolo" > /dev/null
    elif [ -f "/build/\${BOARD_NAME}/etc/portage/package.license" ]; then
        echo "\$license_entry" | sudo tee -a "/build/\${BOARD_NAME}/etc/portage/package.license" > /dev/null
    else
        echo "\$license_entry" | sudo tee "/build/\${BOARD_NAME}/etc/portage/package.license" > /dev/null
    fi
}

setup_license_config

# Generate manifest for the ebuild
echo "Generating manifest for ebuild..."
ebuild_path="sdk_container/src/third_party/coreos-overlay/\${EBUILD_CATEGORY}/\${EBUILD_PKG_NAME}/\${EBUILD_PKG_NAME}-\${PICCOLO_VERSION}.ebuild"
ebuild "\${ebuild_path}" manifest

# Find and modify the coreos base ebuild
echo "Locating coreos base ebuild..."
coreos_ebuild_path=\$(find . -path '*/coreos-base/coreos/coreos-0.0.1.ebuild' | head -n 1)
if [ -z "\${coreos_ebuild_path}" ]; then
    echo "FATAL: Could not find the coreos-0.0.1.ebuild file" >&2
    exit 1
fi
echo "Found coreos ebuild at: \${coreos_ebuild_path}"

# Add our package as a dependency to the base system
dep_string="\${EBUILD_CATEGORY}/\${EBUILD_PKG_NAME}"
if ! grep -q "\${dep_string}" "\${coreos_ebuild_path}"; then
    echo "Adding \${dep_string} as an RDEPEND to the coreos package..."
    sed -i "/^\"/i \\\\\t\${dep_string}" "\${coreos_ebuild_path}"
    echo "Dependency added successfully"
else
    echo "Dependency already exists in coreos package"
fi

# Handle autounmask requirements
echo "Running emerge with --autounmask-write..."
emerge-\${BOARD_NAME} --autounmask --autounmask-write "=\${EBUILD_CATEGORY}/\${EBUILD_PKG_NAME}-\${PICCOLO_VERSION}" || {
    echo "Running dispatch-conf to apply autounmask changes..."
    yes | dispatch-conf || true
}

# Build and verify our package
echo "Building \${EBUILD_PKG_NAME} package..."
emerge-\${BOARD_NAME} --ask=n "=\${EBUILD_CATEGORY}/\${EBUILD_PKG_NAME}-\${PICCOLO_VERSION}"

# Verify package installation
echo "Verifying package installation..."
verify_installation() {
    local build_root="/build/\${BOARD_NAME}"
    if [ ! -f "\${build_root}/usr/bin/piccolod" ]; then
        echo "ERROR: piccolod binary not found in build root" >&2
        return 1
    fi
    if [ ! -f "\${build_root}/usr/lib/systemd/system/piccolod.service" ]; then
        echo "ERROR: piccolod.service not found in build root" >&2
        return 1
    fi
    echo "Package verification successful"
}

verify_installation

# Update the coreos base package
echo "Updating coreos base package..."
emerge-\${BOARD_NAME} --ask=n coreos-base/coreos

# Pre-flight dependency check
echo "Running dependency check..."
emerge-\${BOARD_NAME} -p --quiet coreos-base/coreos

# Build all packages
echo "Building all packages..."
./build_packages --board="\${BOARD_NAME}"

# Build the production image
echo "Building production image..."
./build_image --board="\${BOARD_NAME}" --group="\${PICCOLO_UPDATE_GROUP}" --image_compression_formats=gz prod

# Verify build artifacts
echo "Verifying build artifacts..."
latest_build_dir="./__build__/images/images/\${BOARD_NAME}/latest"
if [ -d "\${latest_build_dir}" ]; then
    echo "Build directory contents:"
    ls -la "\${latest_build_dir}/"
    
    if ls "\${latest_build_dir}"/*.bin* &>/dev/null; then
        echo "Update binary found"
    fi
fi

# Create bootable ISO
echo "Creating bootable ISO..."
./image_to_vm.sh --from="\${latest_build_dir}" --format=iso --board="\${BOARD_NAME}"

echo "=== Build completed successfully ==="
EOF
}

build_in_sdk_container() {
    local scripts_repo_dir="$1"
    local version="$2"
    local update_group="$3"
    
    log "Starting SDK container build process..."
    pushd "$scripts_repo_dir" > /dev/null
    
    # Generate and execute the build script inside the SDK
    generate_sdk_build_script "$version" "$update_group" | \
        ./run_sdk_container -- /bin/bash -s
    
    popd > /dev/null
    log "SDK container build completed"
}

package_and_sign_artifacts() {
    local scripts_repo_dir="$1"
    local output_dir="$2"
    local version="$3"
    
    log_step "Packaging and signing final artifacts" >&2
    
    local artifact_src_dir="${scripts_repo_dir}/__build__/images/images/${BOARD_NAME}/latest"
    local src_bin_gz="${artifact_src_dir}/flatcar_production_update.bin.gz"
    local src_iso="${artifact_src_dir}/flatcar_production_iso_image.iso"
    
    # Verify source artifacts exist
    if [ ! -f "$src_bin_gz" ] || [ ! -f "$src_iso" ]; then
        log "ERROR: Required build artifacts not found in ${artifact_src_dir}" >&2
        log "Available files:" >&2
        ls -la "${artifact_src_dir}/" >&2 || true
        exit 1
    fi
    
    # Define final artifact paths
    local final_raw_gz="${output_dir}/piccolo-os-update-${version}.raw.gz"
    local final_asc="${output_dir}/piccolo-os-update-${version}.raw.gz.asc"
    local final_iso="${output_dir}/piccolo-os-live-${version}.iso"
    
    # Copy artifacts to output directory
    log "Copying artifacts to output directory..." >&2
    cp "$src_bin_gz" "$final_raw_gz"
    cp "$src_iso" "$final_iso"
    
    # Sign the update artifact
    log "Signing update artifact with GPG key: ${GPG_SIGNING_KEY_ID}" >&2
    echo "y" | gpg --detach-sign --armor --output "${final_asc}" -u "$GPG_SIGNING_KEY_ID" "$final_raw_gz"
    
    # Verify signature
    log "Verifying GPG signature..." >&2
    gpg --verify "${final_asc}" "${final_raw_gz}"
    log "Signature verification successful" >&2
    
    # Return artifact paths for verification
    echo "$final_raw_gz|$final_asc|$final_iso"
}

verify_and_report_build() {
    local output_dir="$1"
    local artifacts="$2"
    
    log_step "Final verification and summary"
    
    # Parse artifact paths
    local final_raw_gz
    local final_asc
    local final_iso
    IFS='|' read -r final_raw_gz final_asc final_iso <<< "$artifacts"
    
    # Verify artifacts and display sizes
    log "Build artifact verification:"
    if [ -f "$final_raw_gz" ]; then
        echo "✅ Update image: $(stat -c%s "$final_raw_gz" | numfmt --to=iec-i --suffix=B)"
    else
        echo "❌ Update image: Not found"
        exit 1
    fi
    
    if [ -f "$final_iso" ]; then
        echo "✅ Live ISO: $(stat -c%s "$final_iso" | numfmt --to=iec-i --suffix=B)"
    else
        echo "❌ Live ISO: Not found"
        exit 1
    fi
    
    if [ -f "$final_asc" ]; then
        echo "✅ GPG Signature: Valid"
    else
        echo "❌ GPG Signature: Not found"
        exit 1
    fi
    
    log "✅ Build completed successfully!"
    log "Final artifacts located in: ${output_dir}"
    ls -lh "${output_dir}"
    
    log ""
    log "🚀 Next steps:"
    log "  1. Test the live ISO: ${final_iso}"
    log "  2. Deploy the update image: ${final_raw_gz}"
    log "  3. Verify signature: gpg --verify ${final_asc} ${final_raw_gz}"
}

# ---
# Main Script Logic
# ---
main() {
    # Parse and validate arguments
    local args
    args=$(parse_arguments "$@")
    local version
    local binary_path
    IFS='|' read -r version binary_path <<< "$args"
    
    # Load configuration and check dependencies
    load_build_config
    check_dependencies
    
    # Set up build environment
    log_step "Step 1: Preparing build environment"
    local dirs
    dirs=$(setup_build_directories "$version")
    local work_dir
    local output_dir
    IFS='|' read -r work_dir output_dir <<< "$dirs"
    
    local scripts_repo_dir="${work_dir}/scripts"
    clone_flatcar_scripts "$scripts_repo_dir"
    
    # Create custom ebuild package
    log_step "Step 2: Creating custom ebuild package"
    create_ebuild_package "$scripts_repo_dir" "$version" "$binary_path"
    
    # # Build OS image in SDK container
    # log_step "Step 3: Building OS image in SDK container"
    build_in_sdk_container "$scripts_repo_dir" "$version" "$PICCOLO_UPDATE_GROUP"
    
    # Package and sign final artifacts
    log_step "Step 4: Packaging and signing artifacts"
    local artifacts
    artifacts=$(package_and_sign_artifacts "$scripts_repo_dir" "$output_dir" "$version")
    
    # Final verification and reporting
    verify_and_report_build "$output_dir" "$artifacts"
}

# Execute main function if script is run directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi