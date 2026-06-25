Name:           piccolo-os-support
Version:        0.3.10
Release:        0
Summary:        Piccolo OS policy/meta package
License:        AGPL-3.0-or-later
URL:            https://github.com/AtDexters-Lab/piccolo-os
# noarch meta-package; arch-specific Requires (kernel-default, etc.)
# resolved by OBS per-arch repos at install time.
BuildArch:      noarch
Source0:        piccolo.xml
Source2:        piccolo-os.key
Source3:        piccolo-health-check.sh
Source4:        health-checker-piccolo.conf
Source5:        piccolo-logind.conf
Source6:        piccolo-sleep.conf
Source7:        piccolo-wifi-powersave.conf
Source11:       piccolo-clock-epoch.sh
Source12:       piccolo-clock-epoch.service
Source13:       piccolo-clock-epoch-save.service
Source14:       piccolo-clock-epoch-save.timer
Source15:       piccolo-zypp-locks
Source16:       piccolo-system.conf
Source17:       piccolo-panic-reboot.conf
Source18:       piccolo-watchdog.conf
Source19:       piccolo-watchdog-check.sh
Source20:       piccolo-watchdog-check.service
Source21:       piccolo-snapper-policy.sh
Source22:       piccolo-firewalld-policy.sh
Source23:       piccolo-bootstrap-dns.conf
Source24:       piccolo-bootstrap-dns.sh

# ==============================================================================
# KEY ROTATION SOP (STANDARD OPERATING PROCEDURE)
# ==============================================================================
# CRITICAL: This package is the "Key of Keys". If the GPG key expires before
# users have the new one, the fleet is bricked (updates will fail validation).
#
# STRATEGY: Rotate & Overlap (File-Based Trust)
# 1. 3 months before expiry (Current Key Expires: 2028-01-23):
#    - Generate/Extend key in OBS.
#    - Download new key to 'piccolo-os.key.new'.
#    - Add it as Source3.
#    - Update %%install to ship BOTH keys to /etc/pki/rpm-gpg/.
#    - Update %%install to list BOTH keys in the .repo file:
#      gpgkey=file:///etc/pki/rpm-gpg/KEY-OLD
#             file:///etc/pki/rpm-gpg/KEY-NEW
#    - Ship this update signed by the OLD key.
# 2. Once fleet has updated:
#    - Switch OBS to sign metadata with the NEW key.
#    - Zypper will automatically trust the new key because the file exists locally
#      and is referenced in the repo config. No 'rpm --import' needed.
# ==============================================================================

BuildRequires:  systemd
BuildRequires:  firewalld
BuildRequires:  libxml2-tools
BuildRequires:  snapper
# --- Piccolo policy deps (not from upstream patterns) ---
# piccolod is the local control plane and health-check target. Keep this
# unversioned: its package version follows piccolod releases, not
# piccolo-os-support policy-package versions.
Requires:       piccolod
Requires:       firewalld
Requires(post): snapper

# --- Flattened from patterns-microos-basesystem (microos_base) ---
# See ci/upstream-microos-base-requires.txt for upstream baseline and exclusions.
Requires:       patterns-base-minimal_base
Requires:       aaa_base
Requires:       bash
Requires:       btrfsmaintenance
Requires:       btrfsprogs
Requires:       build-key
Requires:       busybox
Requires:       ca-certificates
Requires:       ca-certificates-mozilla
Requires:       chrony
Requires:       coreutils
Requires:       coreutils-systemd
Requires:       curl
Requires:       dosfstools
Requires:       glibc
Requires:       glibc-locale-base
Requires:       gzip
Requires:       health-checker
Requires:       health-checker-plugins-MicroOS
Requires:       hostname
Requires:       iproute2
Requires:       iputils
Requires:       less
Requires:       libnss_usrfiles2
Requires:       libtss2-tcti-device0
Requires:       MicroOS-release
Requires:       microos-tools
Requires:       NetworkManager
Requires:       NetworkManager-wifi
Requires:       pam
Requires:       pam-config
Requires:       procps
Requires:       rpm
Requires:       shadow
Requires:       snapper
Requires:       sysextmgr
Requires:       systemd
Requires:       systemd-presets-branding-MicroOS
Requires:       terminfo-base
Requires:       timezone
Requires:       tpm2-0-tss
Requires:       tpm2.0-tools
Requires:       util-linux
Requires:       vim-small
Requires:       grub2-snapper-plugin

