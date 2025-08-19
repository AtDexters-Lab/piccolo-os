# Piccolo OS UEFI + Secure Boot + Measured Boot Strategy

**Document Version**: 1.0  
**Date**: 2025-01-19  
**Authors**: Claude Code Research  
**Status**: Implementation Ready

---

## Executive Summary

This document outlines the comprehensive strategy for implementing UEFI, Secure Boot, and Measured Boot capabilities in Piccolo OS. Our research reveals that **measured boot operates independently of Secure Boot**, enabling a two-phase approach that delivers immediate security benefits while building toward complete boot chain control.

### Key Strategic Insights

1. **Measured Boot ≠ Secure Boot**: TPM-based attestation works without external signature verification
2. **Custom Keys > External Signing**: Factory-provisioned Piccolo keys provide superior security
3. **No External Dependencies**: Complete control over timeline and trust infrastructure
4. **Immediate Implementation Possible**: Leverage Flatcar's recent GRUB 2.12-flatcar3 updates

---

## Background & Research Findings

### Current Flatcar Status (January 2025)

**Recent Progress**:
- ✅ GRUB 2.12-flatcar3 with Red Hat Secure Boot patches (Nov 2024)
- ✅ Shim 15.8 updated with SBAT support (Jan 2024)
- ✅ `qemu_uefi_secure` testing format available
- ⚠️ Still waiting for shim review approval from Microsoft

**Technical Debt**:
- Current ISO uses `mkisofs` which lacks UEFI support
- No hybrid BIOS+UEFI ISO capability
- Secure Boot blocked by external signing dependencies

### Security Architecture Analysis

#### Measured Boot (TPM-based)
- **Purpose**: Cryptographic evidence of boot state integrity
- **Implementation**: TPM PCR registers store boot component hashes
- **Independence**: Works without UEFI Secure Boot enabled
- **Benefits**: Remote attestation, PCR sealing, tamper detection

#### Secure Boot (Signature-based)
- **Purpose**: Prevent execution of unsigned/malicious code
- **Implementation**: Cryptographic signature verification chain
- **Dependency**: Requires signed bootloader and kernel
- **Limitation**: Traditionally depends on external CAs (Microsoft)

#### Combined Benefits
- **Defense in Depth**: Complementary security mechanisms
- **Enterprise Ready**: Meets compliance and attestation requirements
- **Custom Control**: Own the entire trust infrastructure

---

## Implementation Strategy

### Phase 1: UEFI + TPM Measured Boot (Immediate - 2-4 weeks)

**Objective**: Enable UEFI booting with full TPM measurement capabilities

#### 1.1 Replace mkisofs with xorriso

**Problem**: Current `vm_image_util.sh:784` uses mkisofs which cannot create UEFI-bootable ISOs
```bash
# Current broken approach
mkisofs -v -l -r -J -o $2 -b isolinux/isolinux.bin -c isolinux/boot.cat -no-emul-boot -boot-load-size 4 -boot-info-table .
```

