Name:           piccolo-os-support
Version:        0.2.0
Release:        0
Summary:        Piccolo OS policy/meta package
License:        AGPL-3.0-or-later
URL:            https://github.com/AtDexters-Lab/piccolo-os
ExclusiveArch:  x86_64 aarch64
Source0:        piccolo.xml
Source1:        piccolo-os-support-rpmlintrc

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
install -d %{buildroot}%{_docdir}/%{name}
cat > %{buildroot}%{_docdir}/%{name}/README <<'EOT'
This placeholder README ships with piccolo-os-support so the RPM owns at least one
file. The package's primary responsibility today is to depend on piccolod; future
updates will drop docs that describe transactional-update hooks and on-device health
checks.
EOT

# Install Firewalld Zone
install -D -m 644 %{SOURCE0} %{buildroot}%{_prefix}/lib/firewalld/zones/piccolo.xml

# Install Repo File
install -d -m 755 %{buildroot}/etc/zypp/repos.d
%ifarch x86_64
REPO_URL="https://download.opensuse.org/repositories/home:/abhishekborar93:/piccolo-os/openSUSE_Tumbleweed/"
%endif
%ifarch aarch64
REPO_URL="https://download.opensuse.org/repositories/home:/abhishekborar93:/piccolo-os/openSUSE_Factory_ARM/"
%endif

cat > %{buildroot}/etc/zypp/repos.d/piccolo-os.repo <<EOF
[piccolo-os]
name=Piccolo OS
enabled=1
autorefresh=1
baseurl=$REPO_URL
type=rpm-md
gpgcheck=1
gpgkey=${REPO_URL}repodata/repomd.xml.key
EOF

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
%dir %{_docdir}/%{name}
%doc %{_docdir}/%{name}/README
%dir %{_prefix}/lib/firewalld
%dir %{_prefix}/lib/firewalld/zones
%{_prefix}/lib/firewalld/zones/piccolo.xml
%dir /etc/zypp
%dir /etc/zypp/repos.d
%config(noreplace) /etc/zypp/repos.d/piccolo-os.repo

%changelog
* Fri Dec 05 2025 Piccolo Team <dev@piccolo.local> 0.2.0-0
- Added repository configuration (/etc/zypp/repos.d/piccolo-os.repo)
- Removed BuildArch: noarch to support architecture-specific repo URLs
- Updated to support OBS build workflow

* Wed Nov 19 2025 Piccolo Team <dev@piccolo.local> 0.1.0-0
- Initial package; enforce piccolod presence on Piccolo OS images.
