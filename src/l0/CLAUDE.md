# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Layer 0 (l0) contains the openSUSE MicroOS-based OS build system for Piccolo OS. This layer uses KIWI NG to create bootable ISO images that include the piccolod daemon integrated as a systemd service. The system has evolved from Flatcar Linux to MicroOS for better UEFI support and container-native architecture.

## Development Commands

### Complete OS Build
Build the entire Piccolo OS including ISO and update packages:
```bash
./build.sh
```

This script:
1. Builds the piccolod binary from `../l1/piccolod`
2. Creates a custom MicroOS image with piccolod integrated using KIWI NG
3. Generates bootable ISO with UEFI support
4. Copies the piccolod binary into the overlay filesystem

### Direct Build Script
For more control over the build process:
```bash
./build_piccolo.sh --version 1.0.0 --binary-path ../l1/piccolod/build/piccolod
```

### Testing the Built Image
Test the complete OS in a VM:
```bash
./test_piccolo_os_image.sh --build-dir ./dist --version 1.0.0
```

This automated test script (v3.1+):
- Detects and uses proper UEFI firmware (prioritizing secboot variants)
- Boots the ISO in QEMU with SSH port forwarding (2222)
- Configures cloud-init for SSH access and system setup
- Verifies piccolod binary installation and service status
- Tests HTTP API endpoints on port 80 (both health endpoints)
- Validates container runtime (Podman socket and functionality)
- Runs comprehensive ecosystem health checks via `/api/v1/ecosystem`
- Tests MicroOS rollback integration via `/api/v1/health/ready`
- Provides detailed pass/warn/fail analysis with actionable feedback

### Interactive Debugging
For manual testing and debugging:
```bash
./ssh_into_piccolo.sh --build-dir ./dist --version 1.0.0
```

### Managing Releases
List all preserved releases:
```bash
ls -la ./releases/
ls -lh ./releases/1.0.0/  # View specific version artifacts
```

Test a specific preserved release:
```bash
./test_piccolo_os_image.sh --build-dir ./releases/1.0.0 --version 1.0.0
qemu-system-x86_64 -enable-kvm -m 2048 -cpu host -machine q35,accel=kvm \
  -bios /usr/share/OVMF/OVMF_CODE.fd -cdrom ./releases/1.0.0/piccolo-os-x86_64-1.0.0.iso
```

### Using Preserved Artifacts for Updates
The preserved artifacts enable comprehensive update management:

**Package Analysis:**
```bash
# Compare package versions between releases
diff ./releases/1.0.0/piccolo-os-x86_64-1.0.0.packages ./releases/1.1.0/piccolo-os-x86_64-1.1.0.packages

# Extract package list for vulnerability scanning
cut -d'|' -f1,4 ./releases/1.0.0/piccolo-os-x86_64-1.0.0.packages

# Show all versions of a specific package across releases
for version in ./releases/*/; do
  v=$(basename "$version")
  echo "=== Version $v ==="
  grep "^kernel-default|" "$version"/*.packages || echo "Package not found"
done
```

**Security and Integrity:**
```bash
# Review file verification status
cat ./releases/1.0.0/piccolo-os-x86_64-1.0.0.verified

# Check build metadata
jq . ./releases/1.0.0/piccolo-os-x86_64-1.0.0.json

# Compare integrity status between versions
diff ./releases/1.0.0/piccolo-os-x86_64-1.0.0.verified ./releases/1.1.0/piccolo-os-x86_64-1.1.0.verified
```

## Architecture Details

### Build System Integration

The build process integrates piccolod into openSUSE MicroOS through KIWI NG:

1. **KIWI Configuration**: Uses `kiwi/config.xml` to define MicroOS base system with Tumbleweed repositories
2. **Container-Based Building**: Leverages openSUSE Tumbleweed container with KIWI NG for reproducible builds
3. **Overlay Filesystem**: Copies piccolod binary to `/usr/local/piccolo/v1/bin/` with symlink at `/usr/local/piccolo/current`
4. **Package Selection**: Includes MicroOS base patterns, Podman container runtime, and live ISO support

### Key Build Components