**Solution**: Implement hybrid BIOS+UEFI ISO creation
```bash
_write_iso_disk() {
    local base_dir="${VM_TMP_ROOT}/usr"
    local iso_target="${VM_TMP_DIR}/rootiso"
    
    mkdir "${iso_target}"
    pushd "${iso_target}" >/dev/null
    
    # Create directory structure for both BIOS and UEFI
    mkdir -p isolinux syslinux flatcar EFI/boot boot/grub
    
    # Prepare common boot files
    _write_cpio_common "$1" "${iso_target}/flatcar/cpio.gz"
    cp "${VM_TMP_ROOT}"/boot/flatcar/vmlinuz-a "${iso_target}/flatcar/vmlinuz"
    
    # BIOS boot setup (maintain compatibility)
    cp -R /usr/share/syslinux/* isolinux/
    cat <<EOF > isolinux/isolinux.cfg
INCLUDE /syslinux/syslinux.cfg
EOF
    
    cat <<EOF > syslinux/syslinux.cfg
default flatcar
prompt 1
timeout 15

label flatcar
  menu default
  kernel /flatcar/vmlinuz
  append initrd=/flatcar/cpio.gz flatcar.autologin
EOF

    # UEFI boot setup - leverage existing Flatcar GRUB components
    _create_efi_image "${iso_target}"
    
    # Create hybrid BIOS+UEFI ISO using xorriso
    xorriso -as mkisofs \
        -V "Piccolo-OS" \
        -o "$2" \
        -r -J -joliet-long -cache-inodes \
        -isohybrid-mbr /usr/lib/ISOLINUX/isohdpfx.bin \
        -b isolinux/isolinux.bin \
        -c isolinux/boot.cat \
        -boot-load-size 4 \
        -boot-info-table \
        -no-emul-boot \
        -eltorito-alt-boot \
        -e --interval:appended_partition_2:all:: \
        -append_partition 2 0xef "${iso_target}/efi.img" \
        -no-emul-boot \
        -isohybrid-gpt-basdat \
        .
    
    popd >/dev/null
}

_create_efi_image() {
    local iso_target="$1"
    local efi_img="${iso_target}/efi.img"
    local efi_mount="${iso_target}/efi_mount"
    
    # Create 32MB FAT32 EFI System Partition
    dd if=/dev/zero of="${efi_img}" bs=1M count=32
    mkfs.vfat -F 32 "${efi_img}"
    
    # Mount and populate EFI image
    mkdir "${efi_mount}"
    sudo mount -o loop "${efi_img}" "${efi_mount}"
    
    # Create EFI directory structure
    sudo mkdir -p "${efi_mount}/EFI/boot"
    sudo mkdir -p "${efi_mount}/boot/grub"
    
    # Use existing Flatcar GRUB EFI binary (already available)
    if [[ -f "${VM_TMP_ROOT}/boot/EFI/boot/grubx64.efi" ]]; then
        sudo cp "${VM_TMP_ROOT}/boot/EFI/boot/grubx64.efi" \
                "${efi_mount}/EFI/boot/bootx64.efi"
    fi
    
    # Create GRUB configuration for ISO boot with TPM measurement
    sudo tee "${efi_mount}/boot/grub/grub.cfg" <<EOF
# Load TPM module for measured boot
insmod tpm
insmod all_video

set default="piccolo"
set timeout=1

menuentry "Piccolo OS Live (Measured Boot)" --id=piccolo {
    # TPM measurement happens automatically when tpm module loaded
    linux /flatcar/vmlinuz flatcar.autologin root=live:CDLABEL=Piccolo-OS
    initrd /flatcar/cpio.gz
}
EOF
    
    sudo umount "${efi_mount}"
    rmdir "${efi_mount}"
}
```

#### 1.2 TPM Measured Boot Integration

**Enable GRUB TPM Measurement**:
```bash
# PCR Usage Strategy:
# PCR 8: Boot components (bootloader, kernel, initrd)
# PCR 9: GRUB stages and modules  
# PCR 11: Command line arguments and configuration

# GRUB automatically measures when tpm module loaded
# Measurements include:
# - GRUB binary itself
# - All loaded modules
# - Kernel and initrd
# - Command line parameters
```

**PCR Sealing Implementation**:
```bash
# Seal secrets to known-good boot state
tpm2_createprimary -c primary.ctx
tpm2_policypcr -l sha256:8,9,11 -f pcr.policy
tpm2_create -g sha256 -G aes128 -c primary.ctx -L pcr.policy \
    -r sealed.priv -u sealed.pub -i secret.txt
tpm2_load -c primary.ctx -r sealed.priv -u sealed.pub -c sealed.ctx

# Unseal only if PCRs match expected boot state
tpm2_unseal -c sealed.ctx -p pcr:sha256:8,9,11
```

#### 1.3 Build System Updates

