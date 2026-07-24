#!/bin/bash
set -euo pipefail
export LC_ALL=C
export PATH=/usr/sbin:/usr/bin:/sbin:/bin
readonly bypass_marker=/run/piccolo-build-key-reconcile.bypass
readonly distro_key_pattern='^/usr/lib/rpm/gnupg/keys/gpg-pubkey-[0-9A-Fa-f]{8}-[0-9A-Fa-f]{8}[.]asc$'
readonly piccolo_key_pattern='^/etc/pki/rpm-gpg/RPM-GPG-KEY-piccolo-os[-.A-Za-z0-9_]*$'
log() {
    printf 'piccolo-build-key-reconcile: %s\n' "$*" >&2
}
die() {
    log "ERROR: $*"
    exit 1
}
bounded() {
    timeout --signal=TERM --kill-after=5s 30s "$@"
}
metadata_fingerprint() {
    local metadata=$1
    local -a fields=()
    while IFS=: read -r -a fields; do
        if [ "${fields[0]:-}" = fpr ]; then
            printf '%s\n' "${fields[9]^^}"
            return 0
        fi
    done <<<"$metadata"
    return 1
}
key_fingerprint() {
    local key_file=$1
    local metadata
    local fingerprint
    metadata=$(bounded gpg --batch --quiet --no-options --no-autostart \
        --homedir "$gpg_home" --show-keys --with-colons "$key_file" 2>/dev/null) ||
        return 1
    [ "$(printf '%s\n' "$metadata" | grep -c '^pub:')" -eq 1 ] || return 1
    fingerprint=$(metadata_fingerprint "$metadata") || return 1
    [[ "$fingerprint" =~ ^[0-9A-F]{40}$ ]] || return 1
    printf '%s\n' "$fingerprint"
}
rpm_key_fingerprint() {
    local root=$1
    local package=$2
    local description
    local metadata
    description=$(bounded rpm --root "$root" -q "$package" \
        --qf '%{DESCRIPTION}' 2>/dev/null) || return 1
    metadata=$(printf '%s\n' "$description" |
        bounded gpg --batch --quiet --no-options --no-autostart \
            --homedir "$gpg_home" --show-keys --with-colons 2>/dev/null) ||
        return 1
    metadata_fingerprint "$metadata"
}
expected_key_package() {
    local key_file=$1
    local probe_db
    local packages
    probe_db=$(mktemp -d "$work_dir/rpm-probe.XXXXXX")
    bounded rpm --dbpath "$probe_db" --initdb >/dev/null
    bounded rpmkeys --dbpath "$probe_db" --import "$key_file" >/dev/null
    packages=$(bounded rpm --dbpath "$probe_db" -qa \
        --qf 'gpg-pubkey-%{VERSION}-%{RELEASE}\n' 'gpg-pubkey*')
    rm -rf "$probe_db"
    [ "$(printf '%s\n' "$packages" | grep -c '^gpg-pubkey-')" -eq 1 ] ||
        return 1
    printf '%s\n' "$packages"
}
installed_key_packages() {
    local root=$1
    bounded rpm --root "$root" -qa \
        --qf 'gpg-pubkey-%{VERSION}-%{RELEASE}\n' 'gpg-pubkey*' 2>/dev/null |
        LC_ALL=C sort -u
}
reconcile_key() {
    local root=$1
    local key_file=$2
    local fingerprint
    local expected
    local package
    local packages
    local installed_fingerprint
    local -a matches=()
    fingerprint=$(key_fingerprint "$key_file") ||
        die "cannot read one primary OpenPGP key from $key_file"
    expected=$(expected_key_package "$key_file") ||
        die "cannot determine RPM identity for $key_file"
    packages=$(installed_key_packages "$root") ||
        die "cannot list installed signing keys under $root"
    while IFS= read -r package; do
        [ -n "$package" ] || continue
        installed_fingerprint=$(rpm_key_fingerprint "$root" "$package") ||
            die "cannot read fingerprint for $package under $root"
        if [ "$installed_fingerprint" = "$fingerprint" ]; then
            matches+=("$package")
        fi
    done <<<"$packages"
    if [ "${#matches[@]}" -eq 1 ] && [ "${matches[0]}" = "$expected" ]; then
        return 0
    fi
    for package in "${matches[@]}"; do
        log "removing superseded $package"
        bounded rpmkeys --root "$root" --delete "${package#gpg-pubkey-}" ||
            die "cannot remove $package"
    done
    log "importing $expected"
    bounded rpmkeys --root "$root" --import "$key_file" ||
        die "cannot import $expected"
    bounded rpm --root "$root" -q "$expected" >/dev/null 2>&1 ||
        die "$expected is absent after import"
}
reconcile_package_keys() {
    local root=$1
    local package=$2
    local pattern=$3
    local required=$4
    local package_names
    local package_paths
    local relative_path
    local key_file
    local count=0
    package_names=$(bounded rpm --root "$root" -qa --qf '%{NAME}\n' 2>/dev/null) ||
        die "cannot query installed packages under $root"
    if ! grep -Fxq "$package" <<<"$package_names"; then
        [ "$required" = no ] && return 0
        die "$package is not installed under $root"
    fi
    package_paths=$(bounded rpm --root "$root" -ql "$package") ||
        die "cannot query files owned by $package under $root"
    while IFS= read -r relative_path; do
        [[ "$relative_path" =~ $pattern ]] || continue
        key_file=$root$relative_path
        [ -s "$key_file" ] || die "package-owned key is missing: $relative_path"
        reconcile_key "$root" "$key_file"
        count=$((count + 1))
    done <<<"$package_paths"
    [ "$count" -gt 0 ] || die "$package owns no signing keys"
}
reconcile_root() {
    local root=${1%/}
    [ -n "$root" ] || root=/
    if [[ "$root" != /* ]] || [ ! -d "$root" ]; then
        die "invalid snapshot root: $root"
    fi
    if [ -e "$bypass_marker" ]; then
        log "WARNING: bypass active at $bypass_marker; skipping key reconciliation"
        return 0
    fi
    work_dir=$(mktemp -d)
    gpg_home=$work_dir/gnupg
    install -d -m 700 "$gpg_home"
    trap 'rm -rf "$work_dir"' EXIT
    reconcile_package_keys "$root" openSUSE-build-key "$distro_key_pattern" yes
    reconcile_package_keys "$root" piccolo-os-support "$piccolo_key_pattern" no
}
case "${1:-}" in
    init-post|resume-post|execute-post|callExt-post)
        [ "$#" -ge 2 ] || die "$1 did not provide a snapshot root"
        reconcile_root "$2"
        ;;
    *-pre|*-post)
        exit 0
        ;;
    *)
        die "unsupported Tukit stage: ${1:-<missing>}"
        ;;
esac
