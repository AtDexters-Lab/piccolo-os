#!/bin/bash

set -euo pipefail

VERSION=${1:-"dev"}

echo "Building piccolod version: ${VERSION}"

go build -ldflags="-X main.version=${VERSION}" -o ./build/piccolod ./cmd/piccolod