**Update Dependencies** (`src/l0/build_piccolo.sh`):
```bash
check_dependencies() {
    log "Checking for required dependencies..."
    local deps=("git" "docker" "gpg" "numfmt" "stat" "xorriso" "mkfs.vfat" "tpm2-tools")
    
    for dep in "${deps[@]}"; do
        if ! command -v "${dep}" >/dev/null 2>&1; then
            log "ERROR: Required dependency '${dep}' not found"
            return 1
        fi
    done
    
    log "All dependencies satisfied"
}
```

#### 1.4 Testing Framework

**Create comprehensive UEFI testing** (`src/l0/test_uefi_measured_boot.sh`):
```bash
#!/bin/bash
set -euo pipefail

test_uefi_boot() {
    local iso_path="$1"
    local test_dir=$(mktemp -d)
    
    echo "Testing UEFI boot with TPM measurement for ISO: ${iso_path}"
    
    # Test UEFI boot with QEMU + TPM emulation
    timeout 120 qemu-system-x86_64 \
        -enable-kvm \
        -m 2048 \
        -boot d \
        -cdrom "${iso_path}" \
        -bios /usr/share/ovmf/OVMF.fd \
        -netdev user,id=net0 \
        -device e1000,netdev=net0 \
        -chardev socket,id=chrtpm,path=/tmp/tpm.sock \
        -tpmdev emulator,id=tpm0,chardev=chrtpm \
        -device tpm-tis,tpmdev=tpm0 \
        -nographic \
        -serial stdio \
        -monitor none \
        < /dev/null | tee "${test_dir}/uefi_boot.log"
    
    # Verify boot success and TPM measurement
    if grep -q "flatcar.autologin" "${test_dir}/uefi_boot.log" && \
       grep -q "TPM.*measurement" "${test_dir}/uefi_boot.log"; then
        echo "✅ UEFI boot with TPM measurement test passed"
        return 0
    else
        echo "❌ UEFI boot test failed"
        cat "${test_dir}/uefi_boot.log"
        return 1
    fi
}

test_bios_compatibility() {
    local iso_path="$1"
    echo "Testing BIOS compatibility..."
    
    # Ensure BIOS boot still works
    timeout 60 qemu-system-x86_64 \
        -enable-kvm -m 1024 -boot d -cdrom "${iso_path}" \
        -nographic -serial stdio -monitor none < /dev/null | \
        grep -q "flatcar.autologin"
    
    echo "✅ BIOS compatibility maintained"
}

main() {
    local iso_path="$1"
    
    echo "🔄 Starting comprehensive UEFI + Measured Boot testing"
    
    test_bios_compatibility "${iso_path}"
    test_uefi_boot "${iso_path}"
    
    echo "✅ All tests passed - ISO ready for production"
}

main "$@"
```

### Phase 2: Custom Secure Boot for Piccolo Devices (3-6 months)

**Objective**: Complete boot chain control with factory-provisioned custom keys

#### 2.1 Piccolo Certificate Authority Infrastructure

**CA Hierarchy Design**:
```bash
# Root CA (Air-gapped, offline storage)
openssl genrsa -aes256 -out piccolo-root-ca.key 4096
openssl req -new -x509 -key piccolo-root-ca.key -sha256 -days 7300 \
    -out piccolo-root-ca.crt \
    -subj "/C=US/ST=CA/L=San Francisco/O=Piccolo Systems Inc/CN=Piccolo Root CA"

# Intermediate CA (Online signing)
openssl genrsa -out piccolo-intermediate-ca.key 2048
openssl req -new -key piccolo-intermediate-ca.key \
    -out piccolo-intermediate-ca.csr \
    -subj "/C=US/ST=CA/L=San Francisco/O=Piccolo Systems Inc/CN=Piccolo Intermediate CA"

openssl x509 -req -in piccolo-intermediate-ca.csr \
    -CA piccolo-root-ca.crt -CAkey piccolo-root-ca.key \
    -out piccolo-intermediate-ca.crt -days 3650 -sha256

# Code Signing Certificate  
openssl genrsa -out piccolo-code-sign.key 2048
openssl req -new -key piccolo-code-sign.key \
    -out piccolo-code-sign.csr \
    -subj "/C=US/ST=CA/L=San Francisco/O=Piccolo Systems Inc/CN=Piccolo OS Code Signing"

openssl x509 -req -in piccolo-code-sign.csr \
    -CA piccolo-intermediate-ca.crt -CAkey piccolo-intermediate-ca.key \
    -out piccolo-code-sign.crt -days 1095 -sha256
```

