# Piccolo OS Architecture Decisions

## Overview

This document captures the key architectural decisions made during the design of Piccolo OS update system, based on comprehensive analysis and discussion of implementation approaches.

## Decision Record

### Decision 1: Direct Control vs Omaha Protocol

**Date:** 2025-01-05  
**Status:** Decided  
**Decision:** `piccolod` takes direct control of updates, Flatcar auto-updates disabled at compile-time

**Context:**
- Two approaches considered: implementing Omaha protocol server vs direct control
- Omaha protocol would require `piccolod` to act as local update server
- Direct control means disabling Flatcar updates and managing everything via `piccolod`

**Decision Rationale:**
- **Simplicity**: Direct control avoids complex Omaha protocol implementation
- **Control**: Better integration with subscription model and business logic
- **Flexibility**: Can implement advanced policies (maintenance windows, staged rollouts)
- **Maintenance**: Fewer moving parts, easier to debug and maintain

**Implementation:**
```bash
# /etc/flatcar/update.conf (compile-time)
GROUP=disabled
SERVER=  # Empty
```

**Alternatives Considered:**
- Local Omaha server: More complex, protocol overhead
- Hybrid approach: Inconsistent behavior, added complexity

---

### Decision 2: Compile-time vs Runtime Configuration

**Date:** 2025-01-05  
**Status:** Decided  
**Decision:** All Flatcar configuration overrides happen at build time, not runtime

**Context:**
- Runtime overrides would allow `piccolod` to modify Flatcar settings dynamically
- Compile-time overrides bake configuration into the OS image

**Decision Rationale:**
- **Robustness**: No configuration drift or runtime modification
- **Security**: Tamper-proof configuration baked into OS
- **Simplicity**: Build system already generates configuration files
- **Predictability**: Consistent behavior across all deployments

**Implementation:**
- Build system generates final `/etc/flatcar/update.conf`
- No runtime configuration modification needed

**Alternatives Considered:**
- Runtime modification: Risk of configuration drift, security concerns
- Hybrid approach: Added complexity without clear benefits

---

### Decision 3: Local vs Remote Health Checks

**Date:** 2025-01-05  
**Status:** Decided  
**Decision:** Health checks are entirely local using Flatcar's GPT partition success mechanism

**Context:**
- Remote health checks would require Piccolo server to monitor device health
- Local health checks leverage Flatcar's built-in rollback system

**Decision Rationale:**
- **Reliability**: No remote dependencies for critical rollback decisions
- **Latency**: Immediate rollback without network round-trips
- **Simplicity**: Leverages proven Flatcar rollback mechanism
- **Offline Operation**: Works without network connectivity

**Implementation:**
```ini
# SystemD dependency
[Unit]
After=piccolod.service
Requires=piccolod.service
```

**Technical Details:**
- Flatcar uses GPT partition attributes (`priority`, `tries`, `successful`)
- `update_engine` waits for `piccolod` service success before marking partition successful
- Automatic rollback if `piccolod` fails to start

**Alternatives Considered:**
- Remote health monitoring: Network dependencies, latency issues
- Custom rollback implementation: Reinventing proven mechanisms

---

### Decision 4: JWT Authentication with TPM

**Date:** 2025-01-05  
**Status:** Decided  
**Decision:** All API authentication via JWT tokens with embedded device/subscription info

**Context:**
- Alternative was to pass device_id and subscription as URL parameters
- JWT approach embeds all context in cryptographically signed token

**Decision Rationale:**
- **Cleaner APIs**: No redundant parameters in URLs
- **Security**: Cryptographically signed device identity and entitlements
- **Stateless**: Server doesn't need to store device state
- **Scalability**: Standard JWT validation scales horizontally

**Implementation:**
```json
{
  "device_id": "tpm-ek-hash-abc123",
  "subscription_tier": "premium",
  "features": ["disk-encryption", "priority-support"],
  "exp": 1234567890
}
```

**API Example:**
```http
GET /api/v1/updates/check
Authorization: Bearer <tpm-jwt-token>
# No device_id or subscription parameters needed
```

**Alternatives Considered:**
- URL parameters: Less secure, API clutter, prone to tampering
- Session-based auth: Stateful, doesn't scale well

---

### Decision 5: SystemD Dependency for Health Integration

**Date:** 2025-01-05  
**Status:** Decided  
**Decision:** Use SystemD service dependency to integrate with Flatcar's health check system

**Context:**
- Multiple integration approaches considered: D-Bus communication, custom scripts, service dependencies
- Need to ensure `update_engine` only marks success after `piccolod` validation

