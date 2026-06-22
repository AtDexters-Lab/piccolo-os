#!/usr/bin/env bash
set -euo pipefail

usage() {
    cat >&2 <<'EOF'
Usage:
  scripts/validate-obs-image.sh [--profile PROFILE] [--arch ARCH] [--subvol SUBVOL] [--keep] ARTIFACT_OR_URL

Download or read an OBS-built Piccolo OS disk artifact, mount the Btrfs root
snapshot read-only, and run scripts/validate-image-policy.sh against the final
filesystem.

Examples:
  scripts/validate-obs-image.sh --profile VirtualBox --arch x86_64 \
    ~/Downloads/piccolo-os.x86_64-VirtualBox.vdi.xz

  scripts/validate-obs-image.sh --profile VirtualBox --arch x86_64 \
    https://download.opensuse.org/repositories/home:/atdexterslab:/piccolo-os/home_atdexterslab_atdexterslab_tumbleweed/piccolo-os.x86_64-VirtualBox.vdi.xz

The script needs mount privileges. If run as a normal user, it uses sudo for
mount/umount only.
EOF
}

profile=common
arch=common
subvol='@/.snapshots/1/snapshot'
keep=0
artifact=

while [ "$#" -gt 0 ]; do
    case "$1" in
        --profile)
            [ "$#" -ge 2 ] || { usage; exit 2; }
            profile=$2
            shift 2
            ;;
        --profile=*)
            profile=${1#--profile=}
            shift
            ;;
        --arch)
            [ "$#" -ge 2 ] || { usage; exit 2; }
            arch=$2
            shift 2
            ;;
        --arch=*)
            arch=${1#--arch=}
            shift
            ;;
        --subvol)
            [ "$#" -ge 2 ] || { usage; exit 2; }
            subvol=$2
            shift 2
            ;;
        --subvol=*)
            subvol=${1#--subvol=}
            shift
            ;;
        --keep)
            keep=1
            shift
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        --*)
            usage
            exit 2
            ;;
        *)
            if [ -n "$artifact" ]; then
                usage
                exit 2
            fi
            artifact=$1
            shift
            ;;
    esac
done

if [ -z "$artifact" ]; then
    usage
    exit 2
fi

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
repo_root=$(cd "$script_dir/.." && pwd)
inner_validator="$repo_root/scripts/validate-image-policy.sh"

require_tool() {
    local tool=$1
    if ! command -v "$tool" >/dev/null 2>&1; then
        echo "validate-obs-image: missing required tool: $tool" >&2
        exit 1
    fi
}

require_tool qemu-img
require_tool sfdisk
require_tool mount
require_tool umount
require_tool xz

if [ ! -x "$inner_validator" ]; then
    echo "validate-obs-image: missing executable validator: $inner_validator" >&2
    exit 1
fi

if [ "$(id -u)" -eq 0 ]; then
    sudo_cmd=()
else
    require_tool sudo
    sudo_cmd=(sudo)
fi

tmpdir=$(mktemp -d /tmp/piccolo-obs-image.XXXXXX)
mount_dir="$tmpdir/root"
raw_image="$tmpdir/image.raw"
mounted_paths=()

cleanup() {
    local index
    for ((index=${#mounted_paths[@]} - 1; index >= 0; index--)); do
        "${sudo_cmd[@]}" umount "${mounted_paths[$index]}" || true
    done
    if [ "$keep" -eq 1 ]; then
        echo "validate-obs-image: kept workspace: $tmpdir" >&2
    else
        rm -rf "$tmpdir"
    fi
}
trap cleanup EXIT

mkdir -p "$mount_dir"

case "$artifact" in
    http://*|https://*)
        require_tool curl
        artifact_name=${artifact%%\?*}
        artifact_name=${artifact_name##*/}
        if [ -z "$artifact_name" ]; then
            artifact_name=artifact
        fi
        artifact_file="$tmpdir/$artifact_name"
        echo "validate-obs-image: downloading $artifact" >&2
        curl --fail --location --show-error --output "$artifact_file" "$artifact"
        ;;
    *)
        artifact_file=$artifact
        if [ ! -f "$artifact_file" ]; then
            echo "validate-obs-image: artifact not found: $artifact_file" >&2
            exit 1
        fi
        ;;
esac

case "$artifact_file" in
    *.xz)
        image_file="$tmpdir/image"
        echo "validate-obs-image: decompressing artifact" >&2
        xz -dc "$artifact_file" > "$image_file"
        ;;
    *)
        image_file=$artifact_file
        ;;
esac

echo "validate-obs-image: converting artifact to raw" >&2
qemu-img convert -O raw "$image_file" "$raw_image"

partition_table=$(sfdisk -d "$raw_image")
sector_size=$(printf '%s\n' "$partition_table" | awk -F: '
    /sector-size:/ {
        gsub(/^[[:space:]]+|[[:space:]]+$/, "", $2)
        print $2
        exit
    }
')
if [ -z "$sector_size" ]; then
    sector_size=512
fi

read -r start_sector partition_size < <(
    printf '%s\n' "$partition_table" | awk '
        /start=/ && /size=/ {
            start = ""
            size = ""
            for (i = 1; i <= NF; i++) {
                if ($i == "start=" && (i + 1) <= NF) {
                    start = $(i + 1)
                } else if ($i ~ /^start=/) {
                    start = $i
                    sub(/^start=/, "", start)
                }
                if ($i == "size=" && (i + 1) <= NF) {
                    size = $(i + 1)
                } else if ($i ~ /^size=/) {
                    size = $i
                    sub(/^size=/, "", size)
                }
            }
            gsub(/,/, "", start)
            gsub(/,/, "", size)
            if ((size + 0) > max_size) {
                max_size = size + 0
                max_start = start + 0
            }
        }
        END {
            if (max_size > 0) {
                print max_start, max_size
            }
        }
    '
)

if [ -z "${start_sector:-}" ] || [ -z "${partition_size:-}" ]; then
    echo "validate-obs-image: failed to locate root partition in $raw_image" >&2
    exit 1
fi

offset=$((start_sector * sector_size))
echo "validate-obs-image: mounting largest partition read-only at offset $offset, subvol=$subvol" >&2
"${sudo_cmd[@]}" mount -o "ro,loop,offset=${offset},subvol=${subvol}" "$raw_image" "$mount_dir"
mounted_paths+=("$mount_dir")

if [ -d "$mount_dir/var" ]; then
    echo "validate-obs-image: mounting /var subvolume read-only" >&2
    "${sudo_cmd[@]}" mount -o "ro,loop,offset=${offset},subvol=@/var" "$raw_image" "$mount_dir/var"
    mounted_paths+=("$mount_dir/var")
fi

"$inner_validator" --profile "$profile" --arch "$arch" "$mount_dir"
