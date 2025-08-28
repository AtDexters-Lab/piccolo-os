# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is the `piccolod` Go service - Layer 1 of the Piccolo OS architecture. It's a headless daemon that serves as the core app platform orchestrator for Piccolo OS, enabling users to install, manage, and run containerized applications with a mobile OS-like experience.

### Key Design Principles

- **Stateless Device Model**: Applications and data persist on federated storage, making the physical device recoverable
- **Two-Level Security**: TPM-based device key + user passphrase for data encryption
- **Unified Access**: Supports both local (`piccolo.local:PORT`) and remote (`app.user.piccolospace.com`) access
- **Container Native**: All applications run as Docker containers with strict sandboxing

## Development Commands

### Building

Build the daemon binary:
```bash
./build.sh [VERSION]
```

This creates `./build/piccolod` with the specified version (defaults to "dev").

Standard Go commands also work:
```bash
go build ./cmd/piccolod           # Build binary
go run ./cmd/piccolod             # Run directly
go build -o piccolod ./cmd/piccolod  # Build with custom output name
```

### Testing

Run tests:
```bash
go test ./...                     # All tests
go test ./internal/mdns/...       # Specific package tests
go test -v ./internal/mdns        # Verbose output
go test -run TestSpecificFunction # Run specific test
```

The test suite is primarily in the `internal/mdns` package with comprehensive unit and integration tests.

### Development

```bash
go mod tidy                       # Clean dependencies
go fmt ./...                      # Format code
go vet ./...                      # Static analysis
```

## Architecture

### Core Components

The server initializes and manages these core managers:

- `container.Manager` - Docker container operations
- `storage.Manager` - Storage management 
- `network.Manager` - Network configuration
- `trust.Agent` - Security and trust operations
- `installer.Installer` - System installation
- `update.Manager` - OTA updates
- `backup.Manager` - Backup operations
- `federation.Manager` - Multi-node federation
- `mdns.Manager` - mDNS service discovery

### Entry Point

- `cmd/piccolod/main.go` - Simple entry point that creates and starts the server
- `internal/server/server.go` - Main server initialization and component coordination

### HTTP API

The server runs on port 80 and provides REST endpoints:

**Current Implementation:**
- `/api/v1/containers` - Container management
- `/api/v1/health` - Full ecosystem health details
- `/api/v1/health/ready` - Simple readiness check
- `/version` - Version information

**Planned App Platform API (from PRD):**
- `POST /api/v1/apps` - Install/register new app from app.yaml
- `GET /api/v1/apps` - List all apps and their status
- `DELETE /api/v1/apps/{id}` - Uninstall app
- `POST /api/v1/apps/{id}/start|stop` - Control app state

### App Definition Format

Applications are defined in `app.yaml` files with:
- `name` - Application identifier
- `image` - Container image
- `subdomain` - For remote access routing
- `type` - `system` or `user` (determines boot order)
- Optional: `ports`, `volumes`, `environment`

### Boot & Unlock Sequence (from PRD)

1. Device boots, `piccolod` starts
2. Uses TPM to unseal device secret
3. Authenticates to central Piccolo server for bootstrap config
4. Starts `storage-provider` system container
5. Presents web UI for user passphrase input
6. Derives decryption key using Argon2id from passphrase
7. Unlocks and mounts federated storage volume
8. Reads `apps.db` and starts all `user` type applications

### Key Patterns

- Each manager is initialized in `server.New()` and follows a consistent interface pattern
- The server uses systemd notify protocol for proper service lifecycle management
- mDNS manager handles service discovery and must start successfully for the server to run
- All managers are designed to be independently testable
- **Security Model**: Two-level encryption (TPM + user passphrase), ephemeral in-memory keys
- **Storage Architecture**: Apps.db on federated storage tracks all application state

### Package Structure

- `internal/api/` - Shared API type definitions
- `internal/server/` - HTTP server and routing
- `internal/mdns/` - Service discovery (most comprehensive test coverage)
- `internal/*/` - Individual manager packages for different system areas

The codebase uses standard Go project layout with internal packages to prevent external imports of implementation details.