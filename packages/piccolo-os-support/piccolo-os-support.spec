Name:           piccolo-os-support
Version:        0.2.0
Release:        6
Summary:        Piccolo OS policy/meta package
License:        AGPL-3.0-or-later
URL:            https://github.com/AtDexters-Lab/piccolo-os
ExclusiveArch:  x86_64 aarch64
Source0:        piccolo.xml
Source1:        piccolo-os-support-rpmlintrc
Source2:        piccolo-os.key

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
Requires:       piccolod
Requires:       firewalld
Requires:       zypper
Requires(post): systemd

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

# Define URLs based on architecture
%ifarch x86_64
REPO_URL="https://download.opensuse.org/repositories/home:/abhishekborar93:/piccolo-os/openSUSE_Tumbleweed/"
%endif
%ifarch aarch64
REPO_URL="https://download.opensuse.org/repositories/home:/abhishekborar93:/piccolo-os/openSUSE_Factory_ARM/"
%endif

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

%check
# Validate the firewall zone XML
xmllint --noout %{buildroot}%{_prefix}/lib/firewalld/zones/piccolo.xml

%post
# 1. Mask physical/serial consoles to prevent login
# Use --root=/ --no-reload to ensure this works in chroot/image-build environments
/usr/bin/systemctl --root=/ --no-reload mask serial-getty@ttyS0.service serial-getty@ttyS1.service serial-getty@ttyS2.service getty@tty1.service

# 2. Enable Firewalld Service explicitly
/usr/bin/systemctl --root=/ --no-reload enable firewalld

# 3. Enforce Firewall Zone
# We use firewall-offline-cmd because this script often runs during image build
# (chroot) where firewalld daemon is not running.
if [ -x /usr/bin/firewall-offline-cmd ]; then
    CURRENT_ZONE=$(firewall-offline-cmd --get-default-zone)
    if [ "$CURRENT_ZONE" != "piccolo" ]; then
        firewall-offline-cmd --set-default-zone=piccolo
    fi
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

%changelog
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