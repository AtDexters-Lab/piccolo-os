# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Layer 0 (l0) contains the Flatcar Linux-based OS build system for Piccolo OS. This layer is responsible for creating bootable ISO images and update packages that include the piccolod daemon integrated as a systemd service.

## Development Commands

### Complete OS Build
Build the entire Piccolo OS including ISO and update packages:
```bash
./build.sh
```

This script:
1. Builds the piccolod binary from `../l1/piccolod`
2. Creates a custom Flatcar Linux image with piccolod integrated
3. Generates bootable ISO and update artifacts
4. Signs artifacts with GPG

### Direct Build Script
For more control over the build process:
```bash
./build_piccolo.sh --version 1.0.0 --binary-path ../l1/piccolod/build/piccolod
```

### Testing the Built Image
Test the complete OS in a VM:
```bash
./test_piccolo_os_image.sh --build-dir ./build/output/1.0.0 --version 1.0.0
```

This automated test script:
- Boots the ISO in QEMU
- Verifies piccolod binary and service are running
- Tests HTTP API endpoints
- Validates container runtime functionality
- Runs ecosystem health checks

### Interactive Debugging
For manual testing and debugging:
```bash
./ssh_into_piccolo.sh --build-dir ./build/output/1.0.0 --version 1.0.0
```

## Architecture Details

### Build System Integration

The build process integrates piccolod into Flatcar Linux through:

1. **Custom Ebuild Package**: Creates `app-misc/piccolod-bin` in the Flatcar overlay
2. **Systemd Integration**: Installs piccolod.service with security hardening
3. **System Configuration**: Sets up update configuration pointing to Piccolo's update server
4. **Package Dependencies**: Adds piccolod as a dependency to the base coreos package

### Key Build Components

- **`build_piccolo.sh`**: Main build orchestration script with modular functions
- **`build.sh`**: Simple wrapper that builds piccolod then calls build_piccolo.sh
- **`test_piccolo_os_image.sh`**: Comprehensive automated testing framework
- **`ssh_into_piccolo.sh`**: Interactive debugging tool for manual testing
- **`piccolo.env`**: Build configuration (GPG keys, update server URLs)

### Generated Artifacts

Build outputs are stored in `./build/output/{version}/`:
- `piccolo-os-live-{version}.iso` - Bootable live ISO
- `piccolo-os-update-{version}.raw.gz` - Compressed update image
- `piccolo-os-update-{version}.raw.gz.asc` - GPG signature

### Systemd Service Configuration

The generated piccolod.service includes security hardening:
- Runs as root (required for system operations)
- Capability restrictions (CAP_SYS_ADMIN, CAP_NET_ADMIN, etc.)
- Filesystem access controls via ReadWritePaths
- Automatic restart on failure

### Update System Configuration

Flatcar's update_engine is configured to:
- Point to Piccolo's update server (defined in piccolo.env)
- Use the "piccolo-stable" update group
- Allow piccolod to control the update process

## Dependencies

Required system dependencies:
- `git` - Repository operations
- `docker` - SDK container execution
- `gpg` - Artifact signing
- `qemu-system-x86_64` - VM testing
- `ssh`, `ssh-keygen` - Remote testing
- `ss` - Network port checking

## Configuration

### Build Environment (piccolo.env)
- `GPG_SIGNING_KEY_ID`: Key for signing release artifacts
- `PICCOLO_UPDATE_SERVER`: OTA update server URL
- `PICCOLO_UPDATE_GROUP`: Update channel (e.g., "piccolo-stable")

### Build Constants
- Board: `amd64-usr`
- Ebuild category: `app-misc`
- Package name: `piccolod-bin`
- HTTP port: `8080`

## Testing Framework

The test suite validates:
1. **Binary Installation**: piccolod present and executable
2. **Service Status**: systemd service active and running as root
3. **API Functionality**: HTTP endpoints responding correctly
4. **Container Runtime**: Docker functionality
5. **Ecosystem Health**: Comprehensive self-validation via `/api/v1/ecosystem`

Tests run in an isolated QEMU VM with cloud-init for SSH key injection.