# --- Flattened from patterns-microos-base-zypper ---
Requires:       transactional-update
Requires:       transactional-update-zypp-config
Requires:       zypp-boot-plugin
Requires:       zypper
Requires:       zypper-needs-restarting
Requires:       zypp-excludedocs
Requires:       zypp-no-multiversion
Requires:       zypp-no-recommends

# --- Flattened from patterns-microos-defaults (remaining deps in image-composition below) ---
Requires:       audit
Requires:       sndiff

# --- Piccolo OS image composition (not from upstream patterns) ---
Requires:       patterns-microos-selinux
Requires:       kernel-default
Requires:       device-mapper
Requires:       cryptsetup
Requires:       read-only-root-fs >= 1.0+git20250414
Requires:       patterns-containers-container_runtime
Requires:       growpart-generator
Requires:       patterns-base-bootloader

# --- Scriptlet deps ---
Requires(post): shadow
Requires(post): systemd
Requires(post): bash
Requires(preun): systemd

%description
Piccolo OS support is a lightweight policy package that ensures the Piccolo control
plane (`piccolod`) is present on every image. Future revisions will add transactional
update helpers, service watchdogs, and access-hardening defaults.

%prep
# Nothing to prep yet.

%build
# No build artifacts for this meta package.

%install
# 1. Install Firewalld Zone
install -D -m 644 %{SOURCE0} %{buildroot}%{_prefix}/lib/firewalld/zones/piccolo.xml

# 2. Install GPG Key to Canonical System Location
# We rename it to the standard RPM-GPG-KEY format so it sits alongside distro keys
install -D -m 644 %{SOURCE2} %{buildroot}%{_sysconfdir}/pki/rpm-gpg/RPM-GPG-KEY-piccolo-os

# 3. Create Repo File
install -d -m 755 %{buildroot}%{_sysconfdir}/zypp/repos.d

# Define Unified Repo URL (works for both x86_64 and aarch64)
REPO_URL="https://download.opensuse.org/repositories/home:/atdexterslab/atdexterslab_tumbleweed/"

# Generate the repo file
# CRITICAL: gpgkey points to the LOCAL file we just installed.
# This ensures zypper trusts the repo immediately without interactive prompts.
cat > %{buildroot}%{_sysconfdir}/zypp/repos.d/piccolo-os.repo <<EOF
[piccolo-os]
name=Piccolo OS
enabled=1
autorefresh=1
baseurl=$REPO_URL
type=rpm-md
gpgcheck=1
repo_gpgcheck=1
gpgkey=file://%{_sysconfdir}/pki/rpm-gpg/RPM-GPG-KEY-piccolo-os
EOF

# 4. Configure Transactional Update
# Disable automatic reboots by transactional-update.
# We want to control reboots via piccolod/user interaction, especially for TPM unlocks.
echo "REBOOT_METHOD=none" > %{buildroot}%{_sysconfdir}/transactional-update.conf

# 5. Install Health Check Plugin
# Used by health-checker.service to validate boot success.
install -d -m 755 %{buildroot}%{_libexecdir}/health-checker
install -m 755 %{SOURCE3} %{buildroot}%{_libexecdir}/health-checker/piccolod.sh

# 6. Install Health Checker Ordering Drop-in
install -d -m 755 %{buildroot}%{_prefix}/lib/systemd/system/health-checker.service.d
install -m 644 %{SOURCE4} %{buildroot}%{_prefix}/lib/systemd/system/health-checker.service.d/piccolo.conf

# 7. Always-on power policy: ignore lid close, power key, and sleep keys
install -D -m 644 %{SOURCE5} %{buildroot}%{_prefix}/lib/systemd/logind.conf.d/piccolo.conf

# 8. Disable all sleep states at systemd level
install -D -m 644 %{SOURCE6} %{buildroot}%{_prefix}/lib/systemd/sleep.conf.d/piccolo.conf

# 9. Disable WiFi power saving for reliable LAN connectivity
install -D -m 644 %{SOURCE7} %{buildroot}%{_prefix}/lib/NetworkManager/conf.d/piccolo-wifi-powersave.conf

# 10. Keep host DNS independent from router-provided LAN DNS.
install -D -m 644 %{SOURCE23} %{buildroot}%{_prefix}/lib/NetworkManager/conf.d/piccolo-bootstrap-dns.conf

# 11. Persistent state directory (used by clock-epoch, piccolod watchdog)
install -d -m 755 %{buildroot}/var/lib/piccolo

