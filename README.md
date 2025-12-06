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
- SSO continuity: after signing into the portal, apps open without a second login (local proxy ports or remote listener hosts (e.g., listener.user-domain)). Third‑party apps never see the portal cookie; the proxy gates access.

## Install and Quick Start

Piccolo OS is built for x86_64 and ARM64. The easiest way to try it is in a Virtual Machine or by flashing it to a USB drive/SD card for bare metal.

### Option 1: VirtualBox (Try it now)
Perfect for testing the portal and "time-to-first-service" experience on your laptop.

1.  **Download:** [piccolo-os.x86_64-VirtualBox.vdi.xz](https://download.opensuse.org/repositories/home:/abhishekborar93:/piccolo-os:/images/home_abhishekborar93_piccolo-os_openSUSE_Tumbleweed/piccolo-os.x86_64-VirtualBox.vdi.xz)
2.  **Extract:** Unzip the file to get the `.vdi` disk image.
    ```bash
    unxz piccolo-os.x86_64-VirtualBox.vdi.xz
    ```
3.  **Create VM:**
    *   **Type:** Linux / openSUSE (64-bit).
    *   **Hardware:** 4GB RAM (Rec.), 2 vCPUs.
    *   **Disk:** "Use an Existing Virtual Hard Disk File" -> Select the extracted `.vdi`.
4.  **Configure (Critical):**
    *   **System:** Enable **EFI** (Motherboard -> Enable EFI). *Piccolo OS requires UEFI.*
    *   **Network:** Set Adapter 1 to **Bridged Adapter** (so it gets a LAN IP and is reachable).
5.  **Boot:** Start the VM. Within ~60 seconds, access the portal at `http://piccolo.local`.

### Option 2: Hardware (x86_64)
Runs directly on Intel/AMD mini-PCs, laptops, or servers.

> **Note:** This is currently a "Flash-and-Run" image. You flash it directly to your target boot drive (SSD/USB), plug that drive into your machine, and boot. An interactive installer is coming soon.

1.  **Download:** [piccolo-os.x86_64-SelfInstall.raw.xz](https://download.opensuse.org/repositories/home:/abhishekborar93:/piccolo-os:/images/home_abhishekborar93_piccolo-os_openSUSE_Tumbleweed/piccolo-os.x86_64-SelfInstall.raw.xz)
2.  **Flash:** Write the image to your SSD or USB stick using [BalenaEtcher](https://etcher.balena.io/) or `dd`.
    ```bash
    xzcat piccolo-os.x86_64-SelfInstall.raw.xz | sudo dd of=/dev/sdX bs=4M status=progress
    ```
3.  **Boot:**
    *   Insert the drive into your target machine.
    *   Power on. **UEFI Secure Boot is fully supported** and recommended.
    *   Connect Ethernet.
4.  **Setup:** Access `http://piccolo.local` from another device on the same LAN.

### Option 3: Raspberry Pi & Rock64
*ARM64 support is currently experimental and images are being finalized.*
*   **Status:** Coming Soon.

---

## Two Ways to Use
### Self‑Hosted (Free Forever)
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

## Piccolod
- [piccolod](https://github.com/AtDexters-Lab/piccolod) is the control-plane daemon for Piccolo OS. It exposes the HTTP API, manages runtime supervisors, and serves the minimal UI.

## Contribute
We’re early, scrappy, and community‑powered. PRs, issues, and design discussions are welcome.

### Build Infrastructure
Piccolo OS is built transparently on the Open Build Service (OBS).
- **RPMs (piccolod, support):** [home:abhishekborar93:piccolo-os](https://build.opensuse.org/project/show/home:abhishekborar93:piccolo-os)
- **OS Images (ISO/VDI):** [home:abhishekborar93:piccolo-os:images](https://build.opensuse.org/project/show/home:abhishekborar93:piccolo-os:images)
- **Artifacts/Downloads:** [Repository Browser](https://download.opensuse.org/repositories/home:/abhishekborar93:/piccolo-os:/images/)

### Local Development
```bash
git clone https://github.com/AtDexters-Lab/piccolo-os
cd piccolo-os
# See kiwi/ directory for image definitions and packages/ for RPM specs.
```

Join the conversation:
- 💬 GitHub Discussions: https://github.com/AtDexters-Lab/piccolo-os/discussions
- 🔗 Follow on LinkedIn: https://www.linkedin.com/company/piccolo25/

## License
Piccolo OS is free and open source under the [AGPL‑3.0](./LICENSE).
