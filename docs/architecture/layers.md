# Piccolo OS Layer Architecture

Piccolo OS is built using a three-layer architecture that separates concerns and enables maintainable, secure system development.

## Layer Separation Philosophy

Each layer has distinct responsibilities and interfaces:

- **L0**: Hardware abstraction and base OS provisioning
- **L1**: System services and management APIs  
- **L2**: User applications and workloads

## Layer 0 (L0) - Hardware and Base OS

**Location**: `src/l0/`

### Responsibilities
- Custom Flatcar Linux build system integration
- ISO generation and bootable media creation
- GPG signing and artifact validation
- Update payload generation
- Hardware compatibility and driver integration

### Key Components
```bash
src/l0/
├── build_piccolo.sh         # Main build orchestrator
├── test_piccolo_os_image.sh # Automated QA testing
├── piccolo.env              # Build configuration
└── build/                   # Build artifacts and workspace
```

### Outputs
- `piccolo-os-live-{version}.iso` - Bootable live ISO
- `piccolo-os-{version}.bin` - Update payload for OTA updates
- GPG signatures and checksums
- VM test artifacts

## Layer 1 (L1) - Host OS and Core Daemon

**Location**: `src/l1/piccolod/`

### Responsibilities
- System management APIs (REST on port 8080)
- Container orchestration and lifecycle management
- Storage and network resource management
- TPM-based trust and disk encryption
- System installation from live environment
- OTA update management and rollback
- Federation and backup operations

### Manager Components
```bash
src/l1/piccolod/internal/
├── server/          # HTTP API server and routing
├── container/       # Docker container management  
├── storage/         # Storage operations and pools
├── network/         # Network configuration
├── trust/           # TPM trust agent and attestation
├── installer/       # System installation from live USB
├── update/          # OTA update management
├── backup/          # Backup and restore operations
└── federation/      # Multi-node federation (future)
```

### API Surface
- `/api/v1/version` - System version and status
- `/api/v1/containers/*` - Container lifecycle management
- `/api/v1/storage/*` - Storage pool and volume operations  
- `/api/v1/network/*` - Network configuration
- `/api/v1/trust/*` - TPM attestation and identity
- `/api/v1/installer/*` - System installation
- `/api/v1/updates/*` - OTA update management

## Layer 2 (L2) - Applications and Runtime

**Location**: `src/l2/` *(Future Development)*

### Planned Responsibilities
- User-facing application APIs
- Web UI for system management
- Application marketplace and distribution
- Multi-tenant workload isolation
- Advanced orchestration features

### Integration Points
- Consumes L1 APIs for system operations
- Provides higher-level abstractions for end users
- Manages application lifecycle and dependencies

## Inter-Layer Communication

### L0 → L1 Integration
- **Build-time**: L0 embeds piccolod binary into Flatcar image
- **Runtime**: systemd manages piccolod as root service
- **Updates**: L0 generates update payloads consumed by L1

### L1 → L2 Integration  
- **API**: L1 exposes REST APIs for L2 consumption
- **Events**: L1 provides event streams for L2 monitoring
- **Resources**: L1 manages underlying system resources for L2

## Development Workflow

### Layer-Specific Development
```bash
# L0 - Full OS build and test
cd src/l0
./build.sh

# L1 - Daemon development and testing  
cd src/l1/piccolod
go build ./cmd/piccolod
go test ./...

# L2 - Application development (future)
cd src/l2
# TBD
```

### Cross-Layer Integration Testing
```bash
# Build complete system
cd src/l0
./build.sh

# Test integrated system
./test_piccolo_os_image.sh --build-dir ./build/output/1.0.0 --version 1.0.0
```

## Security Boundaries

### Trust Boundaries
- **L0/L1**: Shared trust domain (both run as root, same host)
- **L1/L2**: API boundary with authentication and authorization
- **L2/Apps**: Container isolation and resource limits

### Privilege Separation
- **L0**: Build-time privileges, no runtime presence
- **L1**: Full system privileges (root) for hardware/system management
- **L2**: User-level privileges with L1 API delegation

For implementation details, see:
- [Architecture Decisions](decisions.md)
- [API Design](../development/api-design.md)
- [Security Model](../security/trust-model.md)