# 12. Host bootstrap DNS helper
install -D -m 755 %{SOURCE24} %{buildroot}%{_libexecdir}/piccolo/bootstrap-dns.sh

# 13. Install clock epoch service for RTC-less devices
install -D -m 755 %{SOURCE11} %{buildroot}%{_libexecdir}/piccolo/clock-epoch.sh
install -D -m 644 %{SOURCE12} %{buildroot}%{_prefix}/lib/systemd/system/piccolo-clock-epoch.service
install -D -m 644 %{SOURCE13} %{buildroot}%{_prefix}/lib/systemd/system/piccolo-clock-epoch-save.service
install -D -m 644 %{SOURCE14} %{buildroot}%{_prefix}/lib/systemd/system/piccolo-clock-epoch-save.timer

# 14. Install zypp package locks to prevent transactional-update from reinstalling removed packages
install -D -m 644 %{SOURCE15} %{buildroot}%{_sysconfdir}/zypp/locks

# 15. Hardware watchdog: auto-reboot on kernel/PID-1 freeze (platform-agnostic)
install -D -m 644 %{SOURCE16} %{buildroot}%{_prefix}/lib/systemd/system.conf.d/piccolo.conf

# 16. Auto-reboot on kernel panic/oops
install -D -m 644 %{SOURCE17} %{buildroot}%{_prefix}/lib/sysctl.d/90-piccolo-panic-reboot.conf

# 17. Watchdog module selection and boot-time validation
install -D -m 644 %{SOURCE18} %{buildroot}%{_prefix}/lib/modprobe.d/piccolo-watchdog.conf
install -D -m 755 %{SOURCE19} %{buildroot}%{_libexecdir}/piccolo/watchdog-check.sh
install -D -m 644 %{SOURCE20} %{buildroot}%{_prefix}/lib/systemd/system/piccolo-watchdog-check.service

# 18. Snapper retention policy for appliance-sized root filesystems
install -D -m 755 %{SOURCE21} %{buildroot}%{_libexecdir}/piccolo/snapper-policy.sh

# 19. Firewalld default-zone policy for appliance firewall posture
install -D -m 755 %{SOURCE22} %{buildroot}%{_libexecdir}/piccolo/firewalld-policy.sh

%check
# Validate the firewall zone XML
xmllint --noout %{buildroot}%{_prefix}/lib/firewalld/zones/piccolo.xml

# Validate zypp locks file has exactly 4 locked packages with expected names
grep -c 'solvable_name:' %{buildroot}%{_sysconfdir}/zypp/locks | grep -q '^4$'
for pkg in openssh openssh-server openssh-clients rebootmgr; do
    grep -q "solvable_name: $pkg$" %{buildroot}%{_sysconfdir}/zypp/locks || exit 1
done

# Validate bootstrap DNS policy keeps the host resolver independent from LAN DNS.
grep -q '^\[main\]$' %{buildroot}%{_prefix}/lib/NetworkManager/conf.d/piccolo-bootstrap-dns.conf
grep -q '^dns=none$' %{buildroot}%{_prefix}/lib/NetworkManager/conf.d/piccolo-bootstrap-dns.conf
BOOTSTRAP_DNS_TEST=$(mktemp)
trap 'rm -f "$BOOTSTRAP_DNS_TEST"' EXIT
%{buildroot}%{_libexecdir}/piccolo/bootstrap-dns.sh apply "$BOOTSTRAP_DNS_TEST"
%{buildroot}%{_libexecdir}/piccolo/bootstrap-dns.sh check "$BOOTSTRAP_DNS_TEST"
printf '%s\n' 'nameserver 192.168.0.201' >> "$BOOTSTRAP_DNS_TEST"
! %{buildroot}%{_libexecdir}/piccolo/bootstrap-dns.sh check "$BOOTSTRAP_DNS_TEST"

# Validate the Snapper appliance policy helper.
SNAPPER_TEST_CONFIG=$(mktemp)
trap 'rm -f "$BOOTSTRAP_DNS_TEST" "$SNAPPER_TEST_CONFIG"' EXIT
cat > "$SNAPPER_TEST_CONFIG" <<'EOF'
NUMBER_CLEANUP="no"
NUMBER_LIMIT="50"
NUMBER_LIMIT_IMPORTANT="10"
TIMELINE_CREATE="yes"
TIMELINE_CLEANUP="yes"
TIMELINE_LIMIT_HOURLY="10"
TIMELINE_LIMIT_DAILY="10"
TIMELINE_LIMIT_MONTHLY="10"
TIMELINE_LIMIT_YEARLY="10"
SPACE_LIMIT="0.5"
FREE_LIMIT="0.2"
EOF
PICCOLO_SNAPPER_SKIP_SYSCONFIG=1 %{buildroot}%{_libexecdir}/piccolo/snapper-policy.sh apply "$SNAPPER_TEST_CONFIG"
PICCOLO_SNAPPER_SKIP_SYSCONFIG=1 %{buildroot}%{_libexecdir}/piccolo/snapper-policy.sh check "$SNAPPER_TEST_CONFIG"

