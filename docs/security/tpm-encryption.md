# TPM-Based Disk Encryption for Piccolo OS

## Overview

This document outlines the implementation of TPM 2.0-based disk encryption for Piccolo OS, providing hardware-backed data protection while integrating seamlessly with Flatcar's A/B partition update system.

## Architecture Overview

### Key Components

1. **TPM 2.0 Hardware**: Hardware root of trust for key sealing
2. **systemd-cryptenroll**: Modern Linux TPM disk encryption tool
3. **LUKS2**: Advanced disk encryption with TPM integration
4. **Automated Re-sealing**: Key management during OS updates
5. **Recovery Mechanisms**: Backup keys for hardware failure scenarios

### Security Model

**Risk Acceptance**: Users choosing TPM disk encryption accept hardware failure risks in exchange for stronger protection against:
- Data theft from powered-off systems
- Unauthorized physical access
- Software-based attacks on stored data
- Compliance requirements for data at rest encryption

## Implementation Strategy

### Phase 1: Basic TPM Disk Encryption

**Integration Points:**
- **Installation**: Add TPM encryption option during USB→SSD installation
- **Trust Agent**: Enhance existing `trust.Agent` with TPM disk management
- **Storage Manager**: Add encrypted volume management capabilities

#### 1. TPM Detection and Validation

```go
// internal/trust/agent.go
func (a *Agent) ValidateTPMForDiskEncryption() error {
    // Open TPM device
    rw, err := tpm2.OpenTPM("/dev/tpm0")
    if err != nil {
        return fmt.Errorf("TPM not available: %v", err)
    }
    defer rw.Close()
    
    // Check TPM 2.0 capabilities
    caps, err := tpm2.GetCapability(rw, tpm2.CapabilityTPMProperties, 1, uint32(tpm2.TPMPTFamilyIndicator))
    if err != nil {
        return fmt.Errorf("failed to read TPM capabilities: %v", err)
    }
    
    // Verify required algorithms are supported
    algos := []tpm2.Algorithm{tpm2.AlgSHA256, tpm2.AlgAES}
    for _, algo := range algos {
        if !a.isAlgorithmSupported(rw, algo) {
            return fmt.Errorf("required algorithm %v not supported", algo)
        }
    }
    
    return nil
}
```

#### 2. Disk Encryption During Installation

```go
// internal/installer/installer.go
type EncryptionConfig struct {
    Enabled           bool   `json:"enabled"`
    TPMSealed         bool   `json:"tmp_sealed"`
    BackupRecoveryKey bool   `json:"backup_recovery_key"`
    PCRPolicy         []int  `json:"pcr_policy"`
}

func (i *Installer) setupDiskEncryption(device string, config EncryptionConfig) error {
    if !config.Enabled {
        return nil
    }
    
    // Create LUKS2 encrypted partition
    luksCmd := exec.Command("cryptsetup", "luksFormat", 
        "--type", "luks2",
        "--cipher", "aes-xts-plain64",
        "--key-size", "512",
        "--hash", "sha256",
        "--use-random",
        device)
    
    // Set temporary passphrase for initial setup
    luksCmd.Stdin = strings.NewReader(i.generateTempPassphrase())
    if err := luksCmd.Run(); err != nil {
        return fmt.Errorf("LUKS format failed: %v", err)
    }
    
    // Open encrypted device
    if err := i.openLUKSDevice(device, "piccolo_root"); err != nil {
        return err
    }
    
    // Enroll TPM for unsealing
    if config.TPMSealed {
        if err := i.enrollTPM(device, config.PCRPolicy); err != nil {
            return fmt.Errorf("TPM enrollment failed: %v", err)
        }
    }
    
    // Generate backup recovery key if requested
    if config.BackupRecoveryKey {
        recoveryKey, err := i.generateRecoveryKey(device)
        if err != nil {
            return err
        }
        i.storeRecoveryKey(recoveryKey)
    }
    
    return nil
}
```

