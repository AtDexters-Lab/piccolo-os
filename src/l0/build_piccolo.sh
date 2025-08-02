#!/bin/bash
#
# Piccolo OS - Production Build Script v3.0 (Definitive)
#
# This version implements the definitive, canonical integration method for
# Flatcar Linux. It creates a repository configuration file on the host and
# bind-mounts it to the specific path (/mnt/host/source/config) that the
# early-stage 'update_chroot' script is hardcoded to look for. This ensures
# the custom overlay is available throughout the entire build process.
#

# ---
# Script Configuration and Safety
# ---
set -euo pipefail # Exit on error, unset var, or pipe failure
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)

# ---
# Load Build Configuration from piccolo.env
# ---
if [ ! -f "${SCRIPT_DIR}/piccolo.env" ]; then
    echo "Error: Build environment file 'piccolo.env' not found." >&2
    exit 1
fi
# shellcheck source=piccolo.env
source "${SCRIPT_DIR}/piccolo.env"

# ---
# Helper Functions
# ---
log() {
    echo "[$(date +'%Y-%m-%d %H:%M:%S')] $*"
}

usage() {
    echo "Usage: $0 --version <VERSION> --binary-path <PATH_TO_PICCOLOD>"
    exit 1
}

check_dependencies() {
    log "Checking for required dependencies..."
    local deps=("git" "docker" "curl" "sha256sum" "gpg")
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
    if [ ! -f "$PICCOLOD_BINARY_PATH" ]; then log "Error: piccolod binary not found at $PICCOLOD_BINARY_PATH" >&2; exit 1; fi
    check_dependencies

    # ---
    # Step 1: Prepare the Build Environment
    # ---
    log "### Step 1: Preparing the build environment..."
    local top_build_dir="${SCRIPT_DIR}/build"
    local work_dir="${top_build_dir}/work-${PICCOLO_VERSION}"
    local output_dir="${top_build_dir}/output/${PICCOLO_VERSION}"
    local scripts_repo_dir="${work_dir}/scripts"
    mkdir -p "$work_dir" "$output_dir"
    
    if [ ! -d "$scripts_repo_dir" ]; then
        log "Cloning Flatcar scripts repository..."
        git clone https://github.com/flatcar/scripts.git "$scripts_repo_dir"
    fi
    
    pushd "$scripts_repo_dir" > /dev/null
    log "Resetting scripts repository to a clean state..."
    git reset --hard && git clean -fd
    log "Fetching latest tags from Flatcar repository..."
    git fetch --prune --prune-tags --tags --force
    LATEST_STABLE_TAG=$(git tag -l | grep -E 'stable-[0-9.]+$' | sort -V | tail -n 1)
    if [ -z "$LATEST_STABLE_TAG" ]; then log "Error: Could not find any stable release tags." >&2; exit 1; fi
    log "Checking out latest stable release: $LATEST_STABLE_TAG"
    git checkout "$LATEST_STABLE_TAG"
    popd > /dev/null

    # ---
    # Step 2: Create Custom Piccolo OS Overlay and Repository Config
    # ---
    log "### Step 2: Creating custom overlay and repository config..."
    local overlay_dir="${scripts_repo_dir}/src/third_party/piccolo-overlay"
    local ebuild_dir="${overlay_dir}/app-piccolo/piccolod"
    
    mkdir -p "${ebuild_dir}/files"
    cp "$(realpath "$PICCOLOD_BINARY_PATH")" "${ebuild_dir}/files/piccolod-${PICCOLO_VERSION}"
    
    cat > "${ebuild_dir}/piccolod-${PICCOLO_VERSION}.ebuild" << EOF
EAPI=7
DESCRIPTION="The core service for the Piccolo OS ecosystem"
HOMEPAGE="https://piccolospace.com"
SRC_URI=""
LICENSE="Piccolo-EULA"
SLOT="0"
KEYWORDS="~amd64"
RESTRICT="strip"
src_unpack() { cp "\${FILESDIR}/\${P}" "\${WORKDIR}/\${P}"; }
src_compile() { :; }
src_install() { dobin "\${WORKDIR}/\${P}"; }
EOF

    mkdir -p "${overlay_dir}/metadata"
    echo "masters = portage-stable" > "${overlay_dir}/metadata/layout.conf"
    mkdir -p "${overlay_dir}/profiles"
    echo "piccolo-overlay" > "${overlay_dir}/profiles/repo_name"
    echo "app-piccolo" > "${overlay_dir}/profiles/categories"

    # Create the repository config file on the host in a 'config' directory.
    local repo_config_dir="${scripts_repo_dir}/sdk_container/config/portage/repos"
    mkdir -p "$repo_config_dir"
    cat > "${repo_config_dir}/piccolo.conf" << EOF
[piccolo-overlay]
location = /home/sdk/trunk/src/scripts/src/third_party/piccolo-overlay
priority = 50
masters = portage-stable
auto-sync = no
EOF
    log "Custom overlay and repo config created successfully."

    # ---
    # Step 3: Build All Artifacts Inside the SDK Container
    # ---
    log "### Step 3: Starting the SDK to build all artifacts..."
    pushd "$scripts_repo_dir" > /dev/null
    
    # Mount the host's 'config' directory to the path the SDK expects.
    ./run_sdk_container -m "${PWD}/config":/mnt/host/source/config:ro -- /bin/bash -s -- "${PICCOLO_VERSION}" << 'EOF'
set -euxo pipefail

PICCOLO_VERSION="$1"

# The build system will now automatically discover our overlay config.
# We still need to generate the manifest for our ebuild.
EBUILD_PATH="src/third_party/piccolo-overlay/app-piccolo/piccolod/piccolod-${PICCOLO_VERSION}.ebuild"
ebuild "${EBUILD_PATH}" manifest
echo "Manifest generated for piccolod."

# Find the coreos ebuild to modify.
COREOS_EBUILD_PATH=$(find . -path '*/coreos-base/coreos/coreos-0.0.1.ebuild' | head -n 1)
if [ -z "${COREOS_EBUILD_PATH}" ]; then
    echo "FATAL: Could not dynamically find the coreos-0.0.1.ebuild file." >&2
    exit 1
fi
echo "Found coreos ebuild at: ${COREOS_EBUILD_PATH}"

if ! grep -q "app-piccolo/piccolod" "${COREOS_EBUILD_PATH}"; then
    echo "Adding piccolod as an RDEPEND to the coreos package..."
    echo "RDEPEND=\"\${RDEPEND} app-piccolo/piccolod\"" >> "${COREOS_EBUILD_PATH}"
else
    echo "piccolod dependency already exists in coreos package."
fi

echo "Running ./build_packages..."
./build_packages --board='amd64-usr'

echo "Running ./build_image..."
./build_image --board='amd64-usr'

echo "Creating bootable ISO and copying artifacts..."
LATEST_BUILD_DIR="../build/images/amd64-usr/latest"
./image_to_vm.sh --from="${LATEST_BUILD_DIR}" --format=iso --to="./iso_out"
cp "${LATEST_BUILD_DIR}/flatcar_production_update.bin.bz2" "./iso_out/flatcar_production_update.bin.bz2"
EOF
    popd > /dev/null

    log "### Finished building all artifacts inside the SDK!"
    
    # ---
    # Step 4: Package and Sign the Update Image
    # ---
    log "### Step 4: Packaging and signing the update image..."
    local iso_output_dir="${scripts_repo_dir}/iso_out"
    local bz2_update_image="${iso_output_dir}/flatcar_production_update.bin.bz2"
    
    if [ ! -f "$bz2_update_image" ]; then
        log "Error: Update artifact not found in exchange directory: ${bz2_update_image}" >&2
        exit 1
    fi

    log "Repackaging update image to .gz format for our update server's consistency..."
    bzip2 -cdk "$bz2_update_image" | gzip -c > "${output_dir}/piccolo-os-update-${PICCOLO_VERSION}.raw.gz"

    log "Signing the update artifact with GPG key: ${GPG_SIGNING_KEY_ID}"
    gpg --detach-sign --armor -u "$GPG_SIGNING_KEY_ID" "${output_dir}/piccolo-os-update-${PICCOLO_VERSION}.raw.gz"
    log "Update image signed."

    # ---
    # Step 5: Package the Live ISO
    # ---
    log "### Step 5: Packaging the bootable Piccolo Live ISO..."
    local generated_iso_path
    generated_iso_path=$(find "$iso_output_dir" -name "*.iso")
    mv "$generated_iso_path" "${output_dir}/piccolo-os-live-${PICCOLO_VERSION}.iso"
    log "Live ISO created successfully."

    # ---
    # Step 6: Final Output
    # ---
    log "✅ Build complete!"
    log "Your final, signed artifacts are located in: ${output_dir}"
    ls -l "${output_dir}"
}

main "$@"
