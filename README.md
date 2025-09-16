# Piccolo OS

A privacy-first, headless operating system for homelabs — built for tinkerers, self‑hosters, and anyone reclaiming control of their digital world.

> Note: Piccolo OS is in early development. This repo captures our vision, architecture, and current progress. Follow along, contribute, or roll up your sleeves and build with us.

## Why Piccolo OS
- Self-host with confidence: run services 24×7 on your own hardware.
- Local-first: fully usable on LAN with no cloud dependency.
- Open by design: Piccolo OS and remote access (Nexus) are open source.
- Secure by default: device‑terminated TLS, encrypted data, hardened base OS.
- Frictionless UX: responsive, mobile‑first portal at `http://piccolo.local`.

## Who It’s For
- Tinkerers and builders: comfortable with containers; want a stable, boring base.
- Privacy‑first users: prefer surveillance‑free tech and true data ownership.

## Vision
We believe in a user‑owned internet. Piccolo OS makes self‑hosting not just possible, but joyful.

## Core Principles
- Local‑first, cloud‑optional: everything works locally; remote access is a plug‑in.
- Immutable base: built on SUSE MicroOS (read‑only root, transactional updates, rollback).
- Container‑native: Podman + systemd; rootless by default for managed apps.
- Device‑terminated TLS: certificates and keys live on the device.
- Strong data protection: per‑directory encryption (gocryptfs‑style), password‑derived keys, optional TPM assist, and a recovery key.
- Open source: code and specs are open; contributions welcome.

## System Architecture
```
+---------------------------------------------------+
|          Layer 3: Your Applications               |
|      (Curated + custom containers)                |
+---------------------------------------------------+
|           Layer 2: System Apps                    |
| (Platform services: storage, federation, DB, etc.)|
+---------------------------------------------------+
|           Layer 1: Host OS + piccolod             |
|   (SUSE MicroOS, piccolod orchestrator/proxy)     |
+---------------------------------------------------+
|           Layer 0: Hardware                       |
|   (x86_64 PCs/mini‑PCs; Raspberry Pi 4/5;         |
|    TPM 2.0 optional but recommended)              |
+---------------------------------------------------+
```

## What You Can Do Today
- Headless operation: access the admin portal at `http://piccolo.local` (Ethernet‑only).
- One‑click app deployment: Vaultwarden, Gitea, WordPress (v1 catalog).
- Storage management: add/adopt disks; mount persistently; health surfaced.
- Encrypted volumes: per‑directory encryption with gated unlock and recovery key support.
- Updates: transactional OS updates with rollback; app updates and revert.
- Optional remote access: self‑host Nexus and publish over HTTPS via ACME HTTP‑01 (device‑terminated TLS). Piccolo Network (managed) is optional.

## Remote Access Model
- Self‑hosted Nexus (first‑class): run your own Nexus Proxy Server on a VPS. Device terminates TLS; Nexus stays L4 passthrough.
- Certificates: device issues/renews its own certs via Let’s Encrypt HTTP‑01 over the tunnel.
- Nexus server TLS: Nexus manages its own cert via ACME TLS‑ALPN‑01; it does not terminate device traffic.
- SSO continuity: after signing into the portal, apps open without a second login (local ports or remote subdomains). Third‑party apps never see the portal cookie; the proxy gates access.

## Install and Quick Start

### Requirements
- x86_64: UEFI Secure Boot‑capable PC/mini‑PC; Ethernet; 4 GB RAM recommended (2 GB minimum for light apps). TPM 2.0 optional (recommended).
- Raspberry Pi: RPi 4/5; quality SD card (USB SSD recommended for performance); Ethernet.

### x86_64 (Live USB with in‑portal “Install to Disk”)
1. Download: get the live UEFI Secure Boot `.img` from Releases.
2. Create bootable USB: use BalenaEtcher or `dd`.
3. Boot: enable Secure Boot; boot from USB. Within ~60s, `http://piccolo.local` shows the portal.
4. Install: in the portal, choose “Install to Disk”, review target disk contents, type‑to‑confirm, and install. The installer writes the image, grows partitions/FS, and creates data subvolumes. Reboot to the installed system.

### Raspberry Pi (SD Image)
1. Download: get the Pi SD image from Releases.
2. Flash: write to SD card; insert and power on.
3. Access: open `http://piccolo.local` within ~60s and complete setup.
4. Note: no in‑portal install/migration in v1 (SD image is the install medium).

## Two Ways to Use
### Self‑Hosted (Free Forever)
- Compile from source.
- Run your own [Nexus Proxy](https://github.com/AtDexters-Lab/nexus-proxy-server).
- Control every service, every update, every byte.

### Piccolo Network (Optional Subscription)
- Managed remote access and services.
- Federated encrypted storage (planned).
- Hassle‑free remote updates.

## Curated Apps (v1)
- Vaultwarden — lightweight password manager (< 5 minutes to first page).
- Gitea — lightweight Git service (SQLite default; < 5 minutes).
- WordPress — personal website/blog (with MariaDB; < 10 minutes).

## Roadmap (Selected)
- Core OS pre‑beta for self‑hosters (curated apps, storage, remote publish).
- Acceptance suite aligned to product features (Gherkin + OpenAPI).
- System apps (federated storage) alpha.
- Piccolo Network (optional managed remote access and services).

## Documentation
Comprehensive documentation is available in the `/docs` directory:
- **[Architecture](docs/architecture/)** — system design and layers
- **[Development](docs/development/)** — building, testing, and contributing
- **[Security](docs/security/)** — encryption and trust model
- **[Operations](docs/operations/)** — installation and system management
- **Admin API**: `docs/api/openapi.yaml` (draft)
- **App platform spec**: `docs/app-platform/specification.yaml`

## Contribute
We’re early, scrappy, and community‑powered. PRs, issues, and design discussions are welcome.

```bash
git clone https://github.com/AtDexters-Lab/piccolo-os
cd piccolo-os
# See docs/development/ for build instructions
```

Join the conversation:
- 💬 GitHub Discussions: https://github.com/AtDexters-Lab/piccolo-os/discussions
- 🔗 Follow on LinkedIn: https://www.linkedin.com/company/piccolo25/

## License
Piccolo OS is free and open source under the [AGPL‑3.0](./LICENSE).
