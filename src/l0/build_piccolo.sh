#!/bin/bash
#
# Piccolo OS - Production Build Script v1.8 (Final Documented)
#
# Author: Piccolo Cofounder Team
# Date: July 30, 2025
#
# Description:
# This script is the single source of truth for manufacturing Piccolo OS. It
# automates the entire end-to-end process of building a custom, hardened
# version of Flatcar Container Linux that includes the 'piccolod' service.
#
# The script produces two primary artifacts for each release:
# 1. piccolo-os-update-[VERSION].raw.gz: A compressed raw disk image used for
#    all Over-the-Air (OTA) updates to existing devices.
# 2. piccolo-os-live-[VERSION].iso: A bootable ISO image used for recovery
#    and for new installations on community-provided hardware.
#
# This script is designed to be run in a CI/CD environment or on a developer's
# local machine.
#

# ---
# Script Configuration and Safety
# ---
# 'set -e' ensures the script exits immediately if any command fails.
# 'set -u' treats unset variables as an error.
# 'set -o pipefail' ensures that a pipeline command fails if any of its components fail.
set -euo pipefail
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)

# ---
# Load Build Configuration from piccolo.env
# ---
# All environment-specific variables (URLs, checksums, GPG keys) are stored
# in a separate piccolo.env file to keep this script clean.
if [ ! -f "${SCRIPT_DIR}/piccolo.env" ]; then
    echo "Error: Build environment file 'piccolo.env' not found." >&2
    exit 1
fi
# shellcheck source=piccolo.env
source "${SCRIPT_DIR}/piccolo.env"

# ---
# Helper Functions
# ---

# Centralized logging function for consistent output.
log() {
    echo "[$(date +'%Y-%m-%d %H:%M:%S')] $*"
}

# Prints usage information and exits.
usage() {
    echo "Usage: $0 --version <VERSION> --binary-path <PATH_TO_PICCOLOD>"
    exit 1
}

# Verifies that all required command-line tools are installed on the host.
check_dependencies() {
    log "Checking for required dependencies..."
    local deps=("git" "docker" "curl" "sha256sum" "gpg" "jq" "file")
    for dep in "${deps[@]}"; do
        if ! command -v "$dep" &> /dev/null; then
            log "Error: Required dependency '$dep' is not installed." >&2
            exit 1
        fi
    done
    log "All dependencies are installed."
}

# Verifies that the provided binary is a 64-bit x86 executable, matching our build target.
# This is a critical sanity check to prevent building a non-functional image.
verify_binary_architecture() {
    local binary_path="$1"
    local target_arch="x86-64" # Corresponds to the 'amd64-usr' board
    
    log "Verifying architecture of ${binary_path}..."
    local file_output
    file_output=$(file "$binary_path")

    if ! echo "$file_output" | grep -q "ELF 64-bit LSB executable"; then
        log "Error: Provided binary is not a 64-bit ELF executable." >&2; exit 1;
    fi
    if ! echo "$file_output" | grep -q "${target_arch}"; then
        log "Error: Binary architecture does not match target. Expected ${target_arch}." >&2; exit 1;
    fi
    log "Binary architecture is correct (x86-64)."
}

# ---
# Main Script Logic
# ---
main() {
    # ---
    # Step 0: Parse Arguments and Check Dependencies
    # ---
    if [ "$#" -eq 0 ]; then usage; fi
    local PICCOLO_VERSION=""
    local PICCOLOD_BINARY_PATH=""
    while [ "$#" -gt 0 ]; do
        case "$1" in
            --version) PICCOLO_VERSION="$2"; shift 2;;
            --binary-path) PICCOLOD_BINARY_PATH="$2"; shift 2;;
            *) usage;;
        esac
    done
    if [ -z "${PICCOLO_VERSION:-}" ] || [ -z "${PICCOLOD_BINARY_PATH:-}" ]; then usage; fi
    if [ ! -f "$PICCOLOD_BINARY_PATH" ]; then log "Error: piccolod binary not found" >&2; exit 1; fi
    check_dependencies
    verify_binary_architecture "$PICCOLOD_BINARY_PATH"

    # ---
    # Step 1: Prepare the Build Environment
    # ---
    log "### Step 1: Preparing the build environment..."
    # All build artifacts are placed in a top-level 'build' directory to keep the project root clean.
    local top_build_dir="${SCRIPT_DIR}/build"
    local work_dir="${top_build_dir}/work-${PICCOLO_VERSION}"
    local output_dir="${top_build_dir}/output/${PICCOLO_VERSION}"
    local scripts_repo_dir="${work_dir}/scripts"
    mkdir -p "$work_dir"
    mkdir -p "$output_dir"
    
    if [ ! -d "$scripts_repo_dir" ]; then
        log "Cloning Flatcar scripts repository..."
        git clone https://github.com/flatcar/scripts.git "$scripts_repo_dir"
    fi
    
    # Use pushd/popd for robust directory navigation.
    pushd "$scripts_repo_dir" > /dev/null
    log "Resetting scripts repository to a clean state..."
    git reset --hard && git clean -fd
    log "Fetching latest tags from Flatcar repository..."
    git fetch --prune --prune-tags --tags --force
    # Automatically find and check out the latest stable tag for reproducibility.
    LATEST_STABLE_TAG=$(git tag -l | grep -E 'stable-[0-9.]+$' | sort -V | tail -n 1)
    if [ -z "$LATEST_STABLE_TAG" ]; then log "Error: Could not find any stable release tags." >&2; exit 1; fi
    log "Checking out latest stable release: $LATEST_STABLE_TAG"
    git checkout "$LATEST_STABLE_TAG"
    popd > /dev/null

    # ---
    # Step 2: Create Custom Piccolo OS Overlay
    # ---
    log "### Step 2: Creating custom overlay..."
    # This overlay contains all our custom modifications to the base Flatcar OS.
    local overlay_dir="${scripts_repo_dir}/src/third_party/coreos-overlay"
    local ebuild_dir="${overlay_dir}/app-piccolo/piccolod"
    local profiles_dir="${overlay_dir}/profiles"
    local metadata_dir="${overlay_dir}/metadata"
    
    mkdir -p "${ebuild_dir}/files"
    cp "$(realpath "$PICCOLOD_BINARY_PATH")" "${ebuild_dir}/files/piccolod-${PICCOLO_VERSION}"
    
    # Create the ebuild file, which tells the Portage build system how to install our package.
    cat > "${ebuild_dir}/piccolod-${PICCOLO_VERSION}.ebuild" << EOF
