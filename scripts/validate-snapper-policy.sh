#!/usr/bin/env bash
set -euo pipefail

usage() {
    cat >&2 <<'EOF'
Usage:
  scripts/validate-snapper-policy.sh [MOUNTED_ROOT]

MOUNTED_ROOT defaults to /. Run this after mounting a produced image root; the
script validates MOUNTED_ROOT/etc/snapper/configs/root against the repo policy.
EOF
}

case "${1:-}" in
    -h|--help)
        usage
        exit 0
        ;;
esac

mounted_root=${1:-/}
mounted_root=${mounted_root%/}
if [ -z "$mounted_root" ]; then
    mounted_root=/
fi

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
repo_root=$(cd "$script_dir/.." && pwd)
policy_script="$repo_root/packages/piccolo-os-support/piccolo-snapper-policy.sh"

if [ "$mounted_root" = "/" ]; then
    snapper_config=/etc/snapper/configs/root
else
    snapper_config="$mounted_root/etc/snapper/configs/root"
fi

if [ ! -x "$policy_script" ]; then
    echo "validate-snapper-policy: missing executable policy helper: $policy_script" >&2
    exit 1
fi

"$policy_script" check "$snapper_config"
