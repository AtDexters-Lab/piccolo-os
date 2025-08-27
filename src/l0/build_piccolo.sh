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
VARIANT="dev"  # Default to development variant (safer for testing)
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
    --variant)
      VARIANT="$2"
      if [[ "$VARIANT" != "prod" && "$VARIANT" != "dev" ]]; then
        echo "ERROR: --variant must be 'prod' or 'dev'"
        exit 1
      fi
      shift # past argument
      shift # past value
      ;;
    -h|--help)
      echo "Usage: ./build_piccolo.sh --binary-path <path> [--variant prod|dev] [--version <ver>] [--arch <arch>]"
      echo ""
      echo "  --binary-path <path>  Path to piccolod binary (required)"
      echo "  --variant <variant>   Build variant: 'prod' (hardened) or 'dev' (with cloud-init) [default: dev]"
      echo "  --version <version>   Version tag [default: 0.1.0]" 
      echo "  --arch <arch>         Architecture [default: x86_64]"
      echo ""
      echo "Examples:"
      echo "  ./build_piccolo.sh --binary-path ../l1/piccolod/build/piccolod --variant prod --version 1.0.0"
      echo "  ./build_piccolo.sh --binary-path ../l1/piccolod/build/piccolod --variant dev"
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
DIST_DIR="${ROOT_DIR}/dist"
RELEASES_DIR="${ROOT_DIR}/releases"
VERSION_RELEASES_DIR="${RELEASES_DIR}/${VERSION}"

# Variant-specific configuration
if [[ "$VARIANT" == "prod" ]]; then
  VARIANT_KIWI_DIR="${KIWI_DIR}/prod"
  IMAGE_NAME="piccolo-os-prod"
  echo "🔒 PRODUCTION BUILD: Zero-access hardened configuration"
else
  # Development builds use additive approach: prod base + dev additions
  VARIANT_KIWI_DIR="${WORK_DIR}/kiwi-dev-generated"
  IMAGE_NAME="piccolo-os-dev"
  echo "🔧 DEVELOPMENT BUILD: Production base + Cloud-init additions"
fi

OVERLAY_DIR="${VARIANT_KIWI_DIR}/root"

IMAGE_LABEL="${IMAGE_NAME}.${ARCH}-${VERSION}"

mkdir -p "${WORK_DIR}" "${DIST_DIR}" "${OVERLAY_DIR}" "${RELEASES_DIR}" "${VERSION_RELEASES_DIR}"

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

# -------- validate and prepare variant configuration ----------
if [[ "$VARIANT" == "prod" ]]; then
  # Production: Use existing prod directory
  if [[ ! -d "${VARIANT_KIWI_DIR}" ]]; then
    echo "ERROR: Production variant directory not found: ${VARIANT_KIWI_DIR}"
    exit 1
  fi
  if [[ ! -f "${VARIANT_KIWI_DIR}/config.xml" ]]; then
    echo "ERROR: Production config file not found: ${VARIANT_KIWI_DIR}/config.xml"
    exit 1
  fi
