# Piccolo OS API Design

## Overview

This document outlines the refined API design for Piccolo OS, emphasizing JWT-based authentication via TPM attestation and simplified, secure communication patterns.

## Key Design Principles

1. **JWT-Based Authentication**: All API calls authenticated via TPM-generated JWT tokens
2. **No Parameter Redundancy**: Device ID, subscription info embedded in JWT claims
3. **Stateless Design**: Server extracts all context from JWT token
4. **TPM Hardware Root of Trust**: Device identity cannot be spoofed or cloned

## Authentication Flow

### 1. Device Registration (One-time)
```http
POST /api/v1/devices/register
Content-Type: application/json

{
  "tpm_ek_certificate": "-----BEGIN CERTIFICATE-----...",
  "attestation_quote": "base64-encoded-tpm-quote",
  "nonce": "server-provided-nonce",
  "device_info": {
    "hostname": "piccolo-homelab-001",
    "cpu_arch": "x86_64",
    "tpm_version": "2.0"
  }
}
```

**Response:**
```json
{
  "jwt_token": "eyJhbGciOiJIUzI1NiIs...",
  "refresh_token": "refresh-token-for-renewal",
  "expires_in": 3600
}
```

### 2. JWT Token Structure
```json
{
  "device_id": "tpm-ek-hash-abc123def456",
  "subscription_tier": "premium",
  "features": [
    "advanced-updates",
    "priority-support", 
    "disk-encryption",
    "federation"
  ],
  "update_channel": "stable",
  "hardware_binding": {
    "tpm_ek_hash": "abc123def456",
    "cpu_id": "intel-i7-12700k"
  },
  "exp": 1234567890,
  "iss": "piccolo-auth-server",
  "aud": "piccolo-update-service"
}
```

## Piccolo Server API

All endpoints require: `Authorization: Bearer <tpm-jwt-token>`

### Update Operations
```http
# Check for available updates
GET /api/v1/updates/check
# Server extracts device_id, subscription_tier, update_channel from JWT

# Download update image
GET /api/v1/images/download/{version}?type={update|full}
# Access control based on JWT subscription claims

# Report update status
POST /api/v1/updates/status
{
  "current_version": "1.2.3",
  "update_status": "completed|failed|in_progress",
  "health_status": "healthy|degraded|unhealthy"
}
```

### Subscription Management
```http
# Get current subscription status
GET /api/v1/subscription/status

# Refresh subscription data
POST /api/v1/subscription/refresh

# Validate feature access
GET /api/v1/features/{feature_name}/access
```

### Device Management
```http
# Get device information
GET /api/v1/device/info

# Update device configuration
PATCH /api/v1/device/config
{
  "update_channel": "beta",
  "auto_update_enabled": true,
  "maintenance_window": "02:00-04:00"
}
```

## piccolod Internal API

These endpoints are exposed locally for system management and health monitoring.

### System Installation
```http
POST /api/v1/system/install
{
  "target_disk": "/dev/sdb",
  "encryption_config": {
    "enabled": true,
    "tpm_sealed": true,
    "backup_recovery_key": true
  },
  "bootloader": "systemd-boot",
  "partition_scheme": "gpt"
}
```

### Update Management
```http
# Get update status
GET /api/v1/updates/status
{
  "current_version": "1.2.3",
  "available_version": "1.2.4",
  "update_status": "idle|checking|downloading|applying|rebooting",
  "last_check": "2024-01-15T10:30:00Z",
  "next_check": "2024-01-15T14:30:00Z"
}

# Trigger manual update check
POST /api/v1/updates/check

# Apply available update
POST /api/v1/updates/apply

# Rollback to previous version
POST /api/v1/updates/rollback
```

