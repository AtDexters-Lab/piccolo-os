#!/bin/bash
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
VERSION="1.0.0"
BINARY_PATH=""
BUILD_DIR="${SCRIPT_DIR}/build"
WORK_DIR="${BUILD_DIR}/work-${VERSION}"
FAST_BUILD=false

usage() {
    echo "Usage: $0 [OPTIONS]"
    echo "Options:"
    echo "  --version VERSION        Set the build version (default: ${VERSION})"
    echo "  --binary-path PATH       Path to piccolod binary to include"
    echo "  --fast                   Use cosa build-fast for quicker development cycles"
    echo "  --help                   Show this help message"
    exit 1
}

while [[ $# -gt 0 ]]; do
    case $1 in
        --version)
            VERSION="$2"
            shift 2
            ;;
        --binary-path)
            BINARY_PATH="$2"
            shift 2
            ;;
        --fast)
            FAST_BUILD=true
            shift
            ;;
        --help)
            usage
            ;;
        *)
            echo "Unknown option: $1"
            usage
            ;;
    esac
done

if [[ -z "${BINARY_PATH}" ]]; then
    echo "Error: --binary-path is required"
    usage
fi

if [[ ! -f "${BINARY_PATH}" ]]; then
    echo "Error: Binary not found at ${BINARY_PATH}"
    exit 1
fi

echo "Building Piccolo OS ${VERSION} based on Fedora CoreOS..."
echo "Using piccolod binary: ${BINARY_PATH}"

mkdir -p "${BUILD_DIR}"
cd "${BUILD_DIR}"

echo "Setting up CoreOS Assembler working directory..."
if [[ ! -d "${WORK_DIR}" ]]; then
    mkdir -p "${WORK_DIR}"
fi

cd "${WORK_DIR}"

# Ensure podman is available
if ! command -v podman >/dev/null 2>&1; then
    echo "Error: podman is required for CoreOS Assembler"
    echo "Please install podman to continue"
    exit 1
fi

# CoreOS Assembler container configuration
COREOS_ASSEMBLER_CONTAINER_LATEST="quay.io/coreos-assembler/coreos-assembler:latest"
COSA_CONTAINER="${COREOS_ASSEMBLER_CONTAINER:-$COREOS_ASSEMBLER_CONTAINER_LATEST}"

# Pull container if not present
echo "Ensuring CoreOS Assembler container is available..."
if ! podman image exists "${COSA_CONTAINER}"; then
    echo "Pulling ${COSA_CONTAINER}..."
    podman pull "${COSA_CONTAINER}"
fi

# Check container age and warn if outdated
if podman image exists "${COSA_CONTAINER}"; then
    cosa_build_date_str=$(podman inspect -f "{{.Created}}" "${COSA_CONTAINER}" | awk '{print $1}')
    cosa_build_date=$(date -d "${cosa_build_date_str}" +%s)
    if [[ $(date +%s) -ge $((cosa_build_date + 60*60*24*7)) ]]; then
        echo -e "\e[0;33m----" >&2
        echo "WARNING: The COSA container image is more than a week old and likely outdated." >&2
        echo "Consider pulling the latest version with:" >&2
        echo "podman pull ${COSA_CONTAINER}" >&2
        echo -e "----\e[0m" >&2
    fi
fi

# Script-friendly cosa function (non-interactive)
cosa() {
    local cmd="$1"
    shift
    
    echo "Running cosa ${cmd} with args: $*"
    
    container_name_suffix="${cmd}-$$"
    
    podman run --rm \
        --security-opt=label=disable \
        --privileged \
        --userns=keep-id:uid=1000,gid=1000 \
        -v="${PWD}:/srv/" \
        --device=/dev/kvm \
        --device=/dev/fuse \
        --tmpfs=/tmp \
        -v=/var/tmp:/var/tmp \
        --name="cosa-${container_name_suffix}" \
        ${COREOS_ASSEMBLER_CONFIG_GIT:+-v="$COREOS_ASSEMBLER_CONFIG_GIT:/srv/src/config/:ro"} \
        ${COREOS_ASSEMBLER_GIT:+-v="$COREOS_ASSEMBLER_GIT/src/:/usr/lib/coreos-assembler/:ro"} \
        ${COREOS_ASSEMBLER_ADD_CERTS:+-v="/etc/pki/ca-trust:/etc/pki/ca-trust:ro"} \
        ${COREOS_ASSEMBLER_CONTAINER_RUNTIME_ARGS:-} \
        "${COSA_CONTAINER}" \
        "${cmd}" "$@"
    
    local rc=$?
    if [[ $rc -ne 0 ]]; then
        echo "Error: cosa ${cmd} failed with exit code ${rc}" >&2
    fi
    return $rc
}

echo "Initializing Fedora CoreOS build..."
if [[ ! -d "src" ]]; then
    echo "Running: cosa init https://github.com/coreos/fedora-coreos-config"
    if ! cosa init https://github.com/coreos/fedora-coreos-config; then
        echo "Error: cosa init failed"
        exit 1
    fi
    echo "cosa init completed successfully"
else
    echo "Source directory already exists, skipping init"
fi

echo "Fetching metadata and packages..."
cosa fetch

if [[ "${FAST_BUILD}" == "true" ]]; then
    # Check if we have a previous build to build-fast from
    if [[ -d "builds" ]] && [[ -L "builds/latest" ]] && [[ -d "builds/latest" ]]; then
        echo "Building Fedora CoreOS (fast mode)..."
        cosa build-fast
    else
        echo "No previous build found - using full build instead of build-fast"
        cosa build
    fi
else
    echo "Building Fedora CoreOS..."
    cosa build
fi

echo "Building metal images (required for ISO)..."
cosa buildextend-metal

echo "Building metal4k images (required for ISO)..."
cosa buildextend-metal4k

echo "Building Live ISO..."
cosa buildextend-live

echo "Build completed successfully!"
echo "Build artifacts available in: ${WORK_DIR}/builds/"
echo "Live ISO should be available in the latest build directory"

# TODO: Integration with piccolod binary will be added in next phase
echo "Note: Piccolod integration not yet implemented - this builds vanilla Fedora CoreOS"
echo "Binary at ${BINARY_PATH} will be integrated in future iterations"