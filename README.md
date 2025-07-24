<h1 align="center">Piccolo OS</h1>

<p align="center">
  <strong>Your personal cloud. Without the cloud.</strong>
  <br />
  An open-source OS that automates storage and secure global access, so you can focus on what matters: running your own containers.
</p>

> **Please Note:** This is the very beginning of our journey. This repository and the Piccolo OS are currently under heavy development. This README lays out our vision and the foundation we are building. We invite you to watch our progress, join the conversation, and get ready to contribute.

Piccolo OS is a container-based operating system designed to give you back control over your digital life. It's for anyone who is tired of trading their privacy and ownership for the convenience of the cloud.

Our mission is to build an open platform that moves us beyond dependency on Big Tech. With Piccolo OS, you can run your own applications, store your own data, and access it from anywhere in the world—all on hardware you own.

## The Vision: A User-Owned Internet

We believe your data is not a product. It's your life, your memories, your work. Piccolo OS is the foundation for a new kind of internet—one that is decentralized, private, and owned by its users.

-   **For the Mindful User:** A simple, secure place for your photos, documents, and files. Your digital life, finally in your hands, working seamlessly without surveillance.
-   **For the Tinkerer:** A powerful, open platform to self-host applications, run 24/7 AI models, host websites, and experiment without limits. Your own slice of the internet, with no gatekeepers.

## Guiding Principles

Our approach is built on a simple philosophy: **Trust Through Verification**. We assume hardware can't always be trusted, so we build security and resilience directly into the software.

1.  **Secure by Default:** The OS is built to be secure from the ground up. It uses a special security chip (a TPM 2.0) to prove your device is running genuine, untampered software. This, combined with end-to-end encryption for all network traffic, creates a true zero-knowledge environment. We can't see your data. Even Mallory can't see your data. Period.

2.  **Reliable & Predictable:** The core operating system is locked to prevent accidental changes or corruption. This makes system updates completely safe—they either work perfectly, or your system is left untouched.

3.  **Infinitely Extensible:** The platform is built for containers. If it runs in Docker, it will run on Piccolo OS. Deploy official apps or bring any container you wish.

4.  **Lightweight & Efficient:** The OS is minimal and stays out of your way, leaving the maximum CPU, RAM, and storage resources available for your applications.

## Architecture

Piccolo OS uses a layered design to keep the system secure and organized.

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
```

-   **Layer 0: Hardware:** The system's security is anchored in a **Trusted Platform Module (TPM) 2.0** chip. This is a mandatory component that provides a secure foundation for the device's identity.
-   **Layer 1: Host OS:** We use **Flatcar Container Linux** as our base—a minimal, locked-down OS that makes the system incredibly stable. A central manager, `piccolod`, runs here to oversee everything.
-   **Layer 2: Piccolo Runtime:** These are the core Piccolo services, running in their own secure boxes. They handle the essentials like storage, networking, and authentication so you don't have to.
-   **Layer 3: Your Applications:** This is where you live. Any app, whether from us or one you deploy yourself, runs in this top layer with limited permissions for maximum safety.

## Key Features

-   **Bring Your Own Hardware:** Install Piccolo OS on our official hardware or your own spare PC or server.
-   **One-Click Docker Apps:** Easily deploy and manage any container. Piccolo handles the persistent storage automatically.
-   **Global Access Out-of-the-Box:** Every device gets a secure `https://*.piccolospace.com` web address. Access your apps from anywhere through our open-source [Nexus Proxy](https://github.com/AtDexters-Lab/nexus-proxy-server), with end-to-end encryption.
-   **Hardened Foundation:** By building on Flatcar Linux, we inherit its proven security and rock-solid update system.
-   **Future: Federated & Resilient:** We're building a system where your device can work with others to become even more reliable. It will intelligently and securely spread encrypted pieces of your data across the network, making it safe even if some devices go offline.

## Freedom of Choice: The Two Paths of Piccolo

Piccolo OS is, and always will be, free and open-source software (AGPL-3.0). We believe in giving you complete control, and that includes how you use the OS. You have two paths:

* **The Self-Hosted Path (Free):** For the ultimate purist. You can compile the OS from source, run your own instance of the [Nexus Proxy](https://github.com/AtDexters-Lab/nexus-proxy-server) on a VPS, and manage your own storage and backups. This path is completely free and gives you absolute, sovereign control over every single component.

* **The Piccolo Network Path (Subscription):** For convenience and powerful out-of-the-box features. A nominal annual subscription will give you access to our managed network, which includes:
    * **Automated Global Access:** Your `https://*.piccolospace.com` address works instantly, powered by our globally distributed Nexus Proxy fleet.
    * **Automated Federated Storage:** (Coming Soon) Your data gains cloud-grade resilience by being intelligently and securely distributed across the network.
    * **Hassle-free Updates & Services.**

## Getting Started

Ready to reclaim your digital life?

### Prerequisites

1.  A compatible **x86-64** computer (e.g., a Mini PC, desktop, or NUC).
2.  A **Trusted Platform Module (TPM) 2.0** chip. This is **mandatory** for security.
3.  A blank SSD or HDD for the installation.

### Installation (via .iso)

1.  **Download the latest ISO:** Grab `piccolo-os-installer.iso` from our [GitHub Releases page](https://github.com/AtDexters-Lab/piccolo-os/releases).
2.  **Create a bootable USB drive:** Use a tool like [BalenaEtcher](https://www.balena.io/etcher/) or `dd`.
3.  **Boot from the USB:** Start your computer from the USB drive and follow the on-screen installer.

Once done, your device will reboot into Piccolo OS. Visit `http://piccolo.local` from another computer on the same network to complete the setup.

## Roadmap

We are just getting started. Here's what we're focused on:

-   [ ] **Core OS Build:** A stable, secure OS for x86-64 systems.
-   [ ] **MVB (Minimum Viable Beta) for Tinkerers:** One-click Docker deployment with storage and global access.
-   [ ] **Community Onboarding:** A polished installation experience for your own hardware.
-   [ ] **Piccolo Drive & Photos:** Our first two official apps for files and photos.
-   [ ] **Federated Storage Alpha:** The first version of our distributed storage engine.

## Contributing

Piccolo is built by the community, for the community. We welcome all contributions.

1.  **Fork** the repository.
2.  **Create** a new feature branch (`git checkout -b feature/your-idea`).
3.  **Commit** your changes (`git commit -am 'Add some feature'`).
4.  **Push** to the branch (`git push origin feature/your-idea`).
5.  **Submit** a **Pull Request**.

<!--
Please check our [Contribution Guidelines](CONTRIBUTING.md) for more details.
-->

## Community

Join the conversation and help us shape the future of the personal internet.

-   **GitHub Discussions:** [Ask questions and share ideas](https://github.com/AtDexters-Lab/piccolo-os/discussions).
-   **Linkedin:** [Follow @piccolo](https://www.linkedin.com/company/piccolo25/) for project updates.

## License

Piccolo OS is licensed under the [GNU Affero General Public License v3.0](/LICENSE).