### Health Check (Critical for Flatcar Integration)
```http
GET /api/v1/system/health
{
  "status": "healthy|degraded|unhealthy",
  "components": {
    "piccolod": "healthy",
    "docker": "healthy",
    "tpm": "healthy", 
    "storage": "healthy",
    "network": "healthy",
    "update_engine": "healthy"
  },
  "version": "1.2.3",
  "boot_id": "abc123def456",
  "uptime": 3600,
  "update_status": "completed|pending|failed",
  "disk_encryption": "enabled|disabled",
  "last_health_check": "2024-01-15T10:35:00Z"
}
```

### Device Information
```http
# Get TPM status and device identity
GET /api/v1/device/identity
{
  "device_id": "tmp-ek-hash-abc123def456",
  "tmp_status": "healthy",
  "ek_certificate": "-----BEGIN CERTIFICATE-----...",
  "ak_public_key": "-----BEGIN PUBLIC KEY-----...",
  "subscription_tier": "premium",
  "features": ["disk-encryption", "advanced-updates"]
}

# Get subscription information
GET /api/v1/device/subscription
{
  "tier": "premium",
  "status": "active",
  "expires": "2024-12-31T23:59:59Z",
  "features": ["disk-encryption", "priority-support"],
  "usage": {
    "updates_this_month": 3,
    "support_tickets": 1
  }
}

# Get TPM health and status
GET /api/v1/device/tpm-status
{
  "version": "2.0",
  "manufacturer": "Infineon",
  "model": "SLB9665",
  "firmware_version": "7.85",
  "status": "healthy",
  "capabilities": ["sealing", "attestation", "nvram"],
  "pcr_values": {
    "0": "sha256:abc123...",
    "7": "sha256:def456..."
  }
}
```

## Security Considerations

### JWT Token Security
- **Short Expiration**: Tokens expire in 1 hour, require refresh
- **Hardware Binding**: Tokens include TPM EK hash for hardware binding
- **Signature Verification**: All tokens signed by Piccolo auth server
- **Scope Limitation**: Tokens scoped to specific operations and features

### TPM Integration
- **Hardware Root of Trust**: Device identity backed by TPM hardware
- **Attestation Required**: Periodic re-attestation for token renewal
- **Anti-Cloning**: TPM private keys never leave hardware
- **Secure Storage**: Sensitive data sealed to TPM PCR values

### API Security
- **TLS Required**: All API communication over HTTPS
- **Rate Limiting**: API calls rate-limited per device
- **Input Validation**: Strict validation of all API inputs
- **Audit Logging**: Comprehensive logging of all API operations

## Error Handling

### Standard Error Response
```json
{
  "error": {
    "code": "SUBSCRIPTION_EXPIRED",
    "message": "Device subscription has expired",
    "details": {
      "expired_date": "2024-01-01T00:00:00Z",
      "renewal_url": "https://piccolo.space/renew"
    }
  },
  "request_id": "req-abc123def456"
}
```

### Common Error Codes
- `DEVICE_NOT_REGISTERED`: Device needs initial registration
- `SUBSCRIPTION_EXPIRED`: Subscription renewal required
- `FEATURE_NOT_AVAILABLE`: Feature not included in subscription
- `TPM_ATTESTATION_FAILED`: TPM hardware verification failed
- `UPDATE_NOT_AVAILABLE`: No updates available for device
- `HEALTH_CHECK_FAILED`: System health check failed

## Implementation Notes

### JWT Library Recommendations
- **Go**: `golang-jwt/jwt/v5` for JWT handling
- **TPM Integration**: `google/go-tpm-tools` for TPM operations
- **HTTP Client**: Standard `net/http` with proper timeout configuration

### Caching Strategy
- **JWT Tokens**: Cache valid tokens in memory with expiration
- **Subscription Data**: Cache subscription info for offline operation
- **Update Metadata**: Cache update information to reduce API calls

### Offline Operation
- **Grace Period**: Continue operation for 24 hours with expired token
- **Cached Permissions**: Use last known subscription data when offline
- **Emergency Updates**: Allow critical security updates without subscription check

This API design provides a clean, secure, and scalable foundation for Piccolo OS device management and updates while leveraging TPM hardware for strong device identity and authentication.