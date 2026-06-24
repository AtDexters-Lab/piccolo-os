#!/usr/bin/env bash
set -uo pipefail

usage() {
    cat >&2 <<'EOF'
Usage:
  scripts/validate-image-policy.sh [--profile PROFILE] [--arch ARCH] [MOUNTED_ROOT]

Validate high-signal Piccolo OS image policy in a mounted final image root.
The script only reads files from MOUNTED_ROOT; it does not boot, chroot, mount,
or use the network.

PROFILE may be VirtualBox, RaspberryPi, Rock64, SelfInstall, or common.
ARCH may be x86_64, i686, aarch64, or common. Passing profile/arch enables
profile- and architecture-specific boot policy checks.
EOF
}

profile=common
arch=common
mounted_root=/

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
        -h|--help)
            usage
            exit 0
            ;;
        --*)
            usage
            exit 2
            ;;
        *)
            if [ "$mounted_root" != "/" ]; then
                usage
                exit 2
            fi
            mounted_root=$1
            shift
            ;;
    esac
done

mounted_root=${mounted_root%/}
if [ -z "$mounted_root" ]; then
    mounted_root=/
fi

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
repo_root=$(cd "$script_dir/.." && pwd)
policy_script="$repo_root/packages/piccolo-os-support/piccolo-snapper-policy.sh"
firewalld_policy_script="$repo_root/packages/piccolo-os-support/piccolo-firewalld-policy.sh"

checks=0
failures=0
warnings=0
boot_options=
boot_options_source=

