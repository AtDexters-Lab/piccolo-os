#!/usr/bin/env bash
set -euo pipefail

# Sync a local mirror of openSUSE Tumbleweed (oss + non-oss) for offline KIWI builds.
#
# Usage:
#   ./scripts/sync_tumbleweed_mirror.sh [--dest /var/mirrors/tw]
#
# The script requires rsync and approximately 30–40 GB of free disk space.

DEST_BASE="/var/mirrors/tw"
RSYNC_BIN="${RSYNC_BIN:-rsync}"
RSYNC_BASE="${RSYNC_BASE:-rsync://download.opensuse.org/tumbleweed/repo}"

usage() {
  cat <<USAGE
Usage: $0 [--dest /var/mirrors/tw] [--rsync-base rsync://mirror/tumbleweed/repo]

Options:
  --dest <path>       Destination directory for the mirror (default: /var/mirrors/tw)
  --rsync-base <url>  Base rsync URL (default: rsync://download.opensuse.org/tumbleweed/repo)
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --dest)
      DEST_BASE="$2"
      shift 2
      ;;
    --rsync-base)
      RSYNC_BASE="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      usage
      exit 1
      ;;
  esac
done

if ! command -v "${RSYNC_BIN}" >/dev/null 2>&1; then
  echo "Error: rsync is required." >&2
  exit 1
fi

mkdir -p "${DEST_BASE}/oss" "${DEST_BASE}/non-oss"

sync_repo() {
  local repo="$1"
  local target="${DEST_BASE}/${repo}"
  local url="${RSYNC_BASE%/}/${repo}/"
  echo "[mirror] Syncing ${repo} → ${target}"
  "${RSYNC_BIN}" -avh --info=progress2 --delete --partial "${url}" "${target}"
}

sync_repo "oss"
# sync_repo "non-oss"

echo "[mirror] Done. Update your build environment to use file://${DEST_BASE}/{oss,non-oss}"