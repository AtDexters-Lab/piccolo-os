# Piccolo OS Update Architecture

## Overview

This document outlines the comprehensive update architecture for Piccolo OS Layer 0, designed to provide subscription-based, secure, and automated OS updates while supporting both live USB installation and regular A/B partition updates.

## Architecture Philosophy

**Core Principle**: `piccolod` acts as the update orchestrator, communicating directly with Piccolo servers and leveraging Flatcar's proven `update_engine` for low-level A/B partition management.

**Key Benefits**:
- Skip Nebraska/Omaha protocol complexity for direct control
- Subscription-based update control for business model integration
- Hardware-backed device authentication via TPM
- Support for both live USB → SSD installation and regular updates
- Single daemon manages both installation and update operations

## Architecture Components

```
┌─────────────────────────────────────────────────────────────────┐
│                    Piccolo Server (SaaS)                       │
│              - Subscription Management                         │
│              - Update Distribution                             │
│              - TPM-based Device Authentication                 │
└─────────────────────────┬───────────────────────────────────────┘
                          │ HTTPS + Custom REST API
                          │ TPM Attestation Protocol
                          ▼
┌─────────────────────────────────────────────────────────────────┐
│                    piccolod Update Manager                     │
│              - TPM Device Identity                             │
│              - Update Check & Download                         │
│              - Signature Verification                          │
│              - Installation Orchestration                      │
└─────────────────────────┬───────────────────────────────────────┘
                          │ D-Bus Interface
                          │ Local CLI Commands
                          ▼
┌─────────────────────────────────────────────────────────────────┐
│              Flatcar update_engine + System Tools              │
│              - A/B Partition Management                        │
│              - Bootloader Configuration                        │
│              - UEFI/GPT Operations                             │
└─────────────────────────┬───────────────────────────────────────┘
                          │ Direct Hardware Access
                          ▼
┌─────────────────────────────────────────────────────────────────┐
│                         Hardware                               │
│              - A/B Partitions                                  │
│              - TPM 2.0 Module                                  │
│              - UEFI Firmware                                   │
└─────────────────────────────────────────────────────────────────┘
```

## Update Flow Scenarios

### 1. Live USB → SSD Installation

**Use Case**: User boots Piccolo live USB and wants to install to local SSD

**Flow**:
1. `piccolod` detects live USB environment via `/proc/cmdline` analysis
2. User initiates installation via API: `POST /api/v1/system/install`
3. `piccolod` authenticates with Piccolo server using TPM identity
4. Downloads latest full OS image based on subscription tier
5. Uses `go-diskfs` library for:
   - GPT partition table creation
   - EFI System Partition (ESP) setup
   - Filesystem formatting
6. Uses system tools for bootloader installation:
   - `systemd-boot` installation via `bootctl install`
   - UEFI boot entry creation via `efibootmgr`
7. Writes OS image to target partitions
8. Configures bootloader and UEFI variables

**Technical Requirements**:
- Root privileges for disk operations
- TPM 2.0 for device authentication
- UEFI-compatible target system
- Network connectivity for image download

### 2. Regular A/B Updates

**Use Case**: Installed Piccolo OS receiving OTA updates

**Flow**:
1. `piccolod` periodically checks Piccolo server: `GET /api/v1/updates/check`
2. Server responds based on:
   - Device TPM identity
   - Current OS version
   - Subscription tier and entitlements
   - Update channel (stable/beta/dev)
3. If update available:
   - Download `.raw.gz` update payload and `.asc` signature
   - Verify GPG signature using embedded public key
   - Validate subscription entitlements
4. Trigger update via D-Bus to `update_engine`:
   - Call `com.coreos.update1.Manager` interface
   - Monitor status via `StatusUpdate` signals
5. `update_engine` handles:
   - A/B partition switching
   - Bootloader priority updates
   - Atomic update application
6. Reboot coordination and rollback on failure

## API Design

### Piccolo Server REST API

```
# Device Registration & Authentication
POST /api/v1/devices/register
  - TPM EK certificate
  - Device hardware info
  - Initial subscription binding

# Update Operations
GET /api/v1/updates/check?device_id={tpm_id}&current_version={ver}&subscription={tier}
  - Returns available updates based on subscription
  - Includes update metadata and download URLs

GET /api/v1/images/download/{version}?device_id={tpm_id}&type={update|full}
  - Downloads signed OS images
  - Enforces subscription-based access control

# Subscription Management
GET /api/v1/subscriptions/{device_id}
POST /api/v1/subscriptions/{device_id}/validate
```

### piccolod Internal API

```
# System Installation
POST /api/v1/system/install
  - Target disk selection
  - Installation configuration
  - Progress monitoring

# Update Management  
GET /api/v1/updates/status
POST /api/v1/updates/check
POST /api/v1/updates/apply
POST /api/v1/updates/rollback

# Device Information
GET /api/v1/device/identity
GET /api/v1/device/subscription
```