#### 2.2 Component Signing Pipeline

**Automated Signing System**:
```bash
#!/bin/bash
# sign-piccolo-components.sh

sign_bootloader() {
    local grub_binary="$1"
    local output_path="$2"
    
    sbsign --key piccolo-code-sign.key \
           --cert piccolo-code-sign.crt \
           --output "${output_path}" \
           "${grub_binary}"
    
    echo "✅ Signed bootloader: ${output_path}"
}

sign_kernel() {
    local kernel_path="$1" 
    local output_path="$2"
    
    sbsign --key piccolo-code-sign.key \
           --cert piccolo-code-sign.crt \
           --output "${output_path}" \
           "${kernel_path}"
           
    echo "✅ Signed kernel: ${output_path}"
}

sign_modules() {
    local modules_dir="$1"
    
    find "${modules_dir}" -name "*.ko" | while read -r module; do
        /usr/src/linux/scripts/sign-file \
            sha512 piccolo-code-sign.key piccolo-code-sign.crt "${module}"
    done
    
    echo "✅ Signed all kernel modules in: ${modules_dir}"
}

main() {
    echo "🔏 Starting Piccolo OS component signing process"
    
    sign_bootloader grubx64.efi grubx64-signed.efi
    sign_kernel vmlinuz vmlinuz-signed  
    sign_modules /lib/modules/$(uname -r)/
    
    echo "✅ All components signed successfully"
}

main "$@"
```

#### 2.3 Factory Key Provisioning

**UEFI Key Management**:
```bash
#!/bin/bash
# provision-piccolo-keys.sh - Factory provisioning script

provision_secure_boot_keys() {
    local device_efi_vars="$1"
    
    # Convert certificates to UEFI format
    cert-to-efi-sig-list -g $(uuidgen) piccolo-root-ca.crt piccolo-pk.esl
    cert-to-efi-sig-list -g $(uuidgen) piccolo-intermediate-ca.crt piccolo-kek.esl  
    cert-to-efi-sig-list -g $(uuidgen) piccolo-code-sign.crt piccolo-db.esl
    
    # Create signed updates (self-signed with PK)
    sign-efi-sig-list -k piccolo-root-ca.key -c piccolo-root-ca.crt \
        PK piccolo-pk.esl piccolo-pk.auth
    sign-efi-sig-list -k piccolo-root-ca.key -c piccolo-root-ca.crt \  
        KEK piccolo-kek.esl piccolo-kek.auth
    sign-efi-sig-list -k piccolo-intermediate-ca.key -c piccolo-intermediate-ca.crt \
        db piccolo-db.esl piccolo-db.auth
    
    # Provision keys to UEFI firmware
    efi-updatevar -f piccolo-pk.auth PK
    efi-updatevar -f piccolo-kek.auth KEK  
    efi-updatevar -f piccolo-db.auth db
    
    echo "✅ Piccolo Secure Boot keys provisioned to device"
}

setup_development_mode() {
    echo "⚠️  Setting up development key enrollment..."
    
    # Generate MOK for development 
    openssl req -newkey rsa:2048 -nodes -keyout MOK-dev.key \
        -new -x509 -sha256 -days 365 \
        -subj "/CN=Piccolo OS Development Key" \
        -out MOK-dev.crt
        
    # Convert to DER for mokutil
    openssl x509 -in MOK-dev.crt -outform DER -out MOK-dev.der
    
    echo "Development key created. Enroll with:"
    echo "  mokutil --import MOK-dev.der"
}

main() {
    case "$1" in
        "factory")
            provision_secure_boot_keys "$2"
            ;;
        "development")
            setup_development_mode
            ;;
        *)
            echo "Usage: $0 {factory|development} [device_path]"
            exit 1
            ;;
    esac
}

main "$@"
```