# Validate the firewalld default-zone helper.
FIREWALLD_TEST_CONFIG=$(mktemp)
FIREWALLD_TEST_DIR=$(mktemp -d)
FIREWALLD_NEW_CONFIG="$FIREWALLD_TEST_DIR/firewalld/firewalld.conf"
trap 'rm -f "$BOOTSTRAP_DNS_TEST" "$SNAPPER_TEST_CONFIG" "$FIREWALLD_TEST_CONFIG"; rm -rf "$FIREWALLD_TEST_DIR"' EXIT
cat > "$FIREWALLD_TEST_CONFIG" <<'EOF'
DefaultZone=public
CleanupOnExit=yes
EOF
FIREWALLD_TEST_PARENT=$(dirname "$FIREWALLD_TEST_CONFIG")
FIREWALLD_TEST_PARENT_MODE=$(stat -c %a "$FIREWALLD_TEST_PARENT")
%{buildroot}%{_libexecdir}/piccolo/firewalld-policy.sh apply "$FIREWALLD_TEST_CONFIG"
%{buildroot}%{_libexecdir}/piccolo/firewalld-policy.sh check "$FIREWALLD_TEST_CONFIG"
test "$(stat -c %a "$FIREWALLD_TEST_PARENT")" = "$FIREWALLD_TEST_PARENT_MODE"
grep -q '^CleanupOnExit=yes$' "$FIREWALLD_TEST_CONFIG"
%{buildroot}%{_libexecdir}/piccolo/firewalld-policy.sh apply "$FIREWALLD_NEW_CONFIG"
%{buildroot}%{_libexecdir}/piccolo/firewalld-policy.sh check "$FIREWALLD_NEW_CONFIG"
test "$(stat -c %a "$(dirname "$FIREWALLD_NEW_CONFIG")")" = "750"
test "$(stat -c %a "$FIREWALLD_NEW_CONFIG")" = "644"

%post
# 1. Mask physical/serial consoles to prevent login
# Use --root=/ --no-reload to ensure this works in chroot/image-build environments
/usr/bin/systemctl --root=/ --no-reload mask serial-getty@ttyS0.service serial-getty@ttyS1.service serial-getty@ttyS2.service getty@tty1.service

# 2. Enable Firewalld Service explicitly
/usr/bin/systemctl --root=/ --no-reload enable firewalld

# 3. Mask sleep/hibernate targets (belt-and-suspenders with sleep.conf drop-in)
/usr/bin/systemctl --root=/ --no-reload mask \
    sleep.target suspend.target hibernate.target \
    hybrid-sleep.target suspend-then-hibernate.target

# 4. Enforce Firewall Zone
# /etc/firewalld/firewalld.conf is mutable appliance state, so package upgrades
# must migrate existing systems in addition to the KIWI image seed.
/usr/libexec/piccolo/firewalld-policy.sh apply /etc/firewalld/firewalld.conf

# 5. Enforce Snapper retention policy on fresh installs and upgrades.
# /etc/snapper/configs/root is mutable appliance state, so package upgrades
# must migrate existing systems in addition to the KIWI image seed.
/usr/libexec/piccolo/snapper-policy.sh apply /etc/snapper/configs/root

# 6. Enforce host bootstrap DNS on fresh installs and upgrades.
# Piccolo may itself provide LAN DNS through an app, so the host resolver must
# not depend on DHCP/router DNS that can point back at Piccolo during boot.
# KIWI fresh images are re-seeded in disk.sh after KIWI removes resolv.conf.
/usr/libexec/piccolo/bootstrap-dns.sh apply /etc/resolv.conf