EAPI=7
DESCRIPTION="The core service for the Piccolo OS ecosystem"
HOMEPAGE="https://github.com/AtDexters-Lab/piccolo-os"
# SRC_URI is empty as we are not downloading sources.
SRC_URI=""
LICENSE="Piccolo-EULA"
SLOT="0"
KEYWORDS="~amd64"
RESTRICT="strip"

# src_unpack copies our pre-compiled binary into the build's work directory.
src_unpack() { cp "\${FILESDIR}/\${P}" "\${WORKDIR}/\${P}"; }
# src_compile is empty as there's nothing to compile.
src_compile() { :; }
# src_install uses the 'dobin' helper to install the binary into /usr/bin.
src_install() { dobin "\${WORKDIR}/\${P}"; }
EOF

    # Create the necessary metadata to make our overlay a valid Portage repository.
    mkdir -p "$profiles_dir"
    echo "app-piccolo" > "${profiles_dir}/categories"
    mkdir -p "$metadata_dir"
    echo "masters = portage-stable" > "${metadata_dir}/layout.conf"
    echo "coreos-overlay" > "${profiles_dir}/repo_name"
    log "Custom overlay created successfully."

    # ---
    # Step 3: Build All Artifacts Inside the SDK
    # ---
    log "### Step 3: Starting the SDK to build all artifacts..."
    pushd "$scripts_repo_dir" > /dev/null
    
    # This heredoc contains the sequence of commands executed inside the SDK container.
    ./run_sdk_container -- /bin/bash -s <<EOF
set -euxo pipefail;

# 1. Generate the manifest for our custom package so Portage can find it.
ebuild src/third_party/coreos-overlay/app-piccolo/piccolod/piccolod-${PICCOLO_VERSION}.ebuild manifest;

# 2. Add our package to the system's core package set. This is the robust way
#    to ensure it's included in the final image.
echo "app-piccolo/piccolod" >> "/etc/portage/make.profile/packages";

# 3. Build all OS packages, including our new dependency.
./build_packages --board='amd64-usr';

# 4. Assemble the final, hardened production image.
./build_image --board='amd64-usr';

# 5. Find the output directory created by the build_image step.
LATEST_BUILD_DIR"../build/images/amd64-usr/latest"

# 6. Use the SDK's official tool to create a bootable ISO from the image we just built.
./image_to_vm.sh --from="\${LATEST_BUILD_DIR}" --format=iso --to="./iso_out"

cp "\${LATEST_BUILD_DIR}/flatcar_production_update.bin.bz2" "./iso_out/flatcar_production_update.bin.bz2";

EOF
    popd > /dev/null

    log "### Finished building all artifacts!"
    local latest_build_dir="${scripts_repo_dir}/iso_out"
    
    # ---
    # Step 4: Package and Sign the Update Image
    # ---
    log "### Step 4: Packaging and signing the update image..."
    # The build process generates a pre-compressed update file. We use this directly for efficiency.
    local bz2_update_image="${latest_build_dir}/flatcar_production_update.bin.bz2"
    log "Repackaging update image to .gz format for our update server's consistency..."
    bzip2 -cdk "$bz2_update_image" | gzip -c > "${output_dir}/piccolo-os-update-${PICCOLO_VERSION}.raw.gz"

    gpg --detach-sign --armor -u "$GPG_SIGNING_KEY_ID" "${output_dir}/piccolo-os-update-${PICCOLO_VERSION}.raw.gz"
    log "Update image signed."

    # ---
    # Step 5: Package the Live ISO
    # ---
    log "### Step 5: Packaging the bootable Piccolo Live ISO..."
    # The ISO was created inside the container and is available on the host.
    local iso_output_dir="${scripts_repo_dir}/iso_out"
    local generated_iso_path
    generated_iso_path=$(find "$iso_output_dir" -name "*.iso")
    mv "$generated_iso_path" "${output_dir}/piccolo-os-live-${PICCOLO_VERSION}.iso"

    local generated_runner_path
    generated_runner_path=$(find "$iso_output_dir" -name "*.sh")
    mv "$generated_runner_path" "${output_dir}/run-piccolo-live-${PICCOLO_VERSION}.sh"
    chmod +x "${output_dir}/run-piccolo-live-${PICCOLO_VERSION}.sh"

    log "Live ISO and test runner created successfully."

    # ---
    # Step 6: Final Output
    # ---
    log "✅ Build complete!"
    log "Your artifacts are located in: ${output_dir}"
    ls -l "${output_dir}"
}

# Run the main function with all arguments passed to the script
main "$@"
