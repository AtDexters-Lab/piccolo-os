# Piccolo OS Documentation

Piccolo OS is a privacy-first, headless container-native operating system for homelabs built on Flatcar Linux.

## Architecture

- [**Overview**](architecture/overview.md) - High-level system architecture
- [**Decisions**](architecture/decisions.md) - Architectural decisions and rationale  
- [**Layers**](architecture/layers.md) - L0/L1/L2 layer explanation
- [**Update System**](architecture/update-system.md) - OTA update architecture

## Development

- [**Building**](development/building.md) - Build instructions and procedures
- [**Testing**](development/testing.md) - Testing procedures and automation
- [**API Design**](development/api-design.md) - REST API design and patterns
- [**Contributing**](development/contributing.md) - Development guidelines

## Security

- [**TPM Encryption**](security/tpm-encryption.md) - TPM-based disk encryption
- [**Trust Model**](security/trust-model.md) - Security and trust overview
- [**Hardening**](security/hardening.md) - System hardening guide

## Operations

- [**Installation**](operations/installation.md) - Installation procedures
- [**Health Checks**](operations/health-checks.md) - Health monitoring integration
- [**Monitoring**](operations/monitoring.md) - System monitoring and observability
- [**Troubleshooting**](operations/troubleshooting.md) - Common issues and solutions

## Reference

- [**API Reference**](reference/api-reference.md) - Complete API documentation
- [**CLI Reference**](reference/cli-reference.md) - Command line tools
- [**Configuration**](reference/configuration.md) - Configuration options and files

## Quick Start

For immediate setup, see the [main README](../README.md) in the repository root.

## Project Structure

```
piccolo-os/
├── src/l0/          # Layer 0: Hardware and base OS (Flatcar build system)  
├── src/l1/          # Layer 1: Host OS and core daemon (piccolod)
├── src/l2/          # Layer 2: Applications and runtime (future)
└── docs/            # Documentation (this directory)
```