# 7. Clean up removed net-watchdog on upgrade (files removed in 0.3.2).
#    disable (--root=/) is authoritative and works in chroot/transactional-update.
#    stop is best-effort — only takes effect on a live (non-chroot) system.
if [ $1 -ge 2 ]; then
    /usr/bin/systemctl --root=/ --no-reload disable piccolo-net-watchdog.timer 2>/dev/null || :
    /usr/bin/systemctl stop piccolo-net-watchdog.timer piccolo-net-watchdog.service 2>/dev/null || :
    # Note: /var/lib/piccolo/net-watchdog-reboots is intentionally NOT deleted here.
    # On MicroOS, /var is shared with the still-running old snapshot until reboot.
    # Clearing the reboot counter while the old watchdog timer runs could allow
    # an extra reboot during a network failure loop. The file is harmless and tiny;
    # piccolod cleans it up on first run.
fi

# 8. Explicitly enable health-checker for boot-time rollback.
# MicroOS preset should enable this, but we make it explicit since
# this service is a critical safety net.
/usr/bin/systemctl --root=/ --no-reload enable health-checker.service 2>/dev/null || :

# 9. Enable clock epoch service and periodic save timer
/usr/bin/systemctl --root=/ --no-reload enable piccolo-clock-epoch.service
/usr/bin/systemctl --root=/ --no-reload enable piccolo-clock-epoch-save.timer
# Seed epoch file so the first reboot after upgrade is protected.
# On fresh installs this overwrites the config.sh seed with a current timestamp.
# On upgrades, this creates the file for the first time (transactional-update
# chroot has /var bind-mounted and uses the running kernel's NTP-correct clock).
mkdir -p /var/lib/piccolo
date +%s > /var/lib/piccolo/clock-epoch 2>/dev/null || :

# 10. Rootless Podman prerequisites (RFC 20260206)
# Create piccolo-runtime user for rootless Podman execution.
if ! getent passwd piccolo-runtime > /dev/null 2>&1; then
    useradd --system --home-dir /home/piccolo-runtime --create-home \
        --shell /usr/sbin/nologin --user-group piccolo-runtime
fi
# Allocate subordinate UID/GID ranges for rootless user namespaces.
# Guard: usermod --add-subuids is not idempotent — it appends on every call.
if ! grep -q '^piccolo-runtime:' /etc/subuid 2>/dev/null || ! grep -q '^piccolo-runtime:' /etc/subgid 2>/dev/null; then
    usermod --add-subuids 100000-165535 --add-subgids 100000-165535 piccolo-runtime
fi
# Enable linger for cgroup delegation (resource limits require this).
# Cannot use 'loginctl enable-linger' in chroot — use file-based equivalent.
mkdir -p /var/lib/systemd/linger
touch /var/lib/systemd/linger/piccolo-runtime

# 11. Enable watchdog driver validation at boot
/usr/bin/systemctl --root=/ --no-reload enable piccolo-watchdog-check.service

# 12. Prefer cold reboot semantics on x86 appliances.
# Reboots are rare for Piccolo OS devices; reliability matters more than the
# speed difference between warm and cold-style firmware restart paths.
piccolo_regenerate_sdboot_entries() {
    case "$(uname -m)" in
        x86_64) sdboot_arch=x64 ;;
        i?86) sdboot_arch=ia32 ;;
        *) sdboot_arch= ;;
    esac

    if [ -n "${sdboot_arch:-}" ] && [ -d /boot/efi ]; then
        export hostonly_l=no
        /usr/bin/sdbootutil -v --arch "$sdboot_arch" --esp-path /boot/efi --entry-token=auto add-all-kernels || \
            echo "piccolo-os-support: WARNING failed to regenerate boot entries after adding reboot=cold" >&2
    else
        echo "piccolo-os-support: WARNING reboot=cold is configured but sdbootutil or /boot/efi is unavailable" >&2
    fi
}

piccolo_regenerate_grub_config() {
    if [ -x /usr/sbin/grub2-mkconfig ] && [ -d /boot/grub2 ]; then
        /usr/sbin/grub2-mkconfig -o /boot/grub2/grub.cfg || \
            echo "piccolo-os-support: WARNING failed to regenerate GRUB config after adding reboot=cold" >&2
    elif [ -x /usr/sbin/update-bootloader ]; then
        /usr/sbin/update-bootloader --reinit || \
            echo "piccolo-os-support: WARNING failed to reinitialize bootloader after adding reboot=cold" >&2
    else
        echo "piccolo-os-support: WARNING reboot=cold is configured but no GRUB regeneration command is available" >&2
    fi
}

