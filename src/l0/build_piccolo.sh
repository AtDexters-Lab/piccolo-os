#!/bin/bash
#
# Piccolo OS - Production Build Script v8.0 (Definitive)
#
# This version incorporates the final understanding of the SDK's mounting
# behavior. It creates the custom overlay and repository configuration inside
# the 'sdk_container' directory, which is automatically mounted into the
# container's source tree. This removes all complex, manual mount commands
# and relies on the SDK's native, robust functionality.
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
    local deps=("git" "docker" "gpg")
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
    
    if [ -z "${PICCOLO_UPDATE_SERVER:-}" ] || [ -z "${PICCOLO_UPDATE_GROUP:-}" ]; then
        log "Error: PICCOLO_UPDATE_SERVER or PICCOLO_UPDATE_GROUP not set in piccolo.env" >&2
        exit 1
    fi

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
    git fetch --prune --prune-tags --tags --force origin
    LATEST_STABLE_TAG=$(git tag -l | grep -E 'stable-[0-9.]+$' | sort -V | tail -n 1)
    if [ -z "$LATEST_STABLE_TAG" ]; then log "Error: Could not find any stable release tags." >&2; exit 1; fi
    log "Checking out latest stable release: $LATEST_STABLE_TAG"
    git checkout "$LATEST_STABLE_TAG"
    popd > /dev/null

    # ---
    # Step 2: Create Custom Ebuild, Overlay, and Repository Config
    # ---
    log "### Step 2: Creating custom ebuild and overlay..."
    # FIX: Place the overlay inside the 'sdk_container' directory.
    # This directory's contents are automatically made available inside the container.
    local overlay_dir="${scripts_repo_dir}/sdk_container/piccolo-overlay"
    local ebuild_category="app-piccolo"
    local ebuild_pkg_name="piccolod-bin"
    local ebuild_dir="${overlay_dir}/${ebuild_category}/${ebuild_pkg_name}"
    
    mkdir -p "${ebuild_dir}/files"
    cp "$(realpath "$PICCOLOD_BINARY_PATH")" "${ebuild_dir}/files/piccolod"
    
    cat > "${ebuild_dir}/files/piccolod.service" << EOF
[Unit]
Description=Piccolo Daemon
After=network-online.target
Wants=network-online.target
[Service]
ExecStart=/usr/bin/piccolod
Restart=always
RestartSec=5s
[Install]
WantedBy=multi-user.target
EOF

    cat > "${ebuild_dir}/files/update.conf" << EOF
GROUP=${PICCOLO_UPDATE_GROUP}
SERVER=${PICCOLO_UPDATE_SERVER}
EOF

    cat > "${ebuild_dir}/${ebuild_pkg_name}-${PICCOLO_VERSION}.ebuild" << EOF
EAPI=7
inherit systemd
DESCRIPTION="The core service for the Piccolo OS ecosystem (pre-compiled)"
HOMEPAGE="https://piccolospace.com"
SRC_URI=""
LICENSE="Piccolo-EULA"
SLOT="0"
KEYWORDS="~amd64"
QA_PREBUILT=*

src_install() {
    dobin "\${FILESDIR}/piccolod"
    systemd_dounit "\${FILESDIR}/piccolod.service"
    insinto /etc/flatcar
    doins "\${FILESDIR}/update.conf"
}
EOF

    mkdir -p "${overlay_dir}/metadata"
    echo "masters = portage-stable" > "${overlay_dir}/metadata/layout.conf"
    mkdir -p "${overlay_dir}/profiles"
    echo "piccolo-overlay" > "${overlay_dir}/profiles/repo_name"

    # FIX: Place the repository config inside the 'sdk_container' directory.
    log "Creating repository config..."
    local repo_config_dir="${scripts_repo_dir}/sdk_container/config/portage/repos"
    mkdir -p "$repo_config_dir"
    cat > "${repo_config_dir}/piccolo.conf" << EOF
[piccolo-overlay]
# The location is now relative to the container's source root.
location = /mnt/host/source/piccolo-overlay
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
    
    # FIX: No manual mounts needed. Rely on the default behavior.
    ./run_sdk_container -- /bin/bash -s -- "${PICCOLO_VERSION}" "${ebuild_category}" "${ebuild_pkg_name}" "${PICCOLO_UPDATE_GROUP}" << 'EOF'
