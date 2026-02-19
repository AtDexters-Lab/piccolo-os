# Piccolo OS ![Stage: Alpha](https://img.shields.io/badge/Stage-Alpha-orange)

A privacy-first, headless operating system for homelabs — built for tinkerers, self‑hosters, and anyone reclaiming control of their digital world.

> **Note:** Piccolo OS is in early development. This repo captures our vision, architecture, and current progress. Follow along, contribute, or roll up your sleeves and build with us.

---

## 📖 Table of Contents
- [Install and Quick Start](#-install-and-quick-start)
- [Why Piccolo OS](#-why-piccolo-os)
- [Architecture](#-system-architecture)
- [Contribute](#-contribute)

---

## 🚀 Install and Quick Start

Piccolo OS is built for **x86_64** and **ARM64**. The easiest way to try it is in a Virtual Machine or by flashing it to a USB drive/SD card for bare metal.

> **💡 Tip: Faster Downloads**
> Browser downloads can be slow for large OS images. We recommend using a download manager like `aria2c` with multiple streams.
> ```bash
> # Example: Download with 16 connections
> aria2c -x 16 <image-url>
> ```

### Option 1: VirtualBox (Try it now)
*Perfect for testing the portal and "time-to-first-service" experience on your laptop.*

1.  **Download:** [piccolo-os.x86_64-VirtualBox.vdi.xz](https://download.opensuse.org/repositories/home:/atdexterslab:/piccolo-os/home_atdexterslab_atdexterslab_tumbleweed/piccolo-os.x86_64-VirtualBox.vdi.xz)

2.  **Extract:** Unzip the file to get the `.vdi` disk image.
    ```bash
    unxz piccolo-os.x86_64-VirtualBox.vdi.xz
    ```

3.  **Resize Disk (Mandatory):** The VDI images are compact. You **must** resize them to at least 24GB before attaching, otherwise the OS will fail to boot.
    ```bash
    # Example: Resize to 24GB (24 * 1024 = 24576 MB)
    VBoxManage modifymedium disk piccolo-os.x86_64-VirtualBox.vdi --resize 24576
    ```

4.  **Create VM:**
    *   **Type:** Linux / openSUSE (64-bit).
    *   **Hardware:** 4GB RAM (Rec.), 2 vCPUs.
    *   **Disk:** "Use an Existing Virtual Hard Disk File" -> Select the extracted `.vdi`.

5.  **Configure (Critical):**
    *   **System:** Enable **EFI** (Motherboard -> Enable EFI). *Piccolo OS requires UEFI.*
    *   **Network:** Set Adapter 1 to **Bridged Adapter** (so it gets a LAN IP and is reachable).

6.  **Boot:** Start the VM. Within ~60 seconds, access the portal at `http://piccolo.local`.

### Option 2: Hardware (x86_64 & ARM64)
*Runs directly on generic x86_64 (Intel/AMD) and ARM64 hardware (UEFI).*

#### Method A: Installer ISO (Recommended for Installation)
Use this if you want to install Piccolo OS to your computer's **internal drive**.

**Downloads:**
*   **x86_64 (Intel/AMD):** [piccolo-os.x86_64-SelfInstall.iso](https://download.opensuse.org/repositories/home:/atdexterslab:/piccolo-os/home_atdexterslab_atdexterslab_tumbleweed/iso/piccolo-os.x86_64-SelfInstall.iso)
*   **ARM64 (Generic):** [piccolo-os.aarch64-SelfInstall.iso](https://download.opensuse.org/repositories/home:/atdexterslab:/piccolo-os/home_atdexterslab_atdexterslab_tumbleweed/iso/piccolo-os.aarch64-SelfInstall.iso)

1.  **Burn:** Write the ISO to a USB stick using [BalenaEtcher](https://etcher.balena.io/), Rufus, or `dd`.
2.  **Boot:** Insert the USB drive, boot from it, and follow the on-screen prompts to install to your hard drive.

#### Method B: Raw Image (Live USB or Direct Flash)
Use this to **"Try Now"** from a USB stick without touching your internal drive, or to flash directly to a drive.

**Downloads:**
*   **x86_64 (Intel/AMD):** [piccolo-os.x86_64-SelfInstall.raw.xz](https://download.opensuse.org/repositories/home:/atdexterslab:/piccolo-os/home_atdexterslab_atdexterslab_tumbleweed/piccolo-os.x86_64-SelfInstall.raw.xz)
*   **ARM64 (Generic):** [piccolo-os.aarch64-SelfInstall.raw.xz](https://download.opensuse.org/repositories/home:/atdexterslab:/piccolo-os/home_atdexterslab_atdexterslab_tumbleweed/piccolo-os.aarch64-SelfInstall.raw.xz)

1.  **Flash:** Write the image to a USB stick (for Live/Try Now) or directly to an SSD (for Install) using [BalenaEtcher](https://etcher.balena.io/) or `dd`.
    ```bash
    # Example for x86_64
    xzcat piccolo-os.x86_64-SelfInstall.raw.xz | sudo dd of=/dev/sdX bs=4M status=progress
    ```

2.  **Boot:**
    *   Insert the drive into your machine.
    *   Power on. **UEFI Secure Boot is fully supported** and recommended.
    *   Connect Ethernet.

3.  **Setup:** Access `http://piccolo.local` from another device on the same LAN.

### Option 3: ARM64 (Raspberry Pi & Rock64)
*Board-specific optimized images (bootloader/firmware pre-configured).*

*   **Raspberry Pi (3+/4/5):** [piccolo-os.aarch64-RaspberryPi.raw.xz](https://download.opensuse.org/repositories/home:/atdexterslab:/piccolo-os/home_atdexterslab_atdexterslab_tumbleweed/piccolo-os.aarch64-RaspberryPi.raw.xz)
*   **Rock64:** [piccolo-os.aarch64-Rock64.raw.xz](https://download.opensuse.org/repositories/home:/atdexterslab:/piccolo-os/home_atdexterslab_atdexterslab_tumbleweed/piccolo-os.aarch64-Rock64.raw.xz)

Follow the **Method B (Raw Image)** instructions from Option 2. Ensure your board is connected to Ethernet.

---

## ❓ Why Piccolo OS
- **Self-host with confidence:** Run services 24×7 on your own hardware.
- **Local-first:** Fully usable on LAN with no cloud dependency.
- **Open by design:** Piccolo OS and remote access (Nexus) are open source.
- **Secure by default:** Device‑terminated TLS, encrypted data, hardened base OS.

### Who It’s For
- **Tinkerers and builders:** Comfortable with containers; want a stable, boring base.
- **Privacy‑first users:** Prefer surveillance‑free tech and true data ownership.

### Vision
We believe in a user‑owned internet. Piccolo OS makes self‑hosting not just possible, but joyful.

### Core Principles
- **Local‑first, cloud‑optional:** Everything works locally; remote access is a plug‑in.
- **Immutable base:** Built on SUSE MicroOS (read‑only root, transactional updates, rollback).
- **Container‑native:** Podman + systemd; rootless by default for managed apps.
- **Device‑terminated TLS:** Certificates and keys live on the device.
- **Strong data protection:** Per‑directory encryption (gocryptfs‑style), password‑derived keys, optional TPM assist, and a recovery key.
- **Open source:** Code and specs are open; contributions welcome.

---

## 🏗 System Architecture
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

### What You Can Do Today
- **Headless operation:** Access the admin portal at `http://piccolo.local` (Ethernet‑only).
- **One‑click app deployment:** Vaultwarden, Gitea, WordPress (v1 catalog).
- **Encrypted volumes:** Per‑directory encryption with gated unlock and recovery key support.
- **Updates:** Transactional OS updates with rollback; app updates and revert.
- **Optional remote access:** Self‑host Nexus and publish over HTTPS via ACME HTTP‑01 (device‑terminated TLS). Piccolo Network (managed) is optional.

### Remote Access Model
- **Self‑hosted Nexus (first‑class):** Run your own Nexus Proxy Server on a VPS. Device terminates TLS; Nexus stays L4 passthrough.
- **Certificates:** Device issues/renews its own certs via Let’s Encrypt HTTP‑01 over the tunnel.
- **Nexus server TLS:** Nexus manages its own cert via ACME TLS‑ALPN‑01; it does not terminate device traffic.
- **SSO continuity:** After signing into the portal, apps open without a second login (local proxy ports or remote listener hosts). Third‑party apps never see the portal cookie; the proxy gates access.

---

## 🛠 Two Ways to Use
### 1. Self‑Hosted (Free Forever)
- Run your own [Nexus Proxy](https://github.com/AtDexters-Lab/nexus-proxy-server).
- Control every service, every update, every byte.

### 2. Piccolo Network (Optional Subscription)
- Managed remote access and services.
- Federated encrypted storage (planned).
- Hassle‑free remote updates.

---

## 📦 Curated Apps (v1)
- **Vaultwarden:** Lightweight password manager (< 5 minutes to first page).
- **Gitea:** Lightweight Git service (SQLite default; < 5 minutes).
- **WordPress:** Personal website/blog (with MariaDB; < 10 minutes).

## ⚙️ Piccolod
- [piccolod](https://github.com/AtDexters-Lab/piccolod) is the control-plane daemon for Piccolo OS. It exposes the HTTP API, manages runtime supervisors, and serves the minimal UI.

---

## 🤝 Contribute
We’re early, scrappy, and community‑powered. PRs, issues, and design discussions are welcome.

### Build Infrastructure
Piccolo OS is built transparently on the Open Build Service (OBS).
- **RPMs (piccolod, support):** [home:atdexterslab](https://build.opensuse.org/project/show/home:atdexterslab)
- **OS Images (ISO/VDI):** [home:atdexterslab:piccolo-os/images](https://build.opensuse.org/package/show/home:atdexterslab:piccolo-os/images)
- **Artifacts/Downloads:** [Repository Browser](https://download.opensuse.org/repositories/home:/atdexterslab:/piccolo-os/home_atdexterslab_atdexterslab_tumbleweed/)

### Local Development
```bash
git clone https://github.com/AtDexters-Lab/piccolo-os
cd piccolo-os
# See kiwi/ directory for image definitions and packages/ for RPM specs.
```

### Join the conversation
- 💬 [GitHub Discussions](https://github.com/AtDexters-Lab/piccolo-os/discussions)
- 🔗 [Follow on LinkedIn](https://www.linkedin.com/company/piccolo25/)

## 📜 License
Piccolo OS is free and open source under the [AGPL‑3.0](./LICENSE).
