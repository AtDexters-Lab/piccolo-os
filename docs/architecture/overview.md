# Piccolo OS Architecture Overview

Piccolo OS follows a layered architecture designed for security, maintainability, and modularity.

## System Layers

### Layer 0 (L0) - Hardware and Base OS
- **Location**: `src/l0/`
- **Purpose**: Flatcar Linux build system and ISO generation
- **Components**:
  - Custom Flatcar Linux builds with piccolod integration
  - Bootable ISO and update artifact generation
  - GPG signing and security validation
  - Build automation and testing infrastructure

### Layer 1 (L1) - Host OS and Core Daemon  
- **Location**: `src/l1/`
- **Purpose**: Core system management daemon (piccolod)
- **Components**:
  - REST API server on port 8080
  - Container management (Docker integration)
  - Storage and network management
  - TPM-based trust and encryption
  - System installation and OTA updates
  - Federation and backup operations

### Layer 2 (L2) - Applications and Runtime
- **Location**: `src/l2/`
- **Purpose**: Application layer (future development)
- **Components**: *To be defined*

## Key Design Principles

### Privacy-First
- No telemetry or data collection by default
- TPM-based local trust and encryption
- Self-hosted update infrastructure

### Container-Native
- Docker as the primary application runtime
- Immutable OS with mutable container workloads
- Storage abstractions for container persistence

### Homelab-Optimized
- Single-node focused (federation planned)
- USB-to-SSD installation workflow
- Minimal resource footprint
- Self-contained operation

## System Architecture

```
┌─────────────────────────────────────────┐
│              Applications               │ ← L2 (Future)
├─────────────────────────────────────────┤
│             piccolod API                │
│  ┌─────────┐ ┌─────────┐ ┌─────────┐   │
│  │Container│ │ Storage │ │ Network │   │ ← L1 (Core Daemon)
│  │ Manager │ │ Manager │ │ Manager │   │
│  └─────────┘ └─────────┘ └─────────┘   │
│  ┌─────────┐ ┌─────────┐ ┌─────────┐   │
│  │  Trust  │ │ Update  │ │Installer│   │
│  │  Agent  │ │ Manager │ │ Manager │   │
│  └─────────┘ └─────────┘ └─────────┘   │
├─────────────────────────────────────────┤
│           Flatcar Linux                 │ ← L0 (Base OS)
│     (Custom Build with piccolod)       │
└─────────────────────────────────────────┘
```

## Component Interactions

### Boot Process
1. **Flatcar boots** with piccolod enabled as systemd service
2. **piccolod starts** and initializes manager components
3. **Health checks** verify system integrity
4. **API server** becomes available on port 8080
5. **Ready for operations** - installation, container management, etc.

### Installation Workflow  
1. **Live USB boot** - System runs from USB with full functionality
2. **Target disk selection** - User selects SSD/NVMe for installation
3. **TPM enrollment** - Hardware trust anchor setup
4. **Disk encryption** - LUKS with TPM-sealed keys
5. **System installation** - Copy and configure persistent system
6. **Reboot to installed system** - Switch from USB to SSD boot

### Container Management
- **Docker integration** - Native Docker daemon communication
- **Image management** - Pull, build, and distribute container images
- **Resource allocation** - CPU, memory, and storage limits
- **Network policies** - Container networking and isolation

For detailed component documentation, see:
- [Architecture Decisions](decisions.md)
- [Update System](update-system.md)
- [API Design](../development/api-design.md)