## Technical Implementation Details

### 1. update_engine_client Integration

**Approach**: Use D-Bus interface instead of CLI for better integration

**Implementation**:
```go
package update

import "github.com/godbus/dbus/v5"

type Manager struct {
    conn *dbus.Conn
}

// D-Bus Interface: com.coreos.update1.Manager
// Object Path: /com/coreos/update1
// Service: com.coreos.update1
```

**Key Methods**:
- `GetStatus()` - Current update engine status
- Monitor `StatusUpdate` signals for state changes
- Status values: `UPDATE_STATUS_IDLE`, `UPDATE_STATUS_DOWNLOADING`, etc.

**Privileges**: Requires root access for D-Bus system bus operations

### 2. Bootloader & UEFI Handling

**Libraries**:
- `github.com/diskfs/go-diskfs` - Pure Go disk and partition management
- `github.com/0x5a17ed/uefi` - UEFI operations in Go

**Implementation Strategy**:
- Use `go-diskfs` for GPT partition creation and filesystem formatting
- Shell out to proven system tools for bootloader installation:
  - `bootctl install` for systemd-boot
  - `efibootmgr` for UEFI boot entries
  - `sgdisk` for complex partition operations

**UEFI Requirements**:
- EFI System Partition (ESP): 1GB, FAT32, type `EF00`
- Signed bootloaders for Secure Boot compatibility
- Proper certificate chain validation

**Security Considerations**:
- Validate bootloader signatures before installation
- Handle Secure Boot certificate management
- Implement proper rollback mechanisms

### 3. Image Signature Verification

**Current State**: Build system already generates GPG signatures (`.asc` files)

**Implementation**:
```go
import "golang.org/x/crypto/openpgp"

// Verify downloaded images before applying
func (m *Manager) verifyImageSignature(imagePath, sigPath string) error {
    // Load Piccolo public key from embedded keyring
    // Verify signature against image
    // Return error if verification fails
}
```

**Security Model**:
- Embedded Piccolo public key in `piccolod` binary
- All update images must have valid GPG signatures
- Signature verification before any system modifications

### 4. TPM-Based Device Authentication

**Libraries**:
- `github.com/google/go-tpm-tools` - High-level TPM operations
- `github.com/google/go-attestation` - Remote attestation protocols

**Authentication Flow**:
1. **Device Registration**: Generate Attestation Key (AK) from Endorsement Key (EK)
2. **Challenge-Response**: Server sends nonce, device responds with TPM quote
3. **Trust Establishment**: Server validates TPM certificates and PCR values
4. **Session Management**: Use attested identity for subsequent API calls

**Implementation**:
```go
package trust

import (
    "github.com/google/go-tpm-tools/client"
    "github.com/google/go-attestation/attest"
)

type Agent struct {
    tpm   *client.Client
    ak    *client.Key  // Attestation Key
}

func (a *Agent) RegisterDevice() error {
    // Generate AK from EK
    // Create TPM-based device identity
    // Register with Piccolo server
}

func (a *Agent) AuthenticateToServer() (token string, error) {
    // Perform TPM attestation challenge
    // Return authentication token
}
```

**Security Benefits**:
- Hardware root of trust
- Device identity cannot be cloned or spoofed
- Support for subscription binding to specific hardware
- Remote attestation for system integrity verification

## Root Privilege Requirements

**Yes, `piccolod` must run as root** for the following operations:

### Installation Operations
- Direct disk access (`/dev/sdX`) for partitioning
- Filesystem creation and mounting
- Bootloader installation and UEFI variable modification
- System file modifications in `/etc`, `/boot`

### Update Operations  
- D-Bus system bus access for `update_engine` communication
- Partition mounting and unmounting
- System service management via systemd

### TPM Operations
- Access to `/dev/tpm0` and `/sys/firmware/efi/efivars`
- TPM resource management and exclusive access
- Secure storage operations

### Network Operations
- Binding to privileged ports (if needed)
- System-level network configuration changes

**Security Mitigation**:
- Minimize root operations to specific functions
- Use principle of least privilege within code
- Implement comprehensive audit logging
- Consider future privilege separation architecture

## Configuration Integration

### Build-Time Configuration

**Files Modified**:
- `src/l0/piccolo.env`: Update server URLs and GPG keys
- Generated `/etc/flatcar/update.conf`: Point to custom update server

**Current Configuration**:
```bash
PICCOLO_UPDATE_SERVER="https://os-updates.system.piccolospace.com/v1/update/"
PICCOLO_UPDATE_GROUP="piccolo-stable"
GPG_SIGNING_KEY_ID="C20F379552B3EC91"
```