#### 3. TPM Key Enrollment

```go
func (i *Installer) enrollTPM(device string, pcrPolicy []int) error {
    // Use systemd-cryptenroll for TPM integration
    pcrArg := fmt.Sprintf("--tpm2-pcrs=%s", strings.Join(intSliceToString(pcrPolicy), "+"))
    
    cmd := exec.Command("systemd-cryptenroll",
        "--tpm2-device=auto",
        pcrArg,
        "--tmp2-with-pin=false", // No PIN for automatic unlock
        device)
    
    if err := cmd.Run(); err != nil {
        return fmt.Errorf("TPM enrollment failed: %v", err)
    }
    
    // Remove temporary passphrase slot
    removeCmd := exec.Command("cryptsetup", "luksKillSlot", device, "0")
    removeCmd.Stdin = strings.NewReader(i.tempPassphrase)
    
    return removeCmd.Run()
}
```

### Phase 2: A/B Update Integration

#### PCR Policy Strategy

**Conservative PCR Selection for Updates:**
- **PCR 7**: Secure Boot state (most stable across updates)
- **PCR 1**: CPU microcode and option ROMs (if needed)
- **Avoid PCR 0**: Too sensitive to firmware changes

```go
// Recommended PCR policy for update compatibility
var DefaultPCRPolicy = []int{7} // Secure Boot state only

// Advanced policy for higher security (requires more update management)
var AdvancedPCRPolicy = []int{1, 7} // Include CPU microcode
```

#### Automated Re-sealing During Updates

```go
// internal/update/manager.go
func (m *Manager) resealTPMKeys() error {
    log.Println("Re-sealing TPM keys for new OS version...")
    
    // Get current LUKS devices
    devices, err := m.getLUKSDevices()
    if err != nil {
        return err
    }
    
    for _, device := range devices {
        if err := m.resealDevice(device); err != nil {
            return fmt.Errorf("failed to reseal %s: %v", device, err)
        }
    }
    
    return nil
}

func (m *Manager) resealDevice(device string) error {
    // Add temporary passphrase for transition
    tempPass := m.generateTempPassphrase()
    if err := m.addLUKSKeySlot(device, tempPass); err != nil {
        return err
    }
    
    // Remove old TPM-sealed key
    if err := m.removeTPMKeySlot(device); err != nil {
        return err
    }
    
    // Re-enroll TPM with current PCR values
    if err := m.enrollTPM(device, DefaultPCRPolicy); err != nil {
        return err
    }
    
    // Remove temporary passphrase
    return m.removeLUKSKeySlot(device, tempPass)
}
```

#### Update Flow Integration

```go
func (m *Manager) ApplyOSUpdate() error {
    // 1. Check if disk encryption is enabled
    encryptionEnabled, err := m.isDiskEncryptionEnabled()
    if err != nil {
        return err
    }
    
    // 2. If encrypted, prepare for re-sealing
    if encryptionEnabled {
        if err := m.prepareEncryptionForUpdate(); err != nil {
            return fmt.Errorf("encryption preparation failed: %v", err)
        }
    }
    
    // 3. Apply update via Flatcar update_engine
    if err := m.triggerFlatcarUpdate(); err != nil {
        return err
    }
    
    // 4. After reboot, re-seal TPM keys (done via systemd service)
    
    return nil
}
```

### Phase 3: Advanced Features

#### 1. Enterprise Policy Integration

```go
type DiskEncryptionPolicy struct {
    Required          bool     `yaml:"required"`
    AllowedPCRs       []int    `yaml:"allowed_pcrs"`
    RequireBackupKey  bool     `yaml:"require_backup_key"`
    KeyRotationDays   int      `yaml:"key_rotation_days"`
    ComplianceMode    string   `yaml:"compliance_mode"` // "basic", "fips", "cc"
}
```

#### 2. Remote Key Management

