Name:           piccolo-os-support
Version:        0.1.0
Release:        0
Summary:        Piccolo OS policy/meta package
License:        AGPL-3.0-or-later
URL:            https://github.com/AtDexters-Lab/piccolo-os
BuildArch:      noarch

Requires:       piccolod

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

%files
%dir %{_docdir}/%{name}
%doc %{_docdir}/%{name}/README

%changelog
* Wed Nov 19 2025 Piccolo Team <dev@piccolo.local> 0.1.0-0
- Initial package; enforce piccolod presence on Piccolo OS images.