case "$(uname -m)" in
    x86_64|i?86)
        if [ -x /usr/bin/sdbootutil ] && [ -e /etc/kernel/cmdline ]; then
            if grep -Eq '(^|[[:space:]])reboot=cold($|[[:space:]])' /etc/kernel/cmdline; then
                piccolo_regenerate_sdboot_entries
            elif grep -Eq '(^|[[:space:]])reboot=' /etc/kernel/cmdline; then
                echo "piccolo-os-support: leaving existing reboot= kernel option unchanged" >&2
            else
                sed -i 's/$/ reboot=cold/' /etc/kernel/cmdline
                piccolo_regenerate_sdboot_entries
            fi
        elif [ -e /etc/default/grub ]; then
            if grep -Eq '^GRUB_CMDLINE_LINUX_DEFAULT=.*reboot=cold' /etc/default/grub; then
                piccolo_regenerate_grub_config
            elif grep -Eq '^GRUB_CMDLINE_LINUX_DEFAULT=.*reboot=' /etc/default/grub; then
                echo "piccolo-os-support: leaving existing reboot= kernel option unchanged" >&2
            elif grep -Eq '^GRUB_CMDLINE_LINUX_DEFAULT=".*"$' /etc/default/grub; then
                sed -i '/^GRUB_CMDLINE_LINUX_DEFAULT=/ s/"$/ reboot=cold"/' /etc/default/grub
                piccolo_regenerate_grub_config
            else
                echo "piccolo-os-support: WARNING could not update GRUB_CMDLINE_LINUX_DEFAULT format" >&2
            fi
        else
            echo "piccolo-os-support: no supported kernel command-line source found; skipping reboot=cold default" >&2
        fi
        ;;
esac

%preun
if [ $1 -eq 0 ]; then
    # Unmask consoles (from %post)
    /usr/bin/systemctl --root=/ unmask \
        serial-getty@ttyS0.service serial-getty@ttyS1.service \
        serial-getty@ttyS2.service getty@tty1.service 2>/dev/null || :
    # Unmask sleep targets (from %post)
    /usr/bin/systemctl --root=/ unmask \
        sleep.target suspend.target hibernate.target \
        hybrid-sleep.target suspend-then-hibernate.target 2>/dev/null || :
    # Note: health-checker.service is intentionally NOT disabled on uninstall.
    # MicroOS presets enable it by default; our %post enable is belt-and-suspenders.
    # Disable clock epoch service and timer (from %post)
    /usr/bin/systemctl --root=/ --no-reload disable piccolo-clock-epoch.service 2>/dev/null || :
    /usr/bin/systemctl --root=/ --no-reload disable piccolo-clock-epoch-save.timer 2>/dev/null || :
    /usr/bin/systemctl stop piccolo-clock-epoch.service piccolo-clock-epoch-save.timer piccolo-clock-epoch-save.service 2>/dev/null || :
    # Disable watchdog check service (from %post)
    /usr/bin/systemctl --root=/ --no-reload disable piccolo-watchdog-check.service 2>/dev/null || :
    /usr/bin/systemctl stop piccolo-watchdog-check.service 2>/dev/null || :
    # Remove rootless Podman prerequisites (from %post)
    rm -f /var/lib/systemd/linger/piccolo-runtime
    sed -i '/^piccolo-runtime:/d' /etc/subuid 2>/dev/null || :
    sed -i '/^piccolo-runtime:/d' /etc/subgid 2>/dev/null || :
    # Note: piccolo-runtime user is intentionally NOT deleted on uninstall.
    # Removing it can orphan ownership on rootless container storage.
fi