```go
// Enterprise feature: Remote key escrow
func (a *Agent) escrowRecoveryKey(deviceID string, encryptedKey []byte) error {
    // Encrypt recovery key with enterprise public key
    enterpriseKey, err := a.getEnterprisePublicKey()
    if err != nil {
        return err
    }
    
    doubleEncrypted, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, enterpriseKey, encryptedKey, nil)
    if err != nil {
        return err
    }
    
    // Store in enterprise key management system
    return a.storeInEnterpriseKMS(deviceID, doubleEncrypted)
}
```

## systemd Integration

### Automatic Unlock Service

```ini
# /etc/systemd/system/luks-tpm-unlock.service
[Unit]
Description=TPM-based LUKS unlock
DefaultDependencies=no
After=systemd-tpm2-generator.service
Before=local-fs.target
Conflicts=shutdown.target

[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=/usr/bin/systemd-cryptsetup attach piccolo_root /dev/disk/by-uuid/xxxx - tpm2-device=auto,headless=1
TimeoutSec=60

[Install]
WantedBy=local-fs.target
```

### Re-sealing Service

```ini
# /etc/systemd/system/piccolo-tpm-reseal.service
[Unit]
Description=Re-seal TPM keys after OS update
After=piccolo.service
ConditionPathExists=/var/lib/piccolod/tpm-reseal-required

[Service]
Type=oneshot
ExecStart=/usr/bin/piccolod --reseal-tpm-keys
ExecStartPost=/bin/rm -f /var/lib/piccolod/tpm-reseal-required
TimeoutSec=300

[Install]
WantedBy=multi-user.target
```

## Security Considerations

### Threat Model

**Protected Against:**
- Data theft from powered-off systems
- Unauthorized physical access to storage
- Software attacks on data at rest
- Compliance violations for sensitive data

**Not Protected Against:**
- Attacks on running systems with unlocked encryption
- Hardware modification attacks (advanced adversaries)
- TPM reset/clearing attacks
- Side-channel attacks on TPM operations

### Key Management Security

```go
// Secure key generation and handling
func (i *Installer) generateRecoveryKey(device string) (string, error) {
    // Generate cryptographically secure recovery key
    keyBytes := make([]byte, 32) // 256-bit key
    if _, err := rand.Read(keyBytes); err != nil {
        return "", err
    }
    
    // Encode as human-readable format
    recoveryKey := base32.StdEncoding.EncodeToString(keyBytes)
    
    // Add to LUKS key slot
    cmd := exec.Command("cryptsetup", "luksAddKey", device)
    cmd.Stdin = strings.NewReader(recoveryKey)
    
    if err := cmd.Run(); err != nil {
        return "", fmt.Errorf("failed to add recovery key: %v", err)
    }
    
    return recoveryKey, nil
}

// Secure storage of recovery keys
func (i *Installer) storeRecoveryKey(key string) error {
    // Store in TPM NVRAM with authentication
    return i.trustAgent.StoreInTPMNVRAM("recovery_key", []byte(key))
}
```

## Testing Strategy

### TPM Simulation Testing

```bash
# Use TPM simulator for development testing
export TPM_SERVER_NAME=simulator
export TPM_SERVER_TYPE=raw
export TPM_INTERFACE_TYPE=socsim

# Start TPM simulator
/usr/bin/tpm_server &
TPM_PID=$!

# Run encryption tests
go test ./internal/encryption/... -tags=tpm_sim

# Cleanup
kill $TPM_PID
```

### VM Testing with Virtual TPM

```bash
# QEMU with virtual TPM
qemu-system-x86_64 \
    -enable-kvm \
    -m 4096 \
    -smp 2 \
    -drive file=piccolo-test.img,format=qcow2 \
    -netdev user,id=net0 \
    -device virtio-net,netdev=net0 \
    -tpmdev emulator,id=tpm0,chardev=chrtpm \
    -chardev socket,id=chrtpm,path=/tmp/swtpm-sock \
    -device tpm-tis,tpmdev=tpm0 \
    -nographic

# Start software TPM
mkdir -p /tmp/tpm-state
swtpm socket --tpmstate dir=/tmp/tpm-state --tpm2 --ctrl type=unixio,path=/tmp/swtpm-sock &
```