else
  # Development: Generate from prod base + dev additions
  echo "==> Generating development configuration from production base..."
  
  # Validate dev additions exist
  if [[ ! -d "${KIWI_DIR}/dev" ]]; then
    echo "ERROR: Development additions directory not found: ${KIWI_DIR}/dev"
    exit 1
  fi
  
  # Clean and create dev build directory
  rm -rf "${VARIANT_KIWI_DIR}"
  mkdir -p "${VARIANT_KIWI_DIR}"
  
  # Copy production as base
  echo "--> Copying production base configuration..."
  cp -r "${KIWI_DIR}/prod"/* "${VARIANT_KIWI_DIR}/"
  
  # Change image name from prod to dev
  echo "--> Updating image name for development variant..."
  sed -i 's/name="piccolo-os-prod"/name="piccolo-os-dev"/' "${VARIANT_KIWI_DIR}/config.xml"
  sed -i 's/MicroOS-based hardened production appliance/MicroOS-based development appliance/' "${VARIANT_KIWI_DIR}/config.xml"
  sed -i 's/ZERO ACCESS/SSH + Cloud-init enabled/' "${VARIANT_KIWI_DIR}/config.xml"
  
  # Apply dev package additions
  if [[ -f "${KIWI_DIR}/dev/packages.xml" ]]; then
    echo "--> Adding development packages..."
    sed -i '/<!-- DEV_PACKAGES_INSERT_POINT -->/r '"${KIWI_DIR}/dev/packages.xml" "${VARIANT_KIWI_DIR}/config.xml"
    sed -i '/<!-- DEV_PACKAGES_INSERT_POINT -->/d' "${VARIANT_KIWI_DIR}/config.xml"
  fi
  
  # Apply dev service additions
  if [[ -f "${KIWI_DIR}/dev/services.sh" ]]; then
    echo "--> Adding development services..."
    sed -i '/# DEV_SERVICES_INSERT_POINT/r '"${KIWI_DIR}/dev/services.sh" "${VARIANT_KIWI_DIR}/config.sh"
    sed -i '/# DEV_SERVICES_INSERT_POINT/d' "${VARIANT_KIWI_DIR}/config.sh"
  fi
  
  # Overlay dev-specific files
  if [[ -d "${KIWI_DIR}/dev/root" ]]; then
    echo "--> Adding development overlay files..."
    cp -r "${KIWI_DIR}/dev/root"/* "${VARIANT_KIWI_DIR}/root/" 2>/dev/null || true
  fi
  
  echo "--> Development configuration generated successfully"
fi

# -------- ensure binary directory exists ----------
mkdir -p "${OVERLAY_DIR}/usr/local/piccolo/v1/bin"

# -------- copy piccolod into overlay ----------
install -m 0755 "${PICCOLOD_BIN}" "${OVERLAY_DIR}/usr/local/piccolo/v1/bin/piccolod"
if [[ -L "${OVERLAY_DIR}/usr/local/piccolo/current" ]]; then
  rm -f "${OVERLAY_DIR}/usr/local/piccolo/current"
fi
ln -sfn v1 "${OVERLAY_DIR}/usr/local/piccolo/current"

# -------- validate variant-specific configuration script ----------
VARIANT_CONFIG_SCRIPT="${VARIANT_KIWI_DIR}/config.sh"
if [[ -f "${VARIANT_CONFIG_SCRIPT}" ]]; then
  echo "==> Validating ${VARIANT} configuration script"
  echo "--> Found variant config: ${VARIANT_CONFIG_SCRIPT}"
else
  echo "ERROR: Variant configuration script not found: ${VARIANT_CONFIG_SCRIPT}"
  exit 1
fi

# Note: Version is managed in config.xml directly - no dynamic replacement needed

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
  
  # Clean entire dist directory for fresh build
  echo "--> Removing entire dist directory for clean build"
  sudo rm -rf "${DIST_DIR}" || {
    echo "Failed to clean dist directory. You may need to run: sudo rm -rf ${DIST_DIR}"
    exit 1
  }
  mkdir -p "${DIST_DIR}"
  
  # Clean existing artifacts for this variant and version in releases directory
  if [[ -d "${VERSION_RELEASES_DIR}" ]]; then
    echo "--> Removing existing ${VARIANT} artifacts for version ${VERSION}"
    rm -f "${VERSION_RELEASES_DIR}/${IMAGE_NAME}.${ARCH}-${VERSION}".*
    # Remove old ISO artifacts that shouldn't exist anymore
    rm -f "${VERSION_RELEASES_DIR}"/piccolo-os*.iso "${VERSION_RELEASES_DIR}"/piccolo-os.x86_64-*
  fi

  echo "==> Running KIWI build using pre-built image with persistent cache"
  # Create named volumes for caching to persist between builds
  ${RUNTIME} volume create piccolo-zypper-cache >/dev/null 2>&1 || true
  ${RUNTIME} volume create piccolo-kiwi-bundle-cache >/dev/null 2>&1 || true
  
  # Ensure loop devices are available on host
  sudo modprobe loop || true
  
  ${RUNTIME} run --rm \
    --user root \
    -v "${VARIANT_KIWI_DIR}:/build/kiwi-config" \
    -v "${DIST_DIR}:/build/result" \
    -v piccolo-zypper-cache:/var/cache/zypp \
    -v piccolo-kiwi-bundle-cache:/var/cache/kiwi \
    --env KIWI_DEBUG=1 \
    --privileged \
    -v /dev:/dev \
    "${BUILDER_IMG_TAG}" \
    kiwi-ng --color-output --debug --logfile /build/result/kiwi.log --target-arch "${ARCH}" \
      system build \
      --description /build/kiwi-config \
      --target-dir /build/result

elif have_kiwi_local; then
  echo "==> Cleaning previous build artifacts"
  
  # Clean entire dist directory for fresh build
  echo "--> Removing entire dist directory for clean build"
  sudo rm -rf "${DIST_DIR}" || {
    echo "Failed to clean dist directory. You may need to run: sudo rm -rf ${DIST_DIR}"
    exit 1
  }
  mkdir -p "${DIST_DIR}"
  
  # Clean existing artifacts for this variant and version in releases directory
  if [[ -d "${VERSION_RELEASES_DIR}" ]]; then
    echo "--> Removing existing ${VARIANT} artifacts for version ${VERSION}"
    rm -f "${VERSION_RELEASES_DIR}/${IMAGE_NAME}.${ARCH}-${VERSION}".*
    # Remove old ISO artifacts that shouldn't exist anymore
    rm -f "${VERSION_RELEASES_DIR}"/piccolo-os*.iso "${VERSION_RELEASES_DIR}"/piccolo-os.x86_64-*
  fi

  echo "==> Using local kiwi-ng"
  kiwi-ng --color-output --debug --logfile "${DIST_DIR}/kiwi.log" --target-arch "${ARCH}" \
    system build \
    --description "${VARIANT_KIWI_DIR}" \
    --target-dir "${DIST_DIR}"
else
  echo "ERROR: Neither podman/docker nor kiwi-ng found."
  exit 1
fi

# -------- collect artefacts ----------
# Look for disk image first (.raw), then fallback to ISO for backward compatibility
DISK_SRC="$(ls -t "${DIST_DIR}"/*.raw 2>/dev/null | head -n1 || true)"
ISO_SRC="$(ls -t "${DIST_DIR}"/*.iso 2>/dev/null | head -n1 || true)"

if [[ -n "${DISK_SRC}" ]]; then
  IMAGE_SRC="${DISK_SRC}"
  IMAGE_EXT="raw"
  IMAGE_TYPE="disk image"
elif [[ -n "${ISO_SRC}" ]]; then
  IMAGE_SRC="${ISO_SRC}"
  IMAGE_EXT="iso"
  IMAGE_TYPE="ISO"
else
  echo "ERROR: No disk image or ISO produced. Check ${DIST_DIR}/kiwi.log for details."
  exit 1
fi

RELEASE_IMAGE="${VERSION_RELEASES_DIR}/${IMAGE_LABEL}.${IMAGE_EXT}"
RELEASE_LOG="${VERSION_RELEASES_DIR}/${IMAGE_LABEL}.log"

# Copy artifacts to releases directory for preservation
echo "==> Preserving build artifacts in releases directory"
cp -f "${IMAGE_SRC}" "${RELEASE_IMAGE}"
if [[ -f "${DIST_DIR}/kiwi.log" ]]; then
  cp -f "${DIST_DIR}/kiwi.log" "${RELEASE_LOG}"
fi

# Preserve additional artifacts critical for updates and system management
RELEASE_PACKAGES="${VERSION_RELEASES_DIR}/${IMAGE_LABEL}.packages"
RELEASE_CHANGES="${VERSION_RELEASES_DIR}/${IMAGE_LABEL}.changes"
RELEASE_VERIFIED="${VERSION_RELEASES_DIR}/${IMAGE_LABEL}.verified"
RELEASE_METADATA="${VERSION_RELEASES_DIR}/${IMAGE_LABEL}.json"

if [[ -f "${DIST_DIR}/${IMAGE_LABEL}.packages" ]]; then
  cp -f "${DIST_DIR}/${IMAGE_LABEL}.packages" "${RELEASE_PACKAGES}"
fi
if [[ -f "${DIST_DIR}/${IMAGE_LABEL}.changes" ]]; then
  cp -f "${DIST_DIR}/${IMAGE_LABEL}.changes" "${RELEASE_CHANGES}"
fi
if [[ -f "${DIST_DIR}/${IMAGE_LABEL}.verified" ]]; then
  cp -f "${DIST_DIR}/${IMAGE_LABEL}.verified" "${RELEASE_VERIFIED}"
fi
if [[ -f "${DIST_DIR}/kiwi.result.json" ]]; then
  cp -f "${DIST_DIR}/kiwi.result.json" "${RELEASE_METADATA}"
fi

echo
echo "✔ Build complete for ${VARIANT} variant"
if [[ "$VARIANT" == "prod" ]]; then
  echo "🔒 PRODUCTION ${IMAGE_TYPE^^}: Zero-access hardened appliance"
  echo "   - USB bootable disk image with systemd-boot"
  echo "   - NO SSH access"
  echo "   - NO cloud-init"  
  echo "   - NO serial console"
  echo "   - API-only access via piccolod on port 80"
  echo "   - Can install to internal drives via OEM modules"
else
  echo "🔧 DEVELOPMENT ${IMAGE_TYPE^^}: Cloud-init enabled for testing"
  echo "   - USB bootable disk image with systemd-boot"
  echo "   - SSH access via cloud-init"
fi
echo "${IMAGE_TYPE^^}: ${IMAGE_SRC}"
echo "Release ${IMAGE_TYPE}: ${RELEASE_IMAGE}"
echo "Log: ${DIST_DIR}/kiwi.log"
echo "Release Log: ${RELEASE_LOG}"
echo
echo "📋 Additional preserved artifacts:"
echo "  Packages: ${RELEASE_PACKAGES}"
echo "  Changes: ${RELEASE_CHANGES}"
echo "  Verified: ${RELEASE_VERIFIED}"
echo "  Metadata: ${RELEASE_METADATA}"
# Show summary of all preserved releases
echo
echo "📦 All preserved releases:"
if [[ -d "${RELEASES_DIR}" ]] && (find "${RELEASES_DIR}" -name "*.raw" -type f | head -1 >/dev/null 2>&1); then
  for version_dir in "${RELEASES_DIR}"/*/; do
    if [[ -d "$version_dir" ]]; then
      version=$(basename "$version_dir")
      # Look for disk image
      disk_path="${version_dir}/${IMAGE_NAME}-${ARCH}-${version}.raw"
      if [[ -f "$disk_path" ]]; then
        size=$(du -h "$disk_path" | cut -f1)
        artifact_count=$(find "$version_dir" -type f | wc -l)
        echo "  - v${version} (${size}, ${artifact_count} artifacts) [DISK IMAGE]"
      fi
    fi
  done
else
  echo "  - v${VERSION} (current build)"
fi

echo
echo "Next steps:"
echo "  - Write to USB: sudo dd if=${IMAGE_SRC} of=/dev/sdX bs=4M status=progress && sync"
echo "  - Test in QEMU: qemu-system-x86_64 -enable-kvm -m 4096 -cpu host -machine q35,accel=kvm -bios /usr/share/OVMF/OVMF_CODE.fd -drive file=${IMAGE_SRC},format=raw"
echo "  - Boot from USB with UEFI + Secure Boot enabled (systemd-boot + shim)"
echo