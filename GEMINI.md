# Piccolo OS Context & Guidelines

Piccolo OS is a privacy-first, headless operating system for homelabs, built on openSUSE MicroOS. It features an immutable root, transactional updates, and a container-native architecture managed by the `piccolod` control plane.

## 1. Project Structure

- **`kiwi/microos-ots/`**: The OS image definition.
    - `os.kiwi`: Main declarative image config (packages, users, bootloader).
    - `config.sh`: Post-image-creation configuration script (runs in chroot).
    - `editbootinstall_*.sh`: Architecture-specific bootloader hooks.
- **`packages/`**: Custom RPM definitions.
    - `piccolo-os-support/`: Metapackage for OS policy, firewall zones, and hardening logic.
- **`scripts/`**: Operational tooling.
    - `run-native.sh`: Orchestrates `kiwi-ng` builds.
    - `start-vm.sh`: Helpers for booting artifacts in VirtualBox.
    - `tw_mirror.sh`: Local repo mirroring tool.
- **`releases/`**: Git-ignored directory for build artifacts.

## 2. Architecture & Security Hardening

The OS is designed as a locked-down appliance:
- **Root Access:** Locked by default. No password-based login.
- **SSH:** Removed. `openssh` packages are explicitly deleted.
- **Console:** Kernel consoles are silenced (`console=` in `config.sh`) and `getty` services are masked via `piccolo-os-support` to prevent login prompts.
- **Networking:**
    - **Firewall:** `firewalld` is enabled.
    - **Policy:** Default zone is `piccolo` (Drop All).
    - **Allow List:** Only LAN (RFC1918) traffic to ports 80 (Portal), 5353 (mDNS), and 35000-45000 (Proxy range) is permitted.
    - **Implementation:** The `piccolo-os-support` RPM installs the zone file and enforces this policy in `%post`.

## 3. Build & Development Commands

- **Build Image:**
  ```bash
  ./scripts/run-native.sh Standard microos-ots
  ```
  Artifacts appear in `releases/microos-ots/`.

- **Test in VM:**
  ```bash
  ./scripts/start-vm.sh releases/microos-ots/<version>/disk.vdi
  ```
  Requires VirtualBox. Creates/clones a `piccolo-template` VM.

- **Mirror Repos:**
  ```bash
  ./scripts/tw_mirror.sh --dest /var/mirrors/tw
  ```

## 4. Coding Conventions

- **Shell Scripts:** `#!/usr/bin/env bash`, `set -euo pipefail`, 2-space indent, UPPERCASE variables.
- **KIWI XML:** Declarative, camelCase attributes, snake_case elements.
- **RPM Specs:**
    - Use `BuildRequires` for tools needed in `%check` or `%post` validation.
    - Use `%check` sections to validate configuration files (e.g., `xmllint`).
    - Use `systemctl --root=/ --no-reload` for systemd operations in `%post`.

## 5. Commit & PR Guidelines

- **Format:** Conventional Commits (e.g., `feat:`, `fix:`, `docs:`, `chore:`).
    - Example: `feat(security): enforce piccolo firewall zone`
- **Scope:** Keep changes focused. Separate refactors from features.
- **Validation:** PRs must include manual test evidence (e.g., "Booted VDI, verified port 80 accessible, SSH connection refused").

## 6. Testing Strategy

Since there is no automated CI yet, every change requires manual validation:
1.  **Build:** Run the build script. Ensure no RPM install errors (especially in `%post`).
2.  **Boot:** Start the VM.
3.  **Verify:**
    - **Console:** Is it quiet (no logs/login prompt)?
    - **Reachability:** Is `http://piccolo.local` reachable?
    - **Security:** Scan with `nmap` to verify firewall policy:
      ```bash
      nmap -p- piccolo.local
      ```
      *Expected:* Only ports 80 (HTTP) and potentially 5353 (mDNS) should be open. SSH (22) should be closed/filtered.

## 7. Critical Maintenance

- **GPG Key Expiry:**
    - The repository signing key (`packages/piccolo-os-support/piccolo-os.key`) expires on **2028-01-23**.
    - **Action:** If the current date is within 3 months of this date, YOU MUST initiate the "Rotate & Overlap" SOP defined in `piccolo-os-support.spec`.
    - **Check:** Periodically verify with `gpg --show-keys packages/piccolo-os-support/piccolo-os.key`.