### Integration Testing

```go
func TestTPMDiskEncryption(t *testing.T) {
    // Test full encryption workflow
    testCases := []struct {
        name      string
        pcrPolicy []int
        expectSuccess bool
    }{
        {"Basic PCR 7", []int{7}, true},
        {"Advanced PCR 1+7", []int{1, 7}, true},
        {"Invalid PCR", []int{99}, false},
    }
    
    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            config := EncryptionConfig{
                Enabled:           true,
                TPMSealed:         true,
                BackupRecoveryKey: true,
                PCRPolicy:         tc.pcrPolicy,
            }
            
            err := testInstaller.setupDiskEncryption("/dev/loop0", config)
            if tc.expectSuccess && err != nil {
                t.Errorf("Expected success, got error: %v", err)
            }
            if !tc.expectSuccess && err == nil {
                t.Error("Expected failure, got success")
            }
        })
    }
}
```

## Monitoring and Diagnostics

### TPM Health Monitoring

```go
func (a *Agent) GetTPMEncryptionStatus() (*TPMEncryptionStatus, error) {
    status := &TPMEncryptionStatus{}
    
    // Check TPM device availability
    if _, err := os.Stat("/dev/tpm0"); err != nil {
        status.TPMAvailable = false
        return status, nil
    }
    status.TPMAvailable = true
    
    // Check encrypted devices
    devices, err := a.getEncryptedDevices()
    if err != nil {
        return nil, err
    }
    
    for _, device := range devices {
        deviceStatus := &EncryptedDeviceStatus{
            Device: device,
            Status: a.checkDeviceEncryptionStatus(device),
        }
        status.EncryptedDevices = append(status.EncryptedDevices, deviceStatus)
    }
    
    return status, nil
}
```

### Logging and Alerting

```go
// Structured logging for encryption events
func (a *Agent) logEncryptionEvent(event string, device string, success bool, details map[string]interface{}) {
    log.WithFields(log.Fields{
        "event": event,
        "device": device,
        "success": success,
        "details": details,
        "tpm_version": a.getTPMVersion(),
        "timestamp": time.Now(),
    }).Info("Encryption event")
}
```

## Recovery Procedures

### Hardware Failure Recovery

1. **Recovery Key Method:**
   ```bash
   # Boot from recovery media
   cryptsetup luksOpen /dev/sda2 root_recovery
   # Enter recovery key when prompted
   # Mount and access data
   ```

2. **Enterprise Key Escrow:**
   ```bash
   # Contact enterprise admin for escrowed key
   # Use enterprise recovery process
   curl -X POST https://enterprise-kms/recover \
        -H "Authorization: Bearer $ADMIN_TOKEN" \
        -d '{"device_id": "abc123", "justification": "hardware failure"}'
   ```

### TPM Reset Recovery

```go
func (a *Agent) HandleTPMReset() error {
    log.Warn("TPM reset detected, switching to recovery mode")
    
    // Check for recovery keys
    recoveryKeys, err := a.getStoredRecoveryKeys()
    if err != nil {
        return fmt.Errorf("no recovery keys available: %v", err)
    }
    
    // Prompt user for recovery key selection
    selectedKey, err := a.promptRecoveryKeySelection(recoveryKeys)
    if err != nil {
        return err
    }
    
    // Re-establish TPM-based encryption
    return a.reEstablishTPMEncryption(selectedKey)
}
```

This comprehensive TPM disk encryption implementation provides robust data protection while maintaining compatibility with Piccolo OS's update system and providing appropriate recovery mechanisms for hardware failure scenarios.