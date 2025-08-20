# UEFI ISO Creation Tools

This directory contains tools for creating UEFI bootable ISOs with TPM measured boot support for Piccolo OS.

## Files

- **`create_piccolo_iso.sh`** - Main ISO creation script (1100+ lines with embedded docs)
- **`test_piccolo_iso.sh`** - Comprehensive TDD testing framework  
- **`qemu-boot.sh`** - QEMU boot testing utility
- **`README.md`** - This documentation file

## Usage

### Create UEFI ISO
```bash
./create_piccolo_iso.sh \
  --source /path/to/flatcar_production_image.bin \
  --output piccolo-os-uefi.iso \
  --version 1.0.0 \
  --update-group piccolo-stable
```

### Run Tests
```bash
./test_piccolo_iso.sh              # Full test suite
./test_piccolo_iso.sh --quick       # Quick tests only
./test_piccolo_iso.sh --boot-only   # Boot tests only
```

## Features

- ✅ UEFI + BIOS hybrid bootable ISOs
- ✅ TPM measured boot support
- ✅ Comprehensive validation framework
- ✅ Self-documenting with embedded architecture docs
- ✅ Standalone operation (no SDK container dependency)

## Background

These tools were developed to address critical issues in the original ISO creation process:
- Fixed empty cpio.gz (was 512B, now proper size)
- Fixed empty EFI boot directories
- Fixed wrong partition mounting (p9 → p3)
- Fixed xorriso parameters for proper UEFI boot discovery

See `create_piccolo_iso.sh` header for detailed architecture documentation.
