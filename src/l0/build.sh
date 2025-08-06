#!/bin/bash
set -euo pipefail # Exit on error, unset var, or pipe failure

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
PICCOLO_VERSION="1.0.0"

cd "${SCRIPT_DIR}/../l1/piccolod"
./build.sh ${PICCOLO_VERSION}

cd "${SCRIPT_DIR}"
PICCOLOD_OUTPUT=${SCRIPT_DIR}/../l1/piccolod/build/piccolod
./build_piccolo.sh --version ${PICCOLO_VERSION} --binary-path $PICCOLOD_OUTPUT  > build.out 2>&1

./test_piccolo_os_image.sh --build-dir ./build/output/${PICCOLO_VERSION} --version ${PICCOLO_VERSION} > test.out 2>&1