set -euxo pipefail

PICCOLO_VERSION="$1"
EBUILD_CATEGORY="$2"
EBUILD_PKG_NAME="$3"
PICCOLO_UPDATE_GROUP="$4"

# The path inside the container now correctly points to the auto-mounted overlay.
EBUILD_PATH="/mnt/host/source/piccolo-overlay/${EBUILD_CATEGORY}/${EBUILD_PKG_NAME}/${EBUILD_PKG_NAME}-${PICCOLO_VERSION}.ebuild"
ebuild "${EBUILD_PATH}" manifest
echo "Manifest generated for ${EBUILD_PKG_NAME}."

COREOS_EBUILD_PATH=$(find . -path '*/coreos-base/coreos/coreos-0.0.1.ebuild' | head -n 1)
if [ -z "${COREOS_EBUILD_PATH}" ]; then
    echo "FATAL: Could not dynamically find the coreos-0.0.1.ebuild file." >&2
    exit 1
fi
echo "Found coreos ebuild at: ${COREOS_EBUILD_PATH}"

DEP_STRING="${EBUILD_CATEGORY}/${EBUILD_PKG_NAME}"
if ! grep -q "${DEP_STRING}" "${COREOS_EBUILD_PATH}"; then
    echo "Adding ${DEP_STRING} as an RDEPEND to the coreos package..."
    sed -i "/^\"$/i \\    ${DEP_STRING}" "${COREOS_EBUILD_PATH}"
else
    echo "${DEP_STRING} dependency already exists in coreos package."
fi

echo "Running pre-flight dependency check..."
emerge-amd64-usr -p --quiet coreos-base/coreos

echo "Running ./build_packages..."
./build_packages --board='amd64-usr'

echo "Running ./build_image to create prod image and update payload..."
./build_image --board='amd64-usr' --group="${PICCOLO_UPDATE_GROUP}" --image_compression_formats=gz prod

echo "Creating bootable ISO from the production image..."
LATEST_BUILD_DIR="./__build__/images/images/amd64-usr/latest"
./image_to_vm.sh --from="${LATEST_BUILD_DIR}" --format=iso --board='amd64-usr'

EOF
    popd > /dev/null

    log "### Finished building all artifacts inside the SDK!"
    
    # ---
    # Step 4 & 5: Package and Sign Final Artifacts
    # ---
    log "### Step 4 & 5: Packaging and signing final artifacts..."
    local artifact_src_dir="${scripts_repo_dir}/__build__/images/images/amd64-usr/latest"
    
    local src_raw_gz="${artifact_src_dir}/flatcar_production_update.raw.gz"
    local src_iso="${artifact_src_dir}/flatcar_production_iso_image.iso"
    
    if [ ! -f "$src_raw_gz" ] || [ ! -f "$src_iso" ]; then
        log "Error: A required build artifact (.raw.gz or .iso) was not found in ${artifact_src_dir}" >&2
        exit 1
    fi

    local final_raw_gz="${output_dir}/piccolo-os-update-${PICCOLO_VERSION}.raw.gz"
    local final_asc="${output_dir}/piccolo-os-update-${PICCOLO_VERSION}.raw.gz.asc"
    local final_iso="${output_dir}/piccolo-os-live-${PICCOLO_VERSION}.iso"

    log "Moving and renaming final artifacts to ${output_dir}"
    mv "$src_raw_gz" "$final_raw_gz"
    mv "$src_iso" "$final_iso"

    log "Signing the update artifact with GPG key: ${GPG_SIGNING_KEY_ID}"
    gpg --detach-sign --armor --output "${final_asc}" -u "$GPG_SIGNING_KEY_ID" "$final_raw_gz"
    log "Verifying signature..."
    gpg --verify "${final_asc}" "${final_raw_gz}"
    log "Update image signed and verified."

    # ---
    # Step 6: Final Output
    # ---
    log "✅ Build complete!"
    log "Your final, signed artifacts are located in: ${output_dir}"
    ls -l "${output_dir}"
}

main "$@"