- **`build_piccolo.sh`**: Main KIWI NG build orchestration with container runtime detection
- **`build.sh`**: Simple wrapper that builds piccolod then calls build_piccolo.sh  
- **`build.Dockerfile`**: Container image definition with KIWI NG and MicroOS dependencies
- **`test_piccolo_os_image.sh`**: Comprehensive automated testing framework (v3.1+) with UEFI and health check validation
- **`ssh_into_piccolo.sh`**: Interactive debugging tool for manual testing (v2.1+) with UEFI support
- **`lib/piccolo_common.sh`**: Shared utilities for VM management, UEFI detection, and cloud-init
- **`kiwi/config.xml`**: KIWI NG system configuration with MicroOS packages and cloud-init
- **`kiwi/root/`**: Overlay filesystem for custom files and systemd services
- **`piccolo.env`**: Build configuration (GPG keys, update server URLs)

### Generated Artifacts

**Current Build Outputs** (in `./dist/`):
- `piccolo-os-{arch}-{version}.iso` - Bootable live ISO with UEFI support (overwritten each build)
- `kiwi.log` - Current build log for troubleshooting
- KIWI NG temporary files during build process

**Preserved Releases** (in `./releases/{version}/`):
Each version is stored in its own subdirectory for better organization:
```
./releases/
├── 1.0.0/
│   ├── piccolo-os-x86_64-1.0.0.iso     - Bootable ISO
│   ├── piccolo-os-x86_64-1.0.0.log     - Build log
│   ├── piccolo-os-x86_64-1.0.0.packages - Package manifest (critical for updates)
│   ├── piccolo-os-x86_64-1.0.0.changes  - Package changelog and history
│   ├── piccolo-os-x86_64-1.0.0.verified - File integrity verification
│   └── piccolo-os-x86_64-1.0.0.json     - Build metadata
├── 1.1.0/
│   └── ...
└── 1.2.0/
    └── ...
```
All previous builds are automatically preserved and listed after each build

### Systemd Service Configuration

The piccolod.service is defined in `kiwi/root/etc/systemd/system/` and configured for:
- **Type=notify**: Proper systemd integration with `sd_notify()` for health monitoring
- **Security**: `ProtectSystem=strict` with specific `ReadWritePaths=/run /var /tmp /etc`
- **Capabilities**: `CAP_NET_BIND_SERVICE` for binding to port 80
- **Installation**: Binary at `/usr/local/piccolo/v1/bin/piccolod`
- **Failure Handling**: `OnFailure=piccolod-failure-handler.service` for MicroOS rollback
- **Container Integration**: Podman socket enabled via systemd preset

### MicroOS Rollback Integration

Critical for OS update safety with automatic rollback capability:

**Health Check Services:**
- `piccolod-health-check.service` - Validates piccolod functionality after boot
- `piccolod-failure-handler.service` - Logs critical failures for rollback detection
- Both services enabled automatically via `80-piccolo.preset`

**Health Endpoints:**
- `/api/v1/health/ready` - Boolean health check (HTTP 200/503) for systemd integration
- `/api/v1/ecosystem` - Detailed diagnostics with comprehensive system validation

**Systemd Preset File** (`80-piccolo.preset`):
```
enable NetworkManager.service
enable sshd.service  
enable cloud-init.target
enable podman.socket
enable piccolod.service
enable piccolod-health-check.service
enable piccolod-failure-handler.service
```

### MicroOS Integration

The system leverages MicroOS features:
- **Immutable Base**: Read-only root filesystem with transactional updates
- **Container-Native**: Built-in Podman support for application containers
- **BTRFS Snapshots**: Automatic system snapshots for rollback capability
- **Atomic Updates**: System updates as complete filesystem replacements

## Dependencies

Required system dependencies:
- `git` - Repository operations
- `docker` or `podman` - Container runtime for KIWI NG builds
- `qemu-system-x86_64` - VM testing with UEFI support
- `ssh`, `ssh-keygen` - Remote testing and debugging
- `ss` - Network port checking for services
- Optional: `kiwi-ng` - For local builds without containers

## Configuration

