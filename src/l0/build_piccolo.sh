#!/usr/bin/env bash
set -euo pipefail

# ------------------------------------------------------------------------------
# Piccolo OS – MicroOS-based ISO builder (UEFI + Secure Boot + Live/Self-Install)
# Uses KIWI NG. Runs inside a container if docker/podman is available.
#
# USAGE:
#   ./build_piccolo.sh /abs/path/to/piccolod [VERSION] [ARCH]
#
# DEFAULTS:
#   VERSION = 0.1.0
#   ARCH    = x86_64   (use aarch64 for Raspberry Pi UEFI boot flows)
#
# OUTPUT:
#   ./dist/piccolo-os-<ARCH>-<VERSION>.iso
#
# NOTES:
# - This script uses a persistent builder container to avoid re-installing
#   dependencies on every run, making builds much faster.
# - The builder image is created automatically on the first run.
# ------------------------------------------------------------------------------

# -------- parameters ----------
# Defaults
VERSION="0.1.0"
ARCH="x86_64"
PICCOLOD_BIN=""

# Parse command-line arguments
while [[ $# -gt 0 ]]; do
  key="$1"
  case $key in
    --binary-path)
      PICCOLOD_BIN="$2"
      shift # past argument
      shift # past value
      ;;
    --version)
      VERSION="$2"
      shift # past argument
      shift # past value
      ;;
    --arch)
      ARCH="$2"
      shift # past argument
      shift # past value
      ;;
    -h|--help)
      echo "Usage: ./build_piccolo.sh --binary-path <path> [--version <ver>] [--arch <arch>]"
      exit 0
      ;;
    *)
      echo "Unknown option: $1"
      exit 1
      ;;
  esac
done

if [[ -z "${PICCOLOD_BIN}" ]] || [[ ! -f "${PICCOLOD_BIN}" ]]; then
  echo "ERROR: --binary-path is required and must be a valid file."
  echo "Example: ./build_piccolo.sh --binary-path /path/to/piccolod"
  exit 1
fi

# -------- env & paths ----------
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WORK_DIR="${ROOT_DIR}/.work"
KIWI_DIR="${ROOT_DIR}/kiwi"
OVERLAY_DIR="${ROOT_DIR}/kiwi/root"
DIST_DIR="${ROOT_DIR}/dist"
IMAGE_NAME="piccolo-os"
IMAGE_LABEL="${IMAGE_NAME}-${ARCH}-${VERSION}"

mkdir -p "${WORK_DIR}" "${DIST_DIR}" "${OVERLAY_DIR}"

# -------- detect container runtime ----------
RUNTIME=""
if command -v docker >/dev/null 2>&1; then
  RUNTIME="docker"
elif command -v podman >/dev/null 2>&1; then
  RUNTIME="podman"
fi

# -------- check kiwi locally if no container ----------
function have_kiwi_local() {
  command -v kiwi-ng >/dev/null 2>&1
}

# -------- config.xml is checked into git, no need to generate ----------
CONFIG_XML="${KIWI_DIR}/config.xml"

# -------- ensure binary directory exists ----------
mkdir -p "${OVERLAY_DIR}/usr/local/piccolo/v1/bin"

# -------- copy piccolod into overlay ----------
install -m 0755 "${PICCOLOD_BIN}" "${OVERLAY_DIR}/usr/local/piccolo/v1/bin/piccolod"
if [[ -L "${OVERLAY_DIR}/usr/local/piccolo/current" ]]; then
  rm -f "${OVERLAY_DIR}/usr/local/piccolo/current"
fi
ln -sfn v1 "${OVERLAY_DIR}/usr/local/piccolo/current"

# -------- bump version in config.xml to match CLI arg ----------
if command -v python3 >/dev/null 2>&1; then
  python3 - <<PY >/dev/null 2>&1 || true
from pathlib import Path
p=Path("${CONFIG_XML}")
s=p.read_text()
s=s.replace("<version>0.1.0</version>", "<version>${VERSION}</version>")
p.write_text(s)
PY
fi

