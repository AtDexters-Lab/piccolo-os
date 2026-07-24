#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
repo_root=$(cd "$script_dir/.." && pwd)
image=${PICCOLO_TUMBLEWEED_IMAGE:-registry.opensuse.org/opensuse/tumbleweed@sha256:2fd91489ebde676125aabd13ffe98442f5496381d46d2e7a96cfe470763500e6}

command -v podman >/dev/null 2>&1 || {
    echo "validate-build-key-reconcile: podman is required" >&2
    exit 1
}
podman image exists "$image" || {
    echo "validate-build-key-reconcile: pinned image is not local: $image" >&2
    exit 1
}

podman run --rm --pull=never --network=none --read-only \
    --tmpfs /tmp:rw,exec,nosuid,nodev,size=256m \
    --tmpfs /root:rw,nosuid,nodev,size=32m \
    --tmpfs /run:rw,nosuid,nodev,size=32m \
    --volume "$repo_root/packages/piccolo-os-support/piccolo-build-key-reconcile.sh:/work/reconcile:ro" \
    --volume "$repo_root/scripts/fixtures/gpg-pubkey-29b700a4-62b07e22.asc:/work/old.asc:ro" \
    "$image" bash -lc '
        set -euo pipefail
        root=$(mktemp -d)
        trap "rm -rf \"$root\"" EXIT

        fingerprint() {
            local key_file=$1
            local -a fields=()
            while IFS=: read -r -a fields; do
                if [ "${fields[0]:-}" = fpr ]; then
                    printf "%s\n" "${fields[9]^^}"
                    return
                fi
            done < <(gpg --batch --quiet --no-options --show-keys \
                --with-colons "$key_file" 2>/dev/null)
            return 1
        }

        old_package=gpg-pubkey-29b700a4-62b07e22
        old_fingerprint=$(fingerprint /work/old.asc)
        renewed_file=
        while IFS= read -r key_file; do
            candidate_fingerprint=$(fingerprint "$key_file")
            if [ "$candidate_fingerprint" = "$old_fingerprint" ]; then
                renewed_file=$key_file
                break
            fi
        done < <(rpm -ql openSUSE-build-key |
            grep -E "^/usr/lib/rpm/gnupg/keys/gpg-pubkey-.*[.]asc$")
        test -n "$renewed_file"
        renewed_package=${renewed_file##*/}
        renewed_package=${renewed_package%.asc}

        mkdir -p "$root/usr/lib/sysimage" "$root/usr/lib/rpm/gnupg"
        cp -a /usr/lib/sysimage/rpm "$root/usr/lib/sysimage/rpm"
        cp -a /usr/lib/rpm/gnupg/keys "$root/usr/lib/rpm/gnupg/keys"

        rpmkeys --root "$root" --delete "${renewed_package#gpg-pubkey-}"
        rpmkeys --root "$root" --import /work/old.asc
        rpm --root "$root" -q "$old_package" >/dev/null

        touch /run/piccolo-build-key-reconcile.bypass
        /work/reconcile init-post "$root" 1
        rpm --root "$root" -q "$old_package" >/dev/null
        rm /run/piccolo-build-key-reconcile.bypass

        /work/reconcile init-post "$root" 1
        rpm --root "$root" -q "$renewed_package" >/dev/null
        ! rpm --root "$root" -q "$old_package" >/dev/null 2>&1

        before=$(rpm --root "$root" -qa \
            --qf "gpg-pubkey-%{VERSION}-%{RELEASE}\n" "gpg-pubkey*" |
            LC_ALL=C sort)
        /work/reconcile callExt-post "$root" 2
        after=$(rpm --root "$root" -qa \
            --qf "gpg-pubkey-%{VERSION}-%{RELEASE}\n" "gpg-pubkey*" |
            LC_ALL=C sort)
        test "$before" = "$after"
    '

echo "validate-build-key-reconcile: PASS"
