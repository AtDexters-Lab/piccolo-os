# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Piccolo OS is a privacy-first, headless container-native operating system for homelabs built on Flatcar Linux. The project follows a layered architecture with three main components:

- **Layer 0 (l0)**: Hardware and base OS - Contains Flatcar Linux build system and ISO generation
- **Layer 1 (l1)**: Host OS and core daemon - Contains `piccolod`, the main Go service that manages containers, storage, networking, and system operations
- **Layer 2 (l2)**: Applications and runtime (future development)

## Development Commands

### Building the Complete OS

Build the entire Piccolo OS including the ISO:
```bash
cd src/l0
./build.sh
```

This will:
1. Build the `piccolod` binary from Layer 1
2. Create a custom Flatcar Linux image with `piccolod` included as a systemd service
3. Generate bootable ISO and update artifacts
4. Sign the artifacts with GPG

### Building Only the Daemon

Build just the `piccolod` Go binary:
```bash
cd src/l1/piccolod
./build.sh [VERSION]
```

### Testing

Test the complete OS build in a VM:
```bash
cd src/l0  
./test_piccolo_os_image.sh --build-dir ./build/output/1.0.0 --version 1.0.0
```

This script boots the ISO in QEMU and runs automated checks to verify:
- `piccolod` binary is present and executable
- `piccolod` service is running
- HTTP API is responding correctly
- Container runtime (Docker) is functional

### Go Development

Standard Go commands work in the `src/l1/piccolod` directory:
```bash
go build ./cmd/piccolod
go test ./...
go mod tidy
```

## Architecture Details

### Piccolod Service Structure

The main `piccolod` daemon is structured with manager components for different system areas:

- `internal/server/` - Main HTTP server and routing
- `internal/container/` - Docker container management
- `internal/storage/` - Storage operations
- `internal/network/` - Network configuration
- `internal/trust/` - Security and trust management  
- `internal/backup/` - Backup operations
- `internal/federation/` - Multi-node federation
- `internal/installer/` - System installation
- `internal/update/` - OTA updates
- `internal/api/` - API type definitions

The service runs on port 8080 and provides REST API endpoints under `/api/v1/`.

### Build System Integration

The Layer 0 build system (`src/l0/build_piccolo.sh`) integrates `piccolod` into Flatcar Linux by:

1. Creating a custom ebuild package (`app-misc/piccolod-bin`) in the Flatcar overlay
2. Installing the binary to `/usr/bin/piccolod` 
3. Adding a systemd service file that enables `piccolod` by default
4. Building the complete OS image with the Flatcar build system
5. Generating signed update payloads

### Configuration

Build configuration is stored in `src/l0/piccolo.env`:
- `GPG_SIGNING_KEY_ID` - Key for signing release artifacts
- `PICCOLO_UPDATE_SERVER` - OTA update server URL
- `PICCOLO_UPDATE_GROUP` - Update channel (e.g., "piccolo-stable")

## Key Files

- `src/l0/build_piccolo.sh` - Main OS build script
- `src/l0/test_piccolo_os_image.sh` - Automated testing script
- `src/l1/piccolod/cmd/piccolod/main.go` - Entry point for the daemon
- `src/l1/piccolod/internal/server/server.go` - Main server setup and component initialization
- `src/l0/piccolo.env` - Build environment configuration