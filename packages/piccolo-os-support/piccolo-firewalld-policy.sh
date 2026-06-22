#!/usr/bin/env bash
set -euo pipefail

usage() {
    cat >&2 <<'EOF'
Usage:
  piccolo-firewalld-policy.sh apply [CONFIG]
  piccolo-firewalld-policy.sh check [CONFIG]

CONFIG defaults to /etc/firewalld/firewalld.conf.
EOF
}

zone=piccolo

set_config_value() {
    local config=$1
    local key=$2
    local value=$3

    if grep -Eq "^[[:space:]]*${key}[[:space:]]*=" "$config"; then
        sed -i "s|^[[:space:]]*${key}[[:space:]]*=.*|${key}=${value}|" "$config"
    else
        printf '%s=%s\n' "$key" "$value" >> "$config"
    fi
}

apply_policy() {
    local config=$1
    local config_dir
    config_dir=$(dirname "$config")

    if [ ! -d "$config_dir" ]; then
        install -d -m 750 "$config_dir"
    fi
    if [ ! -f "$config" ]; then
        install -m 644 /dev/null "$config"
    fi
    set_config_value "$config" DefaultZone "$zone"
}

check_policy() {
    local config=$1
    local actual

    if [ ! -f "$config" ]; then
        echo "piccolo-firewalld-policy: missing firewalld config: $config" >&2
        return 1
    fi

    if grep -Fxq "DefaultZone=${zone}" "$config"; then
        return 0
    fi

    actual=$(grep -E '^DefaultZone=' "$config" || true)
    if [ -z "$actual" ]; then
        actual="<missing>"
    fi
    echo "piccolo-firewalld-policy: expected DefaultZone=${zone} in $config, found $actual" >&2
    return 1
}

command=${1:-apply}
case "$command" in
    apply|check)
        shift || true
        config=${1:-/etc/firewalld/firewalld.conf}
        ;;
    -h|--help|help)
        usage
        exit 0
        ;;
    *)
        usage
        exit 2
        ;;
esac

case "$command" in
    apply)
        apply_policy "$config"
        check_policy "$config"
        ;;
    check)
        check_policy "$config"
        ;;
esac