**Integration Points**:
- L0 build system embeds configuration into OS image
- L1 `piccolod` reads configuration and overrides default Flatcar settings
- Runtime configuration via API for subscription changes

### Runtime Configuration

**Dynamic Settings**:
- Update check frequency
- Subscription tier and entitlements  
- Update channel selection (stable/beta/dev)
- Rollback policies and thresholds

## Testing Strategy

### Automated Testing

**VM Testing Framework**: Extend existing `test_piccolo_os_image.sh`

**Test Scenarios**:
1. **TPM Detection**: Verify TPM 2.0 availability and functionality
2. **D-Bus Connectivity**: Test `update_engine` interface communication
3. **Installation Flow**: Live USB → SSD installation in VM
4. **Update Flow**: Mock update server with test payloads
5. **Signature Verification**: Test with valid and invalid signatures
6. **Rollback Testing**: Simulate failed updates and recovery

**Integration Tests**:
```bash
# TPM functionality
./test_tpm_operations.sh --vm-tpm

# Update engine integration  
./test_update_flow.sh --mock-server

# Installation process
./test_installation.sh --target-disk /dev/vdb
```

### Security Testing

**Penetration Testing**:
- TPM attestation protocol security
- Update server authentication bypass attempts
- Signature verification bypass testing
- Privilege escalation testing

**Compliance Testing**:
- UEFI Secure Boot compatibility
- TPM 2.0 specification compliance  
- Cryptographic algorithm validation

## Deployment Considerations

### Hardware Requirements

**Minimum Requirements**:
- TPM 2.0 module (discrete, integrated, or firmware)
- UEFI firmware (no Legacy/CSM support)
- 8GB storage for A/B partition scheme
- Network connectivity for updates

**Recommended Requirements**:
- Discrete TPM 2.0 for maximum security
- 32GB+ storage for multiple OS versions
- High-speed internet for large update downloads

### Operational Requirements

**Infrastructure**:
- Piccolo update server with high availability
- CDN for global update distribution
- PKI infrastructure for certificate management
- Monitoring and alerting for update failures

**Maintenance**:
- Regular GPG key rotation procedures
- Update server security patching
- Certificate renewal automation
- Rollback testing and validation

## Security Model

### Threat Model

**Protected Against**:
- Unauthorized OS updates from malicious servers
- Update tampering during download or storage
- Device identity spoofing or cloning
- Unauthorized feature access based on subscription
- System integrity compromise detection

**Attack Vectors Considered**:
- Network-based attacks during update download
- Local privilege escalation attempts
- TPM implementation vulnerabilities
- Bootloader compromise and persistence
- Social engineering of support processes

### Security Controls

**Cryptographic Controls**:
- GPG signature verification for all updates
- TPM-backed device identity and attestation
- TLS encryption for all network communications
- Secure Boot verification of bootloader integrity

**Access Controls**:
- Subscription-based feature and update access
- TPM-enforced device binding for licenses
- Role-based access control for server APIs
- Multi-factor authentication for administrative access

**Operational Controls**:
- Comprehensive audit logging of all operations
- Automated security monitoring and alerting
- Regular security assessments and penetration testing
- Incident response procedures for security events

## Future Enhancements

### Short Term (Next 6 months)
- Implement basic D-Bus `update_engine` integration
- Add TPM device registration and authentication
- Create USB→SSD installation capability
- Build signature verification for downloaded images

### Medium Term (6-12 months)
- Full remote attestation protocol implementation
- Advanced subscription management and enforcement  
- Staged rollout capabilities with canary deployments
- Enhanced monitoring and telemetry collection

### Long Term (12+ months)
- Multi-architecture support (ARM64, RISC-V)
- Container-based application updates (Layer 2)
- Edge computing and offline update capabilities
- Advanced AI/ML-driven update optimization

## Implementation Roadmap

### Phase 1: Foundation (Weeks 1-4)
1. Enhance `internal/update/manager.go` with D-Bus integration
2. Implement basic TPM operations in `internal/trust/agent.go`
3. Add signature verification for update images
4. Create API endpoints for update status and control

### Phase 2: Installation (Weeks 5-8)  
1. Implement USB→SSD installation using `go-diskfs`
2. Add UEFI bootloader configuration with systemd-boot
3. Create comprehensive error handling and rollback
4. Build automated testing framework for installation

### Phase 3: Server Integration (Weeks 9-12)
1. Implement Piccolo server API communication
2. Add TPM-based device authentication protocol
3. Create subscription validation and enforcement
4. Build monitoring and telemetry collection

### Phase 4: Production Readiness (Weeks 13-16)
1. Security audit and penetration testing
2. Performance optimization and resource management
3. Documentation and operational procedures
4. Production deployment and monitoring setup

This architecture provides a robust, secure, and scalable foundation for Piccolo OS updates while maintaining simplicity and leveraging proven technologies.