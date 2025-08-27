#!/bin/bash
set -euo pipefail # Exit on error, unset var, or pipe failure

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
PICCOLO_VERSION="1.0.0"
VARIANT="${1:-dev}"  # Default to dev variant for safety

# Validate variant parameter
if [[ "$VARIANT" != "prod" && "$VARIANT" != "dev" ]]; then
  echo "Usage: $0 [prod|dev]"
  echo "  prod - Build hardened production ISO (zero access)"
  echo "  dev  - Build development ISO (with cloud-init) [default]"
  exit 1
fi

echo "Building Piccolo OS ${PICCOLO_VERSION} - ${VARIANT} variant"

# Build piccolod binary
echo "==> Building piccolod binary..."
cd "${SCRIPT_DIR}/../l1/piccolod"
./build.sh ${PICCOLO_VERSION}

# Build OS image with variant
echo "==> Building ${VARIANT} OS image..."
cd "${SCRIPT_DIR}"
PICCOLOD_OUTPUT=${SCRIPT_DIR}/../l1/piccolod/build/piccolod
./build_piccolo.sh --version ${PICCOLO_VERSION} --variant ${VARIANT} --binary-path $PICCOLOD_OUTPUT > build.out 2>&1

# Test the built image (only dev variant for now, since it has SSH access)
if [[ "$VARIANT" == "dev" ]]; then
  echo "==> Testing ${VARIANT} OS image..."
  ./test_piccolo_os_image.sh --build-dir ./releases/${PICCOLO_VERSION} --version ${PICCOLO_VERSION} > test.out 2>&1
else
  echo "==> Production ISO built successfully"
  echo "    Testing production ISO requires manual verification (no SSH access)"
  echo "    Use QEMU to boot and verify API endpoints manually"
fi

echo "Build complete: ${VARIANT} variant"