%files
# Own the specific key file
%dir %{_sysconfdir}/pki
%dir %{_sysconfdir}/pki/rpm-gpg
%config(noreplace) %{_sysconfdir}/pki/rpm-gpg/RPM-GPG-KEY-piccolo-os
%dir %{_prefix}/lib/firewalld
%dir %{_prefix}/lib/firewalld/zones
%{_prefix}/lib/firewalld/zones/piccolo.xml
%dir %{_sysconfdir}/zypp
%dir %{_sysconfdir}/zypp/repos.d
%config(noreplace) %{_sysconfdir}/zypp/repos.d/piccolo-os.repo
%config(noreplace) %{_sysconfdir}/transactional-update.conf
%dir %{_libexecdir}/health-checker
%{_libexecdir}/health-checker/piccolod.sh
%dir %{_prefix}/lib/systemd/system/health-checker.service.d
%{_prefix}/lib/systemd/system/health-checker.service.d/piccolo.conf
%dir %{_prefix}/lib/systemd/logind.conf.d
%{_prefix}/lib/systemd/logind.conf.d/piccolo.conf
%dir %{_prefix}/lib/systemd/sleep.conf.d
%{_prefix}/lib/systemd/sleep.conf.d/piccolo.conf
%dir %{_prefix}/lib/NetworkManager
%dir %{_prefix}/lib/NetworkManager/conf.d
%{_prefix}/lib/NetworkManager/conf.d/piccolo-wifi-powersave.conf
%{_prefix}/lib/NetworkManager/conf.d/piccolo-bootstrap-dns.conf
%dir %{_libexecdir}/piccolo
%{_libexecdir}/piccolo/bootstrap-dns.sh
%{_libexecdir}/piccolo/clock-epoch.sh
%{_prefix}/lib/systemd/system/piccolo-clock-epoch.service
%{_prefix}/lib/systemd/system/piccolo-clock-epoch-save.service
%{_prefix}/lib/systemd/system/piccolo-clock-epoch-save.timer
# %config (not noreplace): intentionally overwrite on upgrade to enforce security invariant.
# Operator-added locks via 'zypper al' will be saved as /etc/zypp/locks.rpmsave.
%config %{_sysconfdir}/zypp/locks
%dir %{_prefix}/lib/systemd/system.conf.d
%{_prefix}/lib/systemd/system.conf.d/piccolo.conf
%{_prefix}/lib/sysctl.d/90-piccolo-panic-reboot.conf
%{_prefix}/lib/modprobe.d/piccolo-watchdog.conf
%{_libexecdir}/piccolo/firewalld-policy.sh
%{_libexecdir}/piccolo/snapper-policy.sh
%{_libexecdir}/piccolo/watchdog-check.sh
%{_prefix}/lib/systemd/system/piccolo-watchdog-check.service
%dir /var/lib/piccolo

%changelog
* Thu Jun 25 2026 Piccolo Team <dev@piccolo.local> 0.3.10-0
- Log piccolod readiness status, curl failures, and bounded response bodies
  during boot health checks so rollback causes are diagnosable.

* Thu Jun 25 2026 Piccolo Team <dev@piccolo.local> 0.3.9-0
- Enforce static host resolver file for bootstrap DNS instead of relying on
  NetworkManager global DNS, which still left DHCP DNS in resolv.conf.

* Wed Jun 24 2026 Piccolo Team <dev@piccolo.local> 0.3.8-0
- Add host bootstrap DNS policy so Piccolo can recover when LAN DNS points at
  an app-hosted resolver that is not ready yet.

* Mon Jun 22 2026 Piccolo Team <dev@piccolo.local> 0.3.7-0
- Enforce firewalld default zone directly in image builds and upgrades.
- Declare Snapper as a build and scriptlet dependency for policy migration.

* Mon Jun 22 2026 Piccolo Team <dev@piccolo.local> 0.3.6-0
- Enforce appliance Snapper retention policy in image builds and upgrades.

* Tue Jun 09 2026 Piccolo Team <dev@piccolo.local> 0.3.5-0
- Restore unversioned piccolod dependency; piccolod has its own release stream.
- Correct 0.3.0 changelog weekday.

* Tue Jun 09 2026 Piccolo Team <dev@piccolo.local> 0.3.4-0
- Add guarded reboot=cold kernel command-line migration for x86 devices,
  covering both sdbootutil and GRUB installs.

* Wed Jun 03 2026 Piccolo Team <dev@piccolo.local> 0.3.3-0
- Ignore short power-key presses so accidental button taps do not power off
  Piccolo appliances.

* Tue Mar 31 2026 Piccolo Team <dev@piccolo.local> 0.3.2-0
- Remove piccolo-net-watchdog (script, service, timer). Network health
  monitoring migrates into piccolod's connectivity state machine for
  unified coordination with WiFi/AP mode transitions.

* Sun Mar 22 2026 Piccolo Team <dev@piccolo.local> 0.3.1-0
- Blacklist intel_oc_wdt and softdog kernel modules to ensure the reliable
  iTCO_wdt claims /dev/watchdog0 on Intel platforms. No-op on AMD/ARM.
- Add piccolo-watchdog-check.service: boot-time oneshot that logs which
  watchdog driver owns watchdog0 for fleet-wide observability.

* Tue Mar 17 2026 Piccolo Team <dev@piccolo.local> 0.3.0-0
- Switch health-checker plugin from /health/live to /health/ready endpoint.
  Enables boot-time rollback on fatal component errors (503 on LevelError).
