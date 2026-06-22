#!/usr/bin/env bash
set -euo pipefail

usage() {
    cat >&2 <<'EOF'
Usage:
  piccolo-snapper-policy.sh apply [CONFIG]
  piccolo-snapper-policy.sh check [CONFIG]

CONFIG defaults to /etc/snapper/configs/root.
EOF
}

policy_lines() {
    cat <<'EOF'
NUMBER_CLEANUP=yes
NUMBER_LIMIT=20
NUMBER_LIMIT_IMPORTANT=5
TIMELINE_CREATE=no
TIMELINE_CLEANUP=yes
TIMELINE_LIMIT_HOURLY=6
TIMELINE_LIMIT_DAILY=3
TIMELINE_LIMIT_WEEKLY=0
TIMELINE_LIMIT_MONTHLY=1
TIMELINE_LIMIT_QUARTERLY=0
TIMELINE_LIMIT_YEARLY=0
SPACE_LIMIT=0.25
FREE_LIMIT=0.35
EOF
}

ensure_snapper_config() {
    local config=$1
    local config_dir
    config_dir=$(dirname "$config")

    mkdir -p "$config_dir"
    if [ -f "$config" ]; then
        return 0
    fi

    if [ -f /etc/snapper/config-templates/default ]; then
        cp /etc/snapper/config-templates/default "$config"
    elif [ -f /usr/share/snapper/config-templates/default ]; then
        cp /usr/share/snapper/config-templates/default "$config"
    else
        echo "piccolo-snapper-policy: missing Snapper default config template" >&2
        return 1
    fi
}

set_config_value() {
    local config=$1
    local key=$2
    local value=$3

    if grep -q "^${key}=" "$config"; then
        sed -i "s|^${key}=.*|${key}=\"${value}\"|" "$config"
    else
        printf '%s="%s"\n' "$key" "$value" >> "$config"
    fi
}

ensure_root_registered() {
    local sysconfig=${PICCOLO_SNAPPER_SYSCONFIG:-/etc/sysconfig/snapper}

    if [ "${PICCOLO_SNAPPER_SKIP_SYSCONFIG:-0}" = "1" ] || [ ! -e "$sysconfig" ]; then
        return 0
    fi

    if grep -q '^SNAPPER_CONFIGS=' "$sysconfig"; then
        sed -i 's|^SNAPPER_CONFIGS=.*|SNAPPER_CONFIGS="root"|' "$sysconfig"
    else
        printf '\nSNAPPER_CONFIGS="root"\n' >> "$sysconfig"
    fi
}

apply_policy() {
    local config=$1
    local key
    local value

    ensure_snapper_config "$config"
    while IFS='=' read -r key value; do
        set_config_value "$config" "$key" "$value"
    done < <(policy_lines)
    ensure_root_registered
}

check_policy() {
    local config=$1
    local key
    local value
    local actual
    local ok=0

    if [ ! -f "$config" ]; then
        echo "piccolo-snapper-policy: missing Snapper config: $config" >&2
        return 1
    fi

    while IFS='=' read -r key value; do
        if ! grep -Fxq "${key}=\"${value}\"" "$config"; then
            actual=$(grep -E "^${key}=" "$config" || true)
            if [ -z "$actual" ]; then
                actual="<missing>"
            fi
            echo "piccolo-snapper-policy: expected ${key}=\"${value}\" in $config, found $actual" >&2
            ok=1
        fi
    done < <(policy_lines)

    return "$ok"
}

command=${1:-apply}
case "$command" in
    apply|check)
        shift || true
        config=${1:-/etc/snapper/configs/root}
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