root_path() {
    local rel=${1#/}
    if [ "$mounted_root" = "/" ]; then
        printf '/%s' "$rel"
    else
        printf '%s/%s' "$mounted_root" "$rel"
    fi
}

ok() {
    checks=$((checks + 1))
    printf 'ok: %s\n' "$*"
}

fail() {
    failures=$((failures + 1))
    printf 'FAIL: %s\n' "$*" >&2
}

warn() {
    warnings=$((warnings + 1))
    printf 'warn: %s\n' "$*" >&2
}

require_file() {
    local rel=$1
    local desc=${2:-$rel}
    local path
    path=$(root_path "$rel")

    if [ -f "$path" ]; then
        ok "$desc exists"
    else
        fail "$desc missing at $path"
    fi
}

require_executable() {
    local rel=$1
    local desc=${2:-$rel}
    local path
    path=$(root_path "$rel")

    if [ -x "$path" ]; then
        ok "$desc is executable"
    else
        fail "$desc missing or not executable at $path"
    fi
}

require_dir() {
    local rel=$1
    local desc=${2:-$rel}
    local path
    path=$(root_path "$rel")

    if [ -d "$path" ]; then
        ok "$desc exists"
    else
        fail "$desc missing at $path"
    fi
}

require_regex() {
    local rel=$1
    local regex=$2
    local desc=$3
    local path
    path=$(root_path "$rel")

    if [ ! -f "$path" ]; then
        fail "$desc missing at $path"
    elif grep -Eq "$regex" "$path"; then
        ok "$desc"
    else
        fail "$desc not found in $path"
    fi
}

require_assignment() {
    local rel=$1
    local key=$2
    local expected=$3
    local desc=$4
    local path
    local actual
    path=$(root_path "$rel")

    if [ ! -f "$path" ]; then
        fail "$desc missing at $path"
        return
    fi

    actual=$(awk -F= -v key="$key" '
        /^[[:space:]]*#/ { next }
        {
            name = $1
            gsub(/^[[:space:]]+|[[:space:]]+$/, "", name)
            if (name == key) {
                value = $0
                sub(/^[^=]*=/, "", value)
                gsub(/^[[:space:]]+|[[:space:]]+$/, "", value)
                if (value ~ /^".*"$/) {
                    sub(/^"/, "", value)
                    sub(/"$/, "", value)
                }
                print value
                exit
            }
        }
    ' "$path")

    if [ "$actual" = "$expected" ]; then
        ok "$desc"
    elif [ -n "$actual" ]; then
        fail "$desc expected $key=$expected in $path, found $actual"
    else
        fail "$desc expected $key=$expected in $path, found <missing>"
    fi
}

run_privileged_read() {
    if [ "${PICCOLO_VALIDATE_PRIVILEGED_READ:-0}" = "1" ] && [ "$(id -u)" -ne 0 ]; then
        if ! command -v sudo >/dev/null 2>&1; then
            echo "validate-image-policy: sudo is required for privileged read checks" >&2
            return 127
        fi
        sudo "$@"
    else
        "$@"
    fi
}

require_optional_assignment() {
    local rel=$1
    local key=$2
    local expected=$3
    local desc=$4
    local path
    path=$(root_path "$rel")

    if [ ! -f "$path" ]; then
        warn "$desc skipped; optional file is absent at $path"
        return
    fi

    require_assignment "$rel" "$key" "$expected" "$desc"
}

require_same_file() {
    local rel=$1
    local expected_rel=$2
    local desc=$3
    local actual
    local expected
    actual=$(root_path "$rel")
    expected="$repo_root/$expected_rel"

    if [ ! -f "$actual" ]; then
        fail "$desc missing at $actual"
    elif [ ! -f "$expected" ]; then
        fail "$desc expected source missing at $expected"
    elif cmp -s "$expected" "$actual"; then
        ok "$desc matches repo source"
    else
        fail "$desc at $actual differs from $expected_rel"
    fi
}

require_enabled_unit() {
    local unit=$1
    local target=$2
    require_unit_link "$unit" "${target}.wants" "$unit enabled for $target"
}

require_unit_link() {
    local unit=$1
    local dependency_dir=$2
    local desc=$3
    local rel="etc/systemd/system/${dependency_dir}/${unit}"
    local path
    local link_target
    path=$(root_path "$rel")

    if [ ! -L "$path" ]; then
        fail "$desc missing at $path"
        return
    fi

    link_target=$(readlink "$path")
    case "$link_target" in
        */"$unit"|"$unit")
            ok "$desc"
            ;;
        *)
            fail "$desc symlink has unexpected target: $link_target"
            ;;
    esac
}

require_masked_unit() {
    local unit=$1
    local rel="etc/systemd/system/${unit}"
    local path
    local link_target
    path=$(root_path "$rel")

    if [ ! -L "$path" ]; then
        fail "$unit is not masked at $path"
        return
    fi

    link_target=$(readlink "$path")
    if [ "$link_target" = "/dev/null" ]; then
        ok "$unit masked"
    else
        fail "$unit mask points to $link_target, expected /dev/null"
    fi
}

load_boot_options() {
    local kernel_cmdline
    local grub_default
    kernel_cmdline=$(root_path etc/kernel/cmdline)
    grub_default=$(root_path etc/default/grub)

    if [ -f "$kernel_cmdline" ]; then
        boot_options=$(tr '\n' ' ' < "$kernel_cmdline")
        boot_options_source=$kernel_cmdline
    elif [ -f "$grub_default" ]; then
        boot_options=$(awk -F= '
            /^GRUB_CMDLINE_LINUX_DEFAULT=/ {
                value = $0
                sub(/^[^=]*=/, "", value)
                gsub(/^[[:space:]]+|[[:space:]]+$/, "", value)
                if (value ~ /^".*"$/) {
                    sub(/^"/, "", value)
                    sub(/"$/, "", value)
                }
                print value
                exit
            }
        ' "$grub_default")
        boot_options_source=$grub_default
    else
        fail "boot command line source missing: expected /etc/kernel/cmdline or /etc/default/grub"
    fi
}

require_boot_option() {
    local token=$1

    if [ -z "$boot_options_source" ]; then
        fail "boot option $token cannot be checked without a boot command line source"
        return
    fi

    case " $boot_options " in
        *" $token "*)
            ok "boot option $token present in $boot_options_source"
            ;;
        *)
            fail "boot option $token missing from $boot_options_source"
            ;;
    esac
}

check_rpm_package_installed() {
    local package=$1
    local output

    if ! command -v rpm >/dev/null 2>&1; then
        warn "rpm not available on validator host; skipping rpmdb check for $package"
        return
    fi

    local dbpath=
    if [ -d "$(root_path usr/lib/sysimage/rpm)" ]; then
        dbpath=/usr/lib/sysimage/rpm
    elif [ -d "$(root_path var/lib/rpm)" ]; then
        dbpath=/var/lib/rpm
    fi

    if [ -z "$dbpath" ]; then
        warn "no rpm database found under $mounted_root; skipping rpmdb check for $package"
        return
    fi

    if output=$(rpm --root "$mounted_root" --dbpath "$dbpath" -q "$package" 2>&1); then
        ok "rpm package installed: $output"
    else
        fail "rpm package $package is not installed according to rpmdb: $output"
    fi
}

check_snapper_policy() {
    local snapper_config
    snapper_config=$(root_path etc/snapper/configs/root)

    if [ ! -x "$policy_script" ]; then
        fail "missing executable policy helper: $policy_script"
    elif "$policy_script" check "$snapper_config"; then
        ok "Snapper appliance policy"
    else
        fail "Snapper appliance policy failed for $snapper_config"
    fi
}

check_firewalld_policy() {
    local firewalld_config
    firewalld_config=$(root_path etc/firewalld/firewalld.conf)

    if [ ! -x "$firewalld_policy_script" ]; then
        fail "missing executable policy helper: $firewalld_policy_script"
    elif run_privileged_read "$firewalld_policy_script" check "$firewalld_config"; then
        ok "firewalld default-zone policy"
    else
        fail "firewalld default-zone policy failed for $firewalld_config"
    fi
}

check_boot_profile() {
    local ignition_platform=

    load_boot_options
    require_boot_option quiet
    require_boot_option systemd.show_status=yes
    require_boot_option button.lid_init_state=open

    case "$arch" in
        x86_64|i386|i586|i686)
            require_boot_option reboot=cold
            ;;
        common|aarch64|arm64)
            ;;
        *)
            warn "unknown arch '$arch'; skipping architecture-specific boot checks"
            ;;
    esac

    case "$profile" in
        VirtualBox)
            ignition_platform=virtualbox
            ;;
        RaspberryPi|Rock64)
            ignition_platform=metal
            ;;
        SelfInstall|common)
            ;;
        *)
            warn "unknown profile '$profile'; skipping profile-specific boot checks"
            ;;
    esac

    if [ -n "$ignition_platform" ]; then
        require_boot_option "ignition.platform.id=$ignition_platform"
    fi

    if [ -f "$(root_path etc/selinux/config)" ]; then
        require_assignment etc/selinux/config SELINUX enforcing "SELinux enforcing"
        require_assignment etc/selinux/config SELINUXTYPE targeted "SELinux targeted policy"
        require_boot_option security=selinux
        require_boot_option selinux=1
    fi
}

