Name:           piccolo-os-support
Version:        0.1.0
Release:        0
Summary:        Piccolo OS policy/meta package
License:        AGPL-3.0-or-later
URL:            https://github.com/AtDexters-Lab/piccolo-os
BuildArch:      noarch
Source0:        piccolo.xml

BuildRequires:  systemd
BuildRequires:  firewalld
BuildRequires:  libxml2-tools
Requires:       piccolod
Requires:       firewalld
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

%changelog
* Wed Nov 19 2025 Piccolo Team <dev@piccolo.local> 0.1.0-0
- Initial package; enforce piccolod presence on Piccolo OS images.