- Explicitly enable health-checker.service in %%post as belt-and-suspenders
  for MicroOS presets.

* Wed Mar 11 2026 Piccolo Team <dev@piccolo.local> 0.2.9-0
- Move flattened MicroOS pattern packages from kiwi to spec Requires.
  Pins all packages during transactional-update dup, preventing provider
  swaps (e.g., systemd-presets-branding-Aeon replacing -MicroOS).

* Tue Mar 10 2026 Piccolo Team <dev@piccolo.local> 0.2.8-0
- Add hardware watchdog (RuntimeWatchdogSec=30): auto-reboot on kernel freeze
  via platform-agnostic /dev/watchdog0. Silently ignored on platforms without
  a hardware watchdog.
- Add kernel panic auto-reboot (kernel.panic=10, kernel.panic_on_oops=1):
  reboot 10s after panic instead of halting on a headless appliance.

* Thu Mar 05 2026 Piccolo Team <dev@piccolo.local> 0.2.7-0
- Remove obsolete piccolo-apps group setup and fuse.conf user_allow_other edits.
- Keep rootless runtime prerequisites: piccolo-runtime user, subuid/subgid
  allocation, and linger setup.

* Fri Feb 20 2026 Piccolo Team <dev@piccolo.local> 0.2.6-0
- Add piccolo-apps group and piccolo-runtime system user for rootless
  Podman execution. Configure subordinate UID/GID ranges, loginctl
  linger, and user_allow_other in fuse.conf.

* Mon Feb 16 2026 Piccolo Team <dev@piccolo.local> 0.2.5-0
- Add zypp package locks to prevent transactional-update from reinstalling
  removed packages (openssh, openssh-server, openssh-clients, rebootmgr).

* Sun Feb 15 2026 Piccolo Team <dev@piccolo.local> 0.2.4-0
- Add clock epoch service: persist/restore system clock for RTC-less devices
  to prevent large NTP time steps that trigger Persistent=true timers.

* Sat Feb 14 2026 Piccolo Team <dev@piccolo.local> 0.2.3-0
- Add network health watchdog: ARP-based gateway detection with interface bounce
  and reboot escalation for unrecoverable network failures.

* Wed Feb 11 2026 Piccolo Team <dev@piccolo.local> 0.2.2-0
- Add always-on power policy: ignore lid close, mask sleep/hibernate targets.
- Disable WiFi power saving for reliable LAN reachability.

* Mon Dec 15 2025 Piccolo Team <dev@piccolo.local> 0.2.0-8
- Consolidate repositories into a single unified repo (home:atdexterslab:atdexterslab_tumbleweed).
- Removed architecture-specific repo URL logic.
- Removed ExclusiveArch restriction.
- Switched to BuildArch: noarch.
- Removed unused rpmlintrc filter (no-binary).

* Mon Dec 15 2025 Piccolo Team <dev@piccolo.local> 0.2.0-7
- Added health-checker plugin to verify piccolod availability (/api/v1/health/live).
- Added Requires: health-checker and curl.
- Implemented automatic rollback support via health-checker service.
- Added systemd drop-in to order health-checker.service After=piccolod.service.
- Added retry logic to health check script (30s timeout) to handle slow startups.

* Thu Dec 11 2025 Piccolo Team <dev@piccolo.local> 0.2.0-6
- Added /etc/transactional-update.conf with REBOOT_METHOD=none to disable automatic reboots.

* Tue Dec 09 2025 Piccolo Team <dev@piccolo.local> 0.2.0-2
- Switched to file-based GPG trust (canonical method).
- Key is now installed to /etc/pki/rpm-gpg/RPM-GPG-KEY-piccolo-os.
- Repo file now uses gpgkey=file://... to avoid interactive trust prompts.
- Removed %%post rpm --import to prevent database locking issues in transactional updates.
- Removed placeholder README now that package ships actual config files.

* Tue Dec 09 2025 Piccolo Team <dev@piccolo.local> 0.2.0-1
- Import repository GPG key in post script to avoid interactive trust prompts.

* Fri Dec 05 2025 Piccolo Team <dev@piccolo.local> 0.2.0-0
- Added repository configuration (/etc/zypp/repos.d/piccolo-os.repo)
- Removed BuildArch: noarch to support architecture-specific repo URLs
- Updated to support OBS build workflow

* Wed Nov 19 2025 Piccolo Team <dev@piccolo.local> 0.1.0-0
- Initial package; enforce piccolod presence on Piccolo OS images.