### Build Environment (piccolo.env)
- `GPG_SIGNING_KEY_ID`: Key for signing release artifacts
- `PICCOLO_UPDATE_SERVER`: OTA update server URL
- `PICCOLO_UPDATE_GROUP`: Update channel (e.g., "piccolo-stable")

### Build Constants
- Architecture: `x86_64` (default, `aarch64` supported)
- Base OS: openSUSE Tumbleweed/MicroOS
- Container runtime: Podman
- HTTP port: `80` (requires CAP_NET_BIND_SERVICE capability)
- Binary path: `/usr/local/piccolo/v1/bin/piccolod`

## Testing Framework

The test suite validates:
1. **Binary Installation**: piccolod present and executable at correct path
2. **Service Status**: systemd service active and running with Type=notify
3. **API Functionality**: HTTP endpoints responding on port 80
   - `/api/v1/health/ready` - Boolean health check for systemd integration  
   - `/api/v1/ecosystem` - Detailed ecosystem validation and diagnostics
4. **Container Runtime**: Podman socket availability and container management
5. **Ecosystem Health**: Comprehensive self-validation with proper MicroOS paths
6. **System Integration**: MicroOS base system functionality and rollback readiness

### UEFI and Secure Boot Testing

The test framework automatically detects and uses proper UEFI firmware:

**UEFI Firmware Priority:**
1. `/usr/share/OVMF/OVMF_CODE_4M.secboot.fd` (preferred - secure boot capable)
2. `/usr/share/OVMF/OVMF_CODE.secboot.fd` (fallback secure boot)
3. `/usr/share/OVMF/OVMF_CODE_4M.fd` (standard UEFI)
4. `/usr/share/OVMF/OVMF_CODE.fd` (basic UEFI)

**Cloud-Init Integration:**
- Generates NoCloud seed ISO with CIDATA volume label
- Configures SSH keys and root password for test access
- Enables NetworkManager and SSH services via cloud-init
- Provides immediate SSH access for automated validation

Tests run in an isolated QEMU VM with SSH key injection and port forwarding (host:2222 → guest:22).

## Troubleshooting

### Common Issues and Solutions

**UEFI Boot Problems:**
- **Symptom**: System boots to UEFI Shell instead of MicroOS
- **Solution**: Ensure using secboot UEFI firmware (`OVMF_CODE_4M.secboot.fd`)
- **Test**: Boot with proper QEMU command including `-bios` parameter

**SSH Authentication Failures:**
- **Symptom**: SSH port open but authentication fails
- **Root Cause**: NetworkManager not enabled, missing SSH configuration
- **Solution**: Systemd preset file automatically enables required services
- **Manual Fix**: `systemctl enable NetworkManager` in live system

**Health Check Failures:**
- **Symptom**: piccolod reports "unhealthy" status
- **Common Causes**: 
  - Filesystem access blocked by `ProtectSystem=strict`
  - Podman socket not available (`podman.socket` disabled)
  - Wrong filesystem paths for MicroOS (`/var/run` vs `/run`)
- **Solution**: Systemd unit configured with proper `ReadWritePaths` and Podman socket enabled

**Service Enablement Issues:**
- **Symptom**: Services show "disabled; preset: enabled" or "static"
- **Root Cause**: Missing `[Install]` sections or wrong `WantedBy` targets
- **Solution**: Use `multi-user.target` instead of non-existent targets like `health-check.target`

### Health Check Validation

Verify system health in live VM:
```bash
# Check all service statuses
systemctl status piccolod piccolod-health-check piccolod-failure-handler podman.socket

# Test health endpoints directly
curl -s http://localhost/api/v1/health/ready | jq .
curl -s http://localhost/api/v1/ecosystem | jq .

# Validate Podman integration
podman version
ls -la /run/podman/podman.sock
```

## Migration Notes

This system has migrated from Flatcar Linux to openSUSE MicroOS. Key differences:
- Build system changed from Gentoo/Portage to KIWI NG
- Container runtime changed from Docker to Podman
- Update mechanism uses MicroOS transactional updates instead of CoreOS update_engine
- Research and migration documentation available in `foundation.md` and `coreos-migration-research.md`