check_config_sh_canaries() {
    require_assignment etc/vconsole.conf FONT eurlatgr.psfu "console font policy"
    require_optional_assignment etc/sysconfig/network/dhcp DHCLIENT_SET_HOSTNAME yes "DHCP hostname policy"
    require_regex etc/dracut.conf.d/50-microos-growfs.conf '/usr/lib/systemd/systemd-growfs' "growfs dracut item"
    require_optional_assignment etc/zypp/zypp.conf solver.onlyRequires true "zypp onlyRequires policy"
    require_optional_assignment etc/zypp/zypp.conf rpm.install.excludedocs yes "zypp excludedocs policy"
    require_optional_assignment etc/zypp/zypp.conf multiversion "" "zypp no multiversion kernels policy"
    require_regex var/lib/piccolo/clock-epoch '^[0-9]{10,}$' "clock epoch seed"
    check_boot_profile
}

check_support_package_static_files() {
    require_file usr/bin/piccolod "piccolod binary"
    require_same_file usr/lib/firewalld/zones/piccolo.xml packages/piccolo-os-support/piccolo.xml "firewalld piccolo zone"
    require_file etc/pki/rpm-gpg/RPM-GPG-KEY-piccolo-os "Piccolo OS rpm signing key"
    require_regex etc/zypp/repos.d/piccolo-os.repo '^enabled=1$' "Piccolo repo enabled"
    require_regex etc/zypp/repos.d/piccolo-os.repo '^repo_gpgcheck=1$' "Piccolo repo metadata gpg check"
    require_regex etc/zypp/repos.d/piccolo-os.repo '^gpgcheck=1$' "Piccolo repo package gpg check"
    require_regex etc/zypp/repos.d/piccolo-os.repo '^gpgkey=file:///etc/pki/rpm-gpg/RPM-GPG-KEY-piccolo-os$' "Piccolo repo local gpg key"
    require_assignment etc/transactional-update.conf REBOOT_METHOD none "transactional-update reboot policy"
    require_same_file usr/libexec/health-checker/piccolod.sh packages/piccolo-os-support/piccolo-health-check.sh "health-checker Piccolo plugin"
    require_executable usr/libexec/health-checker/piccolod.sh "health-checker Piccolo plugin"
    require_same_file usr/lib/systemd/system/health-checker.service.d/piccolo.conf packages/piccolo-os-support/health-checker-piccolo.conf "health-checker ordering drop-in"
    require_same_file usr/lib/systemd/logind.conf.d/piccolo.conf packages/piccolo-os-support/piccolo-logind.conf "logind appliance policy"
    require_same_file usr/lib/systemd/sleep.conf.d/piccolo.conf packages/piccolo-os-support/piccolo-sleep.conf "sleep appliance policy"
    require_same_file usr/lib/NetworkManager/conf.d/piccolo-wifi-powersave.conf packages/piccolo-os-support/piccolo-wifi-powersave.conf "NetworkManager WiFi powersave policy"
    require_same_file usr/lib/NetworkManager/conf.d/piccolo-bootstrap-dns.conf packages/piccolo-os-support/piccolo-bootstrap-dns.conf "NetworkManager bootstrap DNS policy"
    require_regex usr/lib/NetworkManager/conf.d/piccolo-bootstrap-dns.conf '^\[global-dns-domain-\*\]$' "NetworkManager bootstrap DNS wildcard domain"
    require_regex usr/lib/NetworkManager/conf.d/piccolo-bootstrap-dns.conf '^servers=1\.1\.1\.1;9\.9\.9\.9;$' "NetworkManager bootstrap DNS resolvers"
    require_same_file usr/libexec/piccolo/clock-epoch.sh packages/piccolo-os-support/piccolo-clock-epoch.sh "clock epoch helper"
    require_executable usr/libexec/piccolo/clock-epoch.sh "clock epoch helper"
    require_same_file usr/lib/systemd/system/piccolo-clock-epoch.service packages/piccolo-os-support/piccolo-clock-epoch.service "clock epoch service"
    require_same_file usr/lib/systemd/system/piccolo-clock-epoch-save.service packages/piccolo-os-support/piccolo-clock-epoch-save.service "clock epoch save service"
    require_same_file usr/lib/systemd/system/piccolo-clock-epoch-save.timer packages/piccolo-os-support/piccolo-clock-epoch-save.timer "clock epoch save timer"
    require_same_file etc/zypp/locks packages/piccolo-os-support/piccolo-zypp-locks "zypp forbidden package locks"
    require_same_file usr/lib/systemd/system.conf.d/piccolo.conf packages/piccolo-os-support/piccolo-system.conf "systemd watchdog policy"
    require_same_file usr/lib/sysctl.d/90-piccolo-panic-reboot.conf packages/piccolo-os-support/piccolo-panic-reboot.conf "kernel panic reboot policy"
    require_same_file usr/lib/modprobe.d/piccolo-watchdog.conf packages/piccolo-os-support/piccolo-watchdog.conf "watchdog module policy"
    require_same_file usr/libexec/piccolo/firewalld-policy.sh packages/piccolo-os-support/piccolo-firewalld-policy.sh "installed firewalld policy helper"
    require_executable usr/libexec/piccolo/firewalld-policy.sh "installed firewalld policy helper"
    require_same_file usr/libexec/piccolo/snapper-policy.sh packages/piccolo-os-support/piccolo-snapper-policy.sh "installed Snapper policy helper"
    require_executable usr/libexec/piccolo/snapper-policy.sh "installed Snapper policy helper"
    require_same_file usr/libexec/piccolo/watchdog-check.sh packages/piccolo-os-support/piccolo-watchdog-check.sh "watchdog validation helper"
    require_executable usr/libexec/piccolo/watchdog-check.sh "watchdog validation helper"
    require_same_file usr/lib/systemd/system/piccolo-watchdog-check.service packages/piccolo-os-support/piccolo-watchdog-check.service "watchdog validation service"
}