**Decision Rationale:**
- **Simplicity**: Leverages existing SystemD dependency system
- **Reliability**: Well-tested mechanism with clear failure modes
- **Integration**: Works seamlessly with Flatcar's existing rollback logic
- **Debuggability**: Clear service dependency chain in SystemD

**Implementation:**
```ini
# /etc/systemd/system/update-engine.service.d/10-piccolod-dependency.conf
[Unit]
After=piccolod.service
Requires=piccolod.service
```

**Failure Mode:**
- If `piccolod` fails to start → `update_engine` never marks partition successful
- Flatcar automatically rolls back on next reboot when `tries` counter reaches 0

**Alternatives Considered:**
- D-Bus integration: More complex, custom protocol needed
- Health check scripts: Additional complexity, timing issues
- Custom success marking: Reimplementing proven mechanisms

---

### Decision 6: TPM Disk Encryption Implementation

**Date:** 2025-01-05  
**Status:** Decided  
**Decision:** Implement TPM disk encryption using `systemd-cryptenroll` with automated re-sealing

**Context:**
- Users want hardware-backed disk encryption for homelab use
- Need to work with Flatcar's A/B partition update system
- Hardware failure risk is acceptable for target users

**Decision Rationale:**
- **Mature Tools**: `systemd-cryptenroll` is production-ready and well-tested
- **Update Compatibility**: Automated re-sealing works with A/B updates
- **User Choice**: Optional feature for users who accept hardware failure risk
- **Security**: Hardware root of trust via TPM 2.0

**Implementation Strategy:**
- **Phase 1**: Basic `systemd-cryptenroll` integration
- **Phase 2**: Automated re-sealing during updates
- **Phase 3**: Pure Go implementation using `canonical/go-tpm2`

**PCR Policy:**
```go
// Conservative policy for update compatibility
var DefaultPCRPolicy = []int{7} // Secure Boot state only
```

**Alternatives Considered:**
- Software-only encryption: No hardware binding
- Full-disk encryption without TPM: Key management complexity
- Enterprise key management: Overkill for homelab use case

---

### Decision 7: Root Privilege Requirement

**Date:** 2025-01-05  
**Status:** Decided  
**Decision:** `piccolod` must run as root user for update and installation operations

**Context:**
- Alternative approaches like privilege separation or capabilities considered
- Multiple operations require root access across different subsystems

**Decision Rationale:**
- **Technical Requirements**: Multiple operations require root access:
  - Direct disk access (`/dev/sdX`) for installation
  - D-Bus system bus for `update_engine` communication  
  - TPM device access (`/dev/tpm0`)
  - UEFI variable modification (`/sys/firmware/efi/efivars`)
  - Bootloader installation and partition management
- **Complexity**: Privilege separation would add significant complexity
- **Security**: Implement principle of least privilege within code

**Security Mitigations:**
- Comprehensive audit logging of all root operations
- Input validation and sanitization
- Minimize scope of root operations to specific functions
- Future consideration of privilege separation architecture

**Alternatives Considered:**
- Privilege separation: High complexity, limited benefit for single-user homelab
- Linux capabilities: Insufficient for required operations
- Sudo-based approach: Complex configuration, security risks

---

## Implementation Priority

Based on these decisions, the implementation priority is:

### Phase 1: Foundation (Weeks 1-4)
1. **Root service configuration** - Critical for all other features
2. **Health check integration** - Essential for reliable updates
3. **Compile-time Flatcar disable** - Required for direct control

### Phase 2: Core Features (Weeks 5-8)
4. **TPM authentication** - Device identity and security foundation
5. **Direct update control** - Core update functionality
6. **Installation system** - USB→SSD installation capability

### Phase 3: Advanced Features (Weeks 9-12)
7. **JWT API implementation** - Clean authentication system
8. **TPM disk encryption** - Enhanced security feature
9. **Enhanced testing** - Comprehensive validation framework

## Future Considerations

### Potential Architecture Evolution

1. **Privilege Separation**: Future consideration for enterprise deployments
2. **Multi-node Coordination**: Federation and cluster update coordination
3. **Policy Engine**: Advanced update policies and compliance frameworks
4. **Telemetry Integration**: Update success/failure analytics

### Technology Monitoring

1. **SystemD Evolution**: Monitor systemd updates for new integration opportunities
2. **TPM Standards**: Track TPM 2.0 specification updates and new capabilities
3. **Container Standards**: Monitor OCI and container runtime evolution
4. **Security Standards**: Track FIPS, Common Criteria, and other compliance requirements

This decision record provides the foundation for Piccolo OS update architecture implementation and future evolution.