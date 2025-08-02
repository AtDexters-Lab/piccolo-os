<h1 align="center">Piccolo OS</h1>

<p align="center">
  A privacy-first, headless OS for homelabs—built for tinkerers, self-hosters, and anyone reclaiming control over their digital world.
</p>

> 🚧 **Note:** Piccolo OS is in early development. This repo captures our vision, architecture, and current progress. Follow along, contribute, or roll up your sleeves and build with us.

---

## 🧠 Why Piccolo OS?

**Piccolo OS** is a container-native operating system for those who want to **self-host apps, own their data, and escape the walled gardens of Big Tech**.

It's designed to:
- Run on your own hardware
- Be fully headless and containerized
- Offer global access and built-in redundancy
- Stay secure by default, with zero-knowledge guarantees

Whether you're running a media server, AI model, website, or personal cloud—Piccolo OS is your homelab’s secure, no-friction backbone.

---

## 👤 Who Is This For?

- **🔧 Tinkerers & Builders:** Want to self-host your life? Run containers? Automate your home? This OS is your playground.
- **🔐 Privacy-First Users:** Prefer minimal, surveillance-free tech? You’ll appreciate the zero-trust security model and fully local control.

Right now, Piccolo OS is geared toward **technical users and early adopters**, with powerful abstractions that get out of your way.

---

## 🌍 Our Vision: A User-Owned Internet

Your data isn’t a product. It’s your life. Piccolo OS is part of a bigger bet: that **a decentralized, user-run internet** can outlast the cloud monopolies.

We’re building tools to make self-hosting mainstream—not just possible, but joyful.

---

## 🔐 Core Principles

1. **Zero Trust, Zero Knowledge:** Built-in TPM support ensures devices boot securely. Encrypted traffic and local-first design mean no one—not even us—can see your data.
2. **Atomic & Reliable:** Based on Flatcar Linux, the base OS is immutable and safe to update. Updates are either successful—or not applied at all.
3. **Docker-First Everything:** If it runs in Docker, it runs on Piccolo OS. Full support for official and custom containers.
4. **Minimal & Efficient:** System services stay lean so your apps get the most CPU, RAM, and storage.

---

## 🧱 System Architecture

```
+---------------------------------------------------+
|          Layer 3: Your Applications               |
|      (Piccolo Apps, Custom Docker Containers)     |
+---------------------------------------------------+
|           Layer 2: The Piccolo Runtime            |
| (Core services for Storage, Networking, Auth, DB) |
+---------------------------------------------------+
|             Layer 1: The Host OS                  |
|   (Locked-down Flatcar Linux, piccolod Manager)   |
+---------------------------------------------------+
|           Layer 0: The Hardware                   |
|       (x86-64 CPU, Mandatory TPM 2.0)             |
+---------------------------------------------------+
````

Each layer is hardened, cleanly separated, and built with composability in mind.

---

## ✨ Features

- **Bring Your Own Hardware:** Runs on most x86-64 machines with TPM 2.0 (NUCs, mini PCs, desktops).
- **One-Click Container Deployment:** Launch official or custom Docker containers. Persistent storage handled for you.
- **Global Access Out-of-the-Box:** Every device gets a secure `https://*.piccolospace.com` domain via our open Nexus Proxy.
- **Federated Storage (Coming Soon):** Optional encrypted data sharding across trusted nodes for high availability.
- **Headless & Hardened:** Built on Flatcar for minimalism, automatic updates, and rock-solid security.

---

## 🚀 Quick Start

### Requirements
- x86-64 PC, NUC, or Mini PC
- TPM 2.0 chip (mandatory)
- Blank SSD or HDD

### Install Steps

1. **Download ISO:** Grab `piccolo-os-installer.iso` from our [Releases](https://github.com/AtDexters-Lab/piccolo-os/releases)
2. **Create Bootable USB:** Use [BalenaEtcher](https://www.balena.io/etcher/) or `dd`
3. **Install Piccolo OS:** Boot from USB and follow the setup prompts
4. **Visit `http://piccolo.local`:** Finish setup from another device on the same network

---

## 🔄 Two Ways to Use Piccolo

### 🛠 Self-Hosted (Free Forever)
- Compile from source
- Run your own [Nexus Proxy](https://github.com/AtDexters-Lab/nexus-proxy-server)
- Control every service, every update, every byte

### ☁️ Piccolo Network (Optional Subscription)
- Instant global domain access
- Federated encrypted storage
- Hassle-free remote updates

You choose your level of control—we support both.

---

## 🗺 Roadmap

- [ ] Core OS stable release
- [ ] Beta for self-hosters (Docker, storage, remote access)
- [ ] Installer for custom hardware
- [ ] Piccolo Drive & Photos apps
- [ ] Federated storage alpha

---

## 🤝 Contribute

We’re early, scrappy, and community-powered. Want to shape the future of personal computing?

```bash
git clone https://github.com/AtDexters-Lab/piccolo-os
cd piccolo-os
# open issues, send PRs, hack on stuff
````

Or just join the conversation.

* 💬 [GitHub Discussions](https://github.com/AtDexters-Lab/piccolo-os/discussions)
* 🔗 [Follow on LinkedIn](https://www.linkedin.com/company/piccolo25/)

---

## 🪪 License

Piccolo OS is free and open-source under the [AGPL-3.0](./LICENSE).