check_support_package_post_effects() {
    check_firewalld_policy
    require_enabled_unit firewalld.service multi-user.target
    require_unit_link health-checker.service boot-complete.target.requires "health-checker.service required by boot-complete.target"
    require_unit_link health-checker.service default.target.wants "health-checker.service wanted by default.target"
    require_enabled_unit piccolo-clock-epoch.service sysinit.target
    require_enabled_unit piccolo-clock-epoch-save.timer timers.target
    require_enabled_unit piccolo-watchdog-check.service multi-user.target
    require_enabled_unit NetworkManager.service multi-user.target
    require_masked_unit serial-getty@ttyS0.service
    require_masked_unit serial-getty@ttyS1.service
    require_masked_unit serial-getty@ttyS2.service
    require_masked_unit getty@tty1.service
    require_masked_unit sleep.target
    require_masked_unit suspend.target
    require_masked_unit hibernate.target
    require_masked_unit hybrid-sleep.target
    require_masked_unit suspend-then-hibernate.target
    require_regex etc/passwd '^piccolo-runtime:' "piccolo-runtime user"
    require_regex etc/group '^piccolo-runtime:' "piccolo-runtime group"
    require_regex etc/subuid '^piccolo-runtime:' "piccolo-runtime subordinate uid range"
    require_regex etc/subgid '^piccolo-runtime:' "piccolo-runtime subordinate gid range"
    require_file var/lib/systemd/linger/piccolo-runtime "piccolo-runtime linger marker"
    require_dir var/lib/piccolo "Piccolo persistent state directory"
}

case "$profile" in
    common|VirtualBox|RaspberryPi|Rock64|SelfInstall) ;;
    *)
        warn "profile '$profile' is not recognized; common checks will still run"
        ;;
esac

case "$arch" in
    common|x86_64|i386|i586|i686|aarch64|arm64) ;;
    *)
        warn "arch '$arch' is not recognized; common checks will still run"
        ;;
esac

check_rpm_package_installed piccolo-os-support
check_snapper_policy
check_config_sh_canaries
check_support_package_static_files
check_support_package_post_effects

if [ "$failures" -eq 0 ]; then
    printf 'validate-image-policy: PASS (%d checks, %d warnings)\n' "$checks" "$warnings"
    exit 0
fi

printf 'validate-image-policy: FAIL (%d failures, %d checks, %d warnings)\n' "$failures" "$checks" "$warnings" >&2
exit 1