# --- containerized build (preferred for portability) ---
if [[ -n "${RUNTIME}" ]]; then
  BUILDER_IMG_TAG="piccolo-os-builder:${ARCH}"
  BUILDER_DOCKERFILE="${ROOT_DIR}/build.Dockerfile"

  echo "==> Using container runtime '${RUNTIME}'"
  if ${RUNTIME} image inspect "${BUILDER_IMG_TAG}" >/dev/null 2>&1; then
    echo "--> Found existing builder image: ${BUILDER_IMG_TAG}"
  else
    echo "--> Builder image not found. Building it now (this will take a few minutes)..."
    ${RUNTIME} build \
      -t "${BUILDER_IMG_TAG}" \
      -f "${BUILDER_DOCKERFILE}" \
      --build-arg "ARCH=${ARCH}" \
      "${ROOT_DIR}"
    echo "--> Builder image created successfully."
  fi

  echo "==> Cleaning previous build artifacts"
  # Clean up any previous build directories that might cause conflicts
  if [[ -d "${DIST_DIR}/build" ]]; then
    echo "--> Removing existing build directory"
    sudo rm -rf "${DIST_DIR}/build" || {
      echo "Failed to clean build directory. You may need to run: sudo rm -rf ${DIST_DIR}/build"
      exit 1
    }
  fi

  echo "==> Running KIWI build using pre-built image with persistent cache"
  # Create named volumes for caching to persist between builds
  ${RUNTIME} volume create piccolo-zypper-cache >/dev/null 2>&1 || true
  ${RUNTIME} volume create piccolo-kiwi-bundle-cache >/dev/null 2>&1 || true
  
  # Ensure loop devices are available on host
  sudo modprobe loop || true
  
  ${RUNTIME} run --rm \
    --user root \
    -v "${KIWI_DIR}:/build/kiwi" \
    -v "${DIST_DIR}:/build/result" \
    -v piccolo-zypper-cache:/var/cache/zypp \
    -v piccolo-kiwi-bundle-cache:/var/cache/kiwi \
    --env KIWI_DEBUG=1 \
    --privileged \
    -v /dev:/dev \
    "${BUILDER_IMG_TAG}" \
    kiwi-ng --color-output --debug --logfile /build/result/kiwi.log --target-arch "${ARCH}" \
      system build \
      --description /build/kiwi \
      --target-dir /build/result

elif have_kiwi_local; then
  echo "==> Cleaning previous build artifacts"
  # Clean up any previous build directories that might cause conflicts
  if [[ -d "${DIST_DIR}/build" ]]; then
    echo "--> Removing existing build directory"
    sudo rm -rf "${DIST_DIR}/build" || {
      echo "Failed to clean build directory. You may need to run: sudo rm -rf ${DIST_DIR}/build"
      exit 1
    }
  fi

  echo "==> Using local kiwi-ng"
  kiwi-ng --color-output --debug --logfile "${DIST_DIR}/kiwi.log" --target-arch "${ARCH}" \
    system build \
    --description "${KIWI_DIR}" \
    --target-dir "${DIST_DIR}"
else
  echo "ERROR: Neither podman/docker nor kiwi-ng found."
  exit 1
fi

# -------- collect artefacts ----------
ISO_SRC="$(ls -t "${DIST_DIR}"/*.iso 2>/dev/null | head -n1 || true)"
if [[ -z "${ISO_SRC}" ]]; then
  echo "ERROR: No ISO produced. Check ${DIST_DIR}/kiwi.log for details."
  exit 1
fi

FINAL_ISO="${DIST_DIR}/${IMAGE_LABEL}.iso"
mv -f "${ISO_SRC}" "${FINAL_ISO}"

echo
echo "✔ Build complete"
echo "ISO: ${FINAL_ISO}"
echo "Log: ${DIST_DIR}/kiwi.log"
echo
echo "Next steps:"
echo "  - Test in UEFI/QEMU: qemu-system-x86_64 -enable-kvm -m 2048 -cpu host -machine q35,accel=kvm -bios /usr/share/OVMF/OVMF_CODE.fd -cdrom ${FINAL_ISO}"
echo "  - Install to disk, boot with Secure Boot enabled (shim+signed kernel from repo)."
echo