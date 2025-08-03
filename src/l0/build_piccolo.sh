#!/bin/bash
#
# Piccolo OS - Production Build Script v5.1 (Definitive)
#
# This version uses the 'misc-files' pattern for robust file injection.
# This is a simpler and more direct method that bypasses the complexities
# of Portage overlays and dependency management, which have proven fragile.
# It places the binary in a specific directory structure that a standard
# build script then copies into the final image.
#
# v5.1 adds the systemd service file for piccolod and enables it by default.
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
    # Step 2: Place piccolod Binary and Systemd Service for 'misc-files' Ebuild
    # ---
    log "### Step 2: Placing binary and systemd service for 'misc-files' injection..."
    local misc_files_dir="${scripts_repo_dir}/src/third_party/coreos-overlay/coreos-base/misc-files/files"
    
    # 2a: Place the piccolod binary in /usr/bin
    local install_path_in_misc_files="${misc_files_dir}/usr/bin"
    mkdir -p "${install_path_in_misc_files}"
    cp "$(realpath "$PICCOLOD_BINARY_PATH")" "${install_path_in_misc_files}/piccolod"
    chmod +x "${install_path_in_misc_files}/piccolod"
    log "piccolod binary placed in ${install_path_in_misc_files}"

    # 2b: Place the systemd service file in /etc/systemd/system
    local systemd_path="${misc_files_dir}/etc/systemd/system"
    mkdir -p "${systemd_path}"
    cat > "${systemd_path}/piccolod.service" << EOF
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
    log "piccolod.service file created."

    # 2c: Enable the service by creating a symlink
    local enable_path="${systemd_path}/multi-user.target.wants"
    mkdir -p "${enable_path}"
    ln -s ../piccolod.service "${enable_path}/piccolod.service"
    log "piccolod.service enabled."


    # ---
    # Step 3: Build All Artifacts Inside the SDK Container
    # ---
    log "### Step 3: Starting the SDK to build all artifacts..."
    pushd "$scripts_repo_dir" > /dev/null
    
    # The build process will automatically pick up all files we just placed.
    ./run_sdk_container -- /bin/bash -s << 'EOF'
set -euxo pipefail

echo "Running ./build_packages..."
# The 'misc-files' package is part of the default build, so this will
# automatically package our binary and service file.
./build_packages --board='amd64-usr'

echo "Running ./build_image to create prod image and update payload..."
./build_image --board='amd64-usr' --group=stable prod

echo "Creating bootable ISO from the production image..."
LATEST_BUILD_DIR="./__build__/images/amd64-usr/latest"
./image_to_vm.sh --from="${LATEST_BUILD_DIR}" --format=iso --board='amd64-usr'

EOF
    popd > /dev/null

    log "### Finished building all artifacts inside the SDK!"
    
    # ---
    # Step 4 & 5: Package and Sign Final Artifacts
    # ---
    log "### Step 4 & 5: Packaging and signing final artifacts..."
    local artifact_src_dir="${scripts_repo_dir}/__build__/images/amd64-usr/latest"
    
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
