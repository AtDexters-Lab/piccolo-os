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

This automated test script (v2.8):
- Boots the ISO in QEMU with SSH port forwarding (2222)
- Verifies piccolod binary installation and service status
- Tests HTTP API endpoints on port 8080
- Validates container runtime (Podman) functionality
- Runs comprehensive ecosystem health checks via `/api/v1/ecosystem`
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
- **`test_piccolo_os_image.sh`**: Comprehensive automated testing framework (v2.8)
- **`ssh_into_piccolo.sh`**: Interactive debugging tool for manual testing
- **`kiwi/config.xml`**: KIWI NG system configuration with MicroOS packages
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
- Installation to `/usr/local/piccolo/v1/bin/piccolod` 
- Automatic startup via symlink in `multi-user.target.wants/`
- Integration with MicroOS's systemd-based architecture
- Container runtime integration with Podman

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
- HTTP port: `8080`
- Binary path: `/usr/local/piccolo/v1/bin/piccolod`

## Testing Framework

The test suite validates:
1. **Binary Installation**: piccolod present and executable at correct path
2. **Service Status**: systemd service active and running 
3. **API Functionality**: HTTP endpoints responding on port 8080
4. **Container Runtime**: Podman functionality and container management
5. **Ecosystem Health**: Comprehensive self-validation via `/api/v1/ecosystem`
6. **System Integration**: MicroOS base system functionality

Tests run in an isolated QEMU VM with SSH key injection and port forwarding (host:2222 → guest:22).

## Migration Notes

This system has migrated from Flatcar Linux to openSUSE MicroOS. Key differences:
- Build system changed from Gentoo/Portage to KIWI NG
- Container runtime changed from Docker to Podman
- Update mechanism uses MicroOS transactional updates instead of CoreOS update_engine
- Research and migration documentation available in `foundation.md` and `coreos-migration-research.md`