#### 2.4 Custom Secure Boot ISO Creation

**Enhanced ISO Builder with Signing**:
```bash
_create_secure_boot_efi_image() {
    local iso_target="$1"
    local efi_img="${iso_target}/efi.img"
    local efi_mount="${iso_target}/efi_mount"
    
    # Create larger EFI System Partition for signed components
    dd if=/dev/zero of="${efi_img}" bs=1M count=64
    mkfs.vfat -F 32 "${efi_img}"
    
    # Mount and populate
    mkdir "${efi_mount}"
    sudo mount -o loop "${efi_img}" "${efi_mount}"
    
    # Create directory structure
    sudo mkdir -p "${efi_mount}/EFI/boot"
    sudo mkdir -p "${efi_mount}/EFI/piccolo"
    sudo mkdir -p "${efi_mount}/boot/grub"
    
    # Copy signed GRUB bootloader
    sudo cp grubx64-signed.efi "${efi_mount}/EFI/boot/bootx64.efi"
    sudo cp grubx64-signed.efi "${efi_mount}/EFI/piccolo/grubx64.efi"
    
    # Copy Piccolo certificates for verification
    sudo cp piccolo-*.crt "${efi_mount}/EFI/piccolo/"
    
    # Create enhanced GRUB config with signature verification
    sudo tee "${efi_mount}/boot/grub/grub.cfg" <<EOF
# Piccolo OS Secure Boot Configuration
# Load security modules
insmod tpm
insmod pgp  
insmod verify
insmod all_video

# Set verification policy
set check_signatures=enforce
trust /EFI/piccolo/piccolo-code-sign.crt

set default="piccolo-secure"
set timeout=3

menuentry "Piccolo OS Live (Secure Boot + Measured Boot)" --id=piccolo-secure {
    # Verify and load signed kernel
    linux /flatcar/vmlinuz-signed flatcar.autologin root=live:CDLABEL=Piccolo-OS
    initrd /flatcar/cpio.gz
}

menuentry "Piccolo OS Live (Development Mode)" --id=piccolo-dev {
    # Disable signature checking for development
    set check_signatures=no
    linux /flatcar/vmlinuz flatcar.autologin root=live:CDLABEL=Piccolo-OS
    initrd /flatcar/cpio.gz
}
EOF
    
    sudo umount "${efi_mount}"
    rmdir "${efi_mount}"
}
```

---

## Security Benefits & Attestation

### TPM Measured Boot Capabilities

**Local Attestation**:
- **PCR Sealing**: Bind encryption keys to known-good boot state
- **Tamper Detection**: Detect unauthorized boot chain modifications  
- **Rollback Protection**: Prevent downgrades to vulnerable versions

**Remote Attestation**:
- **Quote Generation**: Cryptographic proof of boot state
- **Attestation Protocols**: Integration with enterprise security systems
- **Compliance Reporting**: Automated security posture verification

### Custom Secure Boot Advantages  

**Complete Trust Control**:
- **No Microsoft Dependency**: Independent of external CA trust
- **Supply Chain Security**: Control entire key provisioning process
- **Incident Response**: Immediate revocation and key rotation capability

**Enterprise Features**:
- **Custom PKI Integration**: Compatible with enterprise certificate infrastructure  
- **Audit Logging**: Complete boot chain verification records
- **Policy Enforcement**: Granular control over allowed boot components

---

## Implementation Timeline

### Phase 1: UEFI + TPM Measured Boot (Weeks 1-4)

**Week 1-2: Core Implementation**
- [ ] Replace mkisofs with xorriso in `vm_image_util.sh`
- [ ] Implement `_create_efi_image()` function
- [ ] Update build dependency checking
- [ ] Enable GRUB TPM module loading

