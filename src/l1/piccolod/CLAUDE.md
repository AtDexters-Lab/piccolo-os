# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is the `piccolod` Go service - Layer 1 of the Piccolo OS architecture. It's a headless daemon that serves as the core app platform orchestrator for Piccolo OS, enabling users to install, manage, and run containerized applications with a mobile OS-like experience.

### Key Design Principles

- **Stateless Device Model**: Applications and data persist on federated storage, making the physical device recoverable
- **Two-Level Security**: TPM-based device key + user passphrase for data encryption
- **Service-Oriented Architecture**: Apps expose named services, system auto-allocates ports
- **Three-Layer Security**: Container (internal) → Host binding (127.0.0.1) → Public proxy (0.0.0.0)
- **Unified Access**: Supports both local (auto-allocated ports) and remote (`app.user.piccolospace.com:port`) access
- **Container Native**: All applications run as Podman containers with strict sandboxing

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

The test suite includes 15 test files across multiple packages, with the most comprehensive test coverage in `internal/mdns` (8 test files covering security, integration, protocol, and resilience testing). The `internal/app` package also has solid test coverage with unit and integration tests.

### Development

```bash
go mod tidy                       # Clean dependencies
go fmt ./...                      # Format code
go vet ./...                      # Static analysis
```

### Testing Specific Packages

Due to the distributed test coverage across packages, focus testing on:

```bash
# mDNS package (most comprehensive tests)
go test -v ./internal/mdns/...    # All mDNS tests
go test -v ./internal/mdns -run TestSecurity  # Security-specific tests

# App management (core functionality)
go test -v ./internal/app/...     # App lifecycle tests
go test -v ./internal/app -run TestIntegration  # Integration tests

# Server endpoints  
go test -v ./internal/server/...  # Gin handler tests
```

## Architecture

### Core Components

The server initializes and manages these core managers:

- `container.Manager` - Podman container operations with security-first defaults
- `app.FSManager` - Application lifecycle with filesystem-based state management
- `ServiceManager` - Service-oriented port allocation and discovery (planned)
- `ProxyManager` - Three-layer proxy system with middleware (planned)
- `storage.Manager` - Storage management 
- `network.Manager` - Network configuration
- `trust.Agent` - Security and trust operations
- `installer.Installer` - System installation
- `update.Manager` - OTA updates
- `backup.Manager` - Backup operations
- `federation.Manager` - Multi-node federation
- `mdns.Manager` - mDNS service discovery

### Entry Point

- `cmd/piccolod/main.go` - Simple entry point that creates and starts the Gin server
- `internal/server/gin_server.go` - Main Gin-based server initialization and component coordination

### HTTP API

The server runs on port 80 and provides REST endpoints:

**Current Implementation:**
- `/` - Web UI (SPA with Tailwind CSS)
- `/api/v1/containers` - Container management
- `/api/v1/apps` - App management (install, list, start/stop, uninstall)
- `/api/v1/health` - Full ecosystem health details
- `/api/v1/health/ready` - Simple readiness check
- `/version` - Version information

**Service-Oriented API (planned):**
- `GET /api/v1/services` - List all services across apps
- `GET /api/v1/apps/{app}/services` - List services for specific app
- `GET /api/v1/services/{service}` - Get specific service details
- Service discovery endpoints for dynamic port allocation

### App Definition Format

Applications are defined in `app.yaml` files with:

**Legacy Structure (current):**
- `name` - Application identifier
- `image` - Container image
- `subdomain` - For remote access routing
- `type` - `system` or `user` (determines boot order)
- `ports` - Port mappings (host:container)

**Service-Oriented Structure (planned):**
```yaml
name: myapp
subdomain: myapp
listeners:
  - name: frontend        # Service name
    guest_port: 80        # Port inside container
    protocol: http        # Protocol hint for optimization
    flow: tcp            # tcp (piccolod handles TLS) | tls (passthrough)
    middleware: [auth, rate_limit]
```

**Three-Layer Port Allocation:**
- **Layer 1**: Container internal ports (80, 8080)
- **Layer 2**: Host security binding (127.0.0.1:15001, 127.0.0.1:15002)  
- **Layer 3**: Public proxy listeners (0.0.0.0:35001, 0.0.0.0:35002)

**Access Patterns:**
- **Local**: `http://localhost:35001` (public proxy port)
- **Remote**: `https://myapp.user.piccolospace.com:80` (service-based routing)

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

- Each manager is initialized in `NewGinServer()` and follows a consistent interface pattern
- The server uses systemd notify protocol for proper service lifecycle management  
- mDNS manager handles service discovery and must start successfully for the server to run (critical dependency)
- All managers are designed to be independently testable
- **Gin Framework**: Uses Gin web framework with middleware for CORS, security headers, logging, and recovery
- **Manager Composition**: EcosystemManager aggregates all other managers for unified health checks
- **Filesystem State**: AppManager uses filesystem-based state management for persistence
- **Security Model**: Two-level encryption (TPM + user passphrase), ephemeral in-memory keys
- **Storage Architecture**: Apps.db on federated storage tracks all application state
- **Container Security**: All containers forced to bind to 127.0.0.1 only (fortress architecture)
- **Service Discovery**: Apps expose named services, system handles all port allocation
- **Proxy Architecture**: Three-layer routing with middleware processing capabilities

### Package Structure

- `internal/api/` - Shared API type definitions
- `internal/server/` - HTTP server and routing
- `internal/mdns/` - Service discovery (most comprehensive test coverage)
- `internal/*/` - Individual manager packages for different system areas

The codebase uses standard Go project layout with internal packages to prevent external imports of implementation details.