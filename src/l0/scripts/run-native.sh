#!/usr/bin/env bash
set -euo pipefail

# install packages only if missing
pkgs=(container-selinux passt-selinux) # these are needed for kiwi to build images containing package:patterns-containers-container_runtime
missing=()
for p in "${pkgs[@]}"; do
  if ! rpm -q "$p" >/dev/null 2>&1; then
    missing+=("$p")
  fi
done

if [ "${#missing[@]}" -ne 0 ]; then
  echo "Installing missing packages: ${missing[*]}"
  sudo zypper -n in "${missing[@]}"
else
  echo "All required packages already installed: ${pkgs[*]}"
fi

VERSION=0.1.0-dev
# Use first argument as the profile, default to "Standard"
PROFILE="${1:-Standard}"
KIWIBASE=${2:-piccolo-microos}

RELEASE_DIR=releases/$KIWIBASE/${PROFILE}_${VERSION}
echo "Building release in $RELEASE_DIR"

sudo rm -rf $RELEASE_DIR
mkdir -p $RELEASE_DIR

kiwi-ng --profile $PROFILE \
  --logfile $RELEASE_DIR/kiwi.log \
  system build \
  --description "$(cd "$(dirname "${BASH_SOURCE[0]}")/../kiwi" >/dev/null 2>&1 && pwd)/${KIWIBASE}" \
  --target-dir $RELEASE_DIR
