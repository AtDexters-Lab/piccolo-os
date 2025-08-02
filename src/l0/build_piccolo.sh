#!/bin/bash
#
# Piccolo OS - Production Build Script v2.3
#
# This script aligns with the canonical integration method for Flatcar Linux.
# It uses a dedicated overlay and the PORTDIR_OVERLAYS variable to correctly
# inject the piccolod package as a runtime dependency (RDEPEND) of the
# core OS.
# v2.3 corrects the build artifact path within the SDK container.
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
    mkdir -p "$work_dir"
    mkdir -p "$output_dir"
    
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
    # Step 2: Create Custom Piccolo OS Overlay
    # ---
    log "### Step 2: Creating custom Piccolo overlay..."
    local overlay_dir="${scripts_repo_dir}/src/third_party/piccolo-overlay"
    local ebuild_dir="${overlay_dir}/app-piccolo/piccolod"
    local metadata_dir="${overlay_dir}/metadata"
    
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

    mkdir -p "${metadata_dir}"
    echo "masters = portage-stable" > "${metadata_dir}/layout.conf"
    mkdir -p "${overlay_dir}/profiles"
    echo "piccolo-overlay" > "${overlay_dir}/profiles/repo_name"
    echo "app-piccolo" > "${overlay_dir}/profiles/categories"
    log "Custom overlay created successfully at ${overlay_dir}"

    # ---
    # Step 3: Build All Artifacts Inside the SDK Container
    # ---
    log "### Step 3: Starting the SDK to build all artifacts..."
    pushd "$scripts_repo_dir" > /dev/null
    
    ./run_sdk_container -- /bin/bash -s -- "${PICCOLO_VERSION}" "src/third_party/piccolo-overlay" << 'EOF'
set -euxo pipefail

PICCOLO_VERSION="$1"
PICCOLO_OVERLAY_PATH="$2"

export PORTDIR_OVERLAYS="${PWD}/${PICCOLO_OVERLAY_PATH} ${PORTDIR_OVERLAYS:-}"
echo "Portage is now searching for packages in: ${PORTDIR_OVERLAYS}"

EBUILD_PATH="${PICCOLO_OVERLAY_PATH}/app-piccolo/piccolod/piccolod-${PICCOLO_VERSION}.ebuild"
ebuild "${EBUILD_PATH}" manifest
echo "Manifest generated for piccolod."

COREOS_EBUILD_PATH="src/third_party/coreos-overlay/coreos-base/coreos/coreos-0.0.1.ebuild"
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
# The 'build' directory is located relative to the SDK root, one level up.
LATEST_BUILD_DIR="../build/images/amd64-usr/latest"
./image_to_vm.sh --from="${LATEST_BUILD_DIR}" --format=iso --to="./iso_out"

# This copy is ESSENTIAL. It moves the update artifact to the shared 'iso_out' directory.
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

    # The auto-generated runner script is no longer needed, as our QA script
    # 'test_piccolo_os_image.sh' handles VM execution.
    log "Live ISO created successfully."

    # ---
    # Step 6: Final Output
    # ---
    log "✅ Build complete!"
    log "Your final, signed artifacts are located in: ${output_dir}"
    ls -l "${output_dir}"
}

main "$@"
