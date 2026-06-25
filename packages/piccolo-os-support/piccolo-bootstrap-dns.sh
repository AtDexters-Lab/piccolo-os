#!/usr/bin/env bash
set -euo pipefail

target=${2:-/etc/resolv.conf}

write_resolv_conf() {
    local dir tmp
    dir=$(dirname "$target")
    mkdir -p "$dir"
    tmp=$(mktemp "$dir/.resolv.conf.XXXXXX")
    print_resolv_conf > "$tmp"
    chmod 0644 "$tmp"
    mv -f "$tmp" "$target"
}

print_resolv_conf() {
    cat <<'EOF'
# Managed by piccolo-os-support bootstrap DNS policy.
nameserver 1.1.1.1
nameserver 9.9.9.9
EOF
}

check_resolv_conf() {
    [ -f "$target" ] && print_resolv_conf | cmp -s "$target" -
}

case "${1:-}" in
    apply)
        write_resolv_conf
        ;;
    check)
        check_resolv_conf
        ;;
    *)
        echo "usage: ${0##*/} apply|check [resolv.conf]" >&2
        exit 2
        ;;
esac
