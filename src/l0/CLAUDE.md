# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Layer 0 (l0) contains the openSUSE MicroOS-based OS build system for Piccolo OS. This layer uses KIWI NG to create USB-bootable disk images (OEM format) that include the piccolod daemon integrated as a systemd service. The system has evolved from Flatcar Linux to MicroOS and from ISO to OEM disk images for better UEFI, systemd-boot, and Secure Boot support.

## Development Commands

### Complete OS Build
Build the entire Piccolo OS including USB-bootable disk image:
```bash
# Development build (default - with SSH and cloud-init for testing)
./build.sh dev

# Production build (hardened - zero access, API-only)
./build.sh prod
```

This script:
1. Builds the piccolod binary from `../l1/piccolod`
2. Creates a custom MicroOS image with piccolod integrated using KIWI NG
3. Generates USB-bootable disk image (.raw) with UEFI and systemd-boot support
4. Uses additive configuration: production base + development additions for dev variant
5. Automatically tests development builds with cloud-init integration

### Direct Build Script
For more control over the build process with variant selection:
```bash
# Development build (default - includes cloud-init and SSH)
./build_piccolo.sh --binary-path ../l1/piccolod/build/piccolod --variant dev --version 1.0.0

# Production build (hardened - no SSH, no cloud-init)
./build_piccolo.sh --binary-path ../l1/piccolod/build/piccolod --variant prod --version 1.0.0
```

### Testing the Built Image
Test the complete OS in a VM:
```bash
# Test from current build artifacts
./test_piccolo_os_image.sh --build-dir ./releases/1.0.0 --version 1.0.0

# Development builds are automatically tested after build
# Production builds require manual verification (no SSH access)
```

This automated test script (v4.0+):
- Automatically detects dev or prod variant disk images (.raw)
- Detects and uses proper UEFI firmware (prioritizing secboot variants) 
- Boots disk images in QEMU with cloud-init seed ISO injection
- SSH authentication via cloud-init configured keys and passwords
- Verifies piccolod binary installation and service status
- Tests HTTP API endpoints on port 80 (health and ecosystem endpoints)
- Validates container runtime (Podman socket and functionality)
- Runs comprehensive ecosystem health checks via `/api/v1/ecosystem`
- Tests MicroOS rollback integration via `/api/v1/health/ready`
- Provides detailed pass/warn/fail analysis with actionable feedback

### Interactive Debugging
For manual testing and debugging:
```bash
./ssh_into_piccolo.sh --build-dir ./releases/1.0.0 --version 1.0.0
```

Note: Only works with development variant images (cloud-init + SSH enabled)

### Managing Releases
List all preserved releases:
```bash
ls -la ./releases/
ls -lh ./releases/1.0.0/  # View specific version artifacts
```

Test a specific preserved release:
```bash
./test_piccolo_os_image.sh --build-dir ./releases/1.0.0 --version 1.0.0

# Manual QEMU testing (automatically detects dev or prod variant disk images)
qemu-system-x86_64 -enable-kvm -m 4096 -cpu host -machine q35,accel=kvm \
  -bios /usr/share/OVMF/OVMF_CODE.fd -drive file=./releases/1.0.0/piccolo-os-dev.x86_64-1.0.0.raw,format=raw
```

### Using Preserved Artifacts for Updates
The preserved artifacts enable comprehensive update management:

**Package Analysis:**
```bash
# Compare package versions between releases (dev and prod variants)
diff ./releases/1.0.0/piccolo-os-dev.x86_64-1.0.0.packages ./releases/1.1.0/piccolo-os-dev.x86_64-1.1.0.packages
diff ./releases/1.0.0/piccolo-os-prod.x86_64-1.0.0.packages ./releases/1.1.0/piccolo-os-prod.x86_64-1.1.0.packages

# Extract package list for vulnerability scanning
cut -d'|' -f1,4 ./releases/1.0.0/piccolo-os-dev.x86_64-1.0.0.packages

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

The build process integrates piccolod into openSUSE MicroOS through KIWI NG with a dual variant system:

1. **Production Base**: `kiwi/prod/` provides hardened, zero-access base configuration
2. **Development Additions**: `kiwi/dev/` adds SSH, cloud-init, and testing capabilities  
3. **Additive Architecture**: Development builds copy production base + apply dev additions
4. **Dynamic Configuration**: Generated dev config in `.work/kiwi-dev-generated/` with proper image naming
5. **Container-Based Building**: Uses persistent Docker containers with cached packages for fast rebuilds
6. **Comprehensive Cleanup**: Full dist/ removal and version-specific artifact management
7. **Service Management**: Proper systemd masking/unmasking and cloud-init marker handling

### Key Build Components

- **`build_piccolo.sh`**: Main KIWI NG build orchestration with container runtime detection
- **`build.sh`**: Simple wrapper that builds piccolod then calls build_piccolo.sh  
- **`build.Dockerfile`**: Container image definition with KIWI NG and MicroOS dependencies
- **`test_piccolo_os_image.sh`**: Comprehensive automated testing framework (v4.0+) with disk image support and cloud-init integration
- **`ssh_into_piccolo.sh`**: Interactive debugging tool (v2.2+) with disk image support and UEFI
- **`lib/piccolo_common.sh`**: Shared utilities for VM management, UEFI detection, and cloud-init
- **`kiwi/prod/`**: Production base configuration (hardened, zero-access)
  - `config.xml`: MicroOS + systemd-boot + minimal packages
  - `config.sh`: Security hardening and service masking
  - `root/`: Production overlay files and systemd services
- **`kiwi/dev/`**: Development additions applied on top of production
  - `packages.xml`: SSH + cloud-init packages
  - `services.sh`: Enable SSH/cloud-init, remove disable markers
  - `root/`: Development overlay files
- **`kiwi/root/`**: Overlay filesystem for custom files and systemd services
- **`piccolo.env`**: Build configuration (GPG keys, update server URLs)

### Generated Artifacts

**Current Build Outputs** (in `./dist/`):
- `piccolo-os-{variant}.{arch}-{version}.raw` - USB-bootable disk image with systemd-boot and UEFI support
- `kiwi.log` - Current build log for troubleshooting
- `kiwi.result.json` - Build metadata and artifact information
- KIWI NG temporary files during build process

Note: Entire `dist/` directory is cleaned before each build for consistency

**Preserved Releases** (in `./releases/{version}/`):
Each version is stored in its own subdirectory for better organization:
```
./releases/
├── 1.0.0/
│   ├── piccolo-os-dev.x86_64-1.0.0.raw     - Development disk image (cloud-init enabled)
│   ├── piccolo-os-prod.x86_64-1.0.0.raw    - Production disk image (hardened, zero-access)
│   ├── piccolo-os-dev.x86_64-1.0.0.log     - Development build log
│   ├── piccolo-os-prod.x86_64-1.0.0.log    - Production build log
│   ├── piccolo-os-dev.x86_64-1.0.0.packages - Development package manifest
│   ├── piccolo-os-prod.x86_64-1.0.0.packages - Production package manifest
│   ├── piccolo-os-dev.x86_64-1.0.0.changes  - Development changelog
│   ├── piccolo-os-prod.x86_64-1.0.0.changes - Production changelog
│   ├── piccolo-os-dev.x86_64-1.0.0.verified - Development integrity verification
│   ├── piccolo-os-prod.x86_64-1.0.0.verified - Production integrity verification  
│   ├── piccolo-os-dev.x86_64-1.0.0.json     - Development build metadata
│   └── piccolo-os-prod.x86_64-1.0.0.json    - Production build metadata
├── 1.1.0/
│   └── ...
└── 1.2.0/
    └── ...
```
All previous builds are automatically preserved and listed after each build

### Dual Variant System

**Production Variant** (`piccolo-os-prod.*.raw`):
- **Zero-Access Security**: No SSH, no cloud-init, no interactive console access
- **API-Only Access**: Communication only via piccolod REST API on port 80
- **Hardened Configuration**: Services masked, minimal logging, kernel hardening
- **Use Cases**: Production deployments, untrusted environments, appliance mode

**Development Variant** (`piccolo-os-dev.*.raw`):
- **Full Access**: SSH with key/password auth, cloud-init for automation
- **Testing Integration**: NoCloud datasource, automated provisioning
- **Interactive Access**: Serial console, interactive login, standard logging
- **Use Cases**: Development, testing, debugging, CI/CD pipelines

**Key Differences**:
| Feature | Production | Development |
|---------|------------|-------------|
| SSH Access | ❌ Disabled & Masked | ✅ Enabled |
| Cloud-init | ❌ Disabled & Marked | ✅ Enabled |
| Interactive Login | ❌ Masked | ✅ Available |
| Logging Level | Minimal (warnings+) | Standard (info+) |
| Serial Console | ❌ Disabled | ✅ Available |
| Authentication | API-only | SSH keys + passwords |

### Systemd Service Configuration

The piccolod.service is defined in `kiwi/prod/root/etc/systemd/system/` and configured for:
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

This system has undergone major architectural evolution:

**Platform Migration (Flatcar → MicroOS)**:
- Build system: Gentoo/Portage → KIWI NG
- Container runtime: Docker → Podman  
- Update mechanism: CoreOS update_engine → MicroOS transactional updates

**Image Format Migration (ISO → OEM Disk Images)**:
- Format: ISO images (.iso) → USB-bootable disk images (.raw)
- Bootloader: GRUB → systemd-boot for modern UEFI compatibility
- Boot method: El Torito CD/DVD boot → direct USB/disk boot with GPT
- Constraints: 31MB El Torito limit → 1GB ESP with full systemd-boot support

**Architecture Enhancements**:
- **Dual Variant System**: Production (zero-access) + Development (cloud-init)
- **Additive Configuration**: Production base + development additions
- **Simplified Codebase**: Removed all ISO backward compatibility (~100+ lines)
- **Modern UEFI Support**: Secure Boot ready with shim + systemd-boot
- **Cloud-init Integration**: Full NoCloud datasource support for automated testing

**Migration Documentation**: `foundation.md` and `coreos-migration-research.md`