**Week 3-4: Testing & Validation**  
- [ ] Create comprehensive test suite
- [ ] Validate BIOS compatibility preservation
- [ ] Test UEFI boot on various firmware versions
- [ ] Implement PCR sealing for development

### Phase 2: Custom Secure Boot (Months 2-6)

**Month 2: Infrastructure Development**
- [ ] Design Piccolo CA hierarchy
- [ ] Implement signing automation
- [ ] Create key provisioning tools
- [ ] Develop factory integration process

**Month 3-4: Implementation & Testing**
- [ ] Build custom Secure Boot ISO creation
- [ ] Implement signature verification 
- [ ] Test with development hardware
- [ ] Create deployment procedures

**Month 5-6: Production Readiness**
- [ ] Factory provisioning integration
- [ ] Production signing pipeline
- [ ] Customer key management tools
- [ ] Documentation and training

---

## Risk Assessment & Mitigation

### Technical Risks

| Risk | Impact | Likelihood | Mitigation |
|------|--------|------------|------------|
| UEFI firmware compatibility | Medium | Low | Extensive testing matrix |
| TPM measurement inconsistency | High | Medium | Robust PCR policy design |
| Signing pipeline compromise | High | Low | Air-gapped CA, HSM integration |
| Build complexity increase | Low | Medium | Comprehensive automation |

### Business Risks

| Risk | Impact | Likelihood | Mitigation |
|------|--------|------------|------------|
| Timeline delays | Medium | Medium | Phased approach, MVP focus |
| Customer key management | Medium | High | Automated tools, documentation |
| Support complexity | Medium | Medium | Training, escalation procedures |
| Regulatory compliance | High | Low | Standards alignment, audit prep |

---

## Success Criteria

### Phase 1 Acceptance Criteria
- [ ] ISO boots successfully on BIOS-only systems (compatibility preserved)
- [ ] ISO boots successfully on UEFI-only systems (new capability)  
- [ ] ISO boots successfully on hybrid BIOS+UEFI systems (maximum compatibility)
- [ ] TPM measurements recorded in PCRs 8, 9, 11
- [ ] PCR sealing and unsealing functional
- [ ] Build time increase <20%
- [ ] All existing Piccolo OS functionality intact

### Phase 2 Acceptance Criteria  
- [ ] Custom Secure Boot keys provisioned in factory
- [ ] Signed components boot successfully with Secure Boot enabled
- [ ] Custom CA hierarchy operational with proper security controls
- [ ] Key rotation and revocation procedures validated
- [ ] Enterprise attestation integration functional
- [ ] Customer key management tools deployed
- [ ] Complete documentation and training materials

---

## Long-term Vision

### Piccolo Security Ecosystem

**Complete Boot Chain Control**:
- Factory-provisioned custom Secure Boot keys
- TPM-based measured boot and attestation  
- Runtime integrity monitoring (IMA/EVM)
- Encrypted storage with PCR-sealed keys

**Enterprise Integration**:
- SCEP/EST certificate enrollment
- SIEM integration for attestation events
- Compliance reporting automation
- Zero-touch provisioning

**Market Differentiation**:
- "The only OS with custom Secure Boot"
- Complete trust infrastructure ownership
- No external CA dependencies
- Enterprise-grade security out of the box

---

## Conclusion

This strategy provides Piccolo OS with industry-leading boot security through a practical, implementable approach:

1. **Immediate Value**: TPM measured boot delivers core security promise without external dependencies
2. **Long-term Control**: Custom Secure Boot provides complete trust infrastructure ownership  
3. **Market Advantage**: Differentiated security posture unavailable in other container-optimized OSes
4. **Enterprise Ready**: Meets compliance and attestation requirements for regulated industries

The phased approach minimizes risk while maximizing time-to-market for critical security capabilities. Phase 1 can be completed in 4 weeks, providing immediate competitive advantage, while Phase 2 establishes long-term market leadership in boot security.

**Recommendation**: Begin Phase 1 implementation immediately while planning Phase 2 infrastructure development.