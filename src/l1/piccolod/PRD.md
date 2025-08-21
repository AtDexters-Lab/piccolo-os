
# Product Requirements Document: Piccolo App Platform

**Version:** 1.0
**Status:** Draft

## 1. Overview

This document outlines the requirements for the Piccolo App Platform, a core feature of the Piccolo OS. The platform will enable users to easily and securely install, manage, and run containerized applications on their Piccolo device. The architecture is designed around a "stateless device" model, where user applications and their data are persisted on a secure, distributed, federated storage network, making the physical device itself transient and easily recoverable. The user experience is modeled after modern mobile operating systems, prioritizing simplicity, security, and data portability.

## 2. Problem Statement

Users of personal cloud devices want the power of self-hosting applications (e.g., code servers, media servers, notes apps) without the traditional complexity of server administration. They need a simple way to install, update, and manage these applications. Furthermore, they require strong guarantees about the security and privacy of their data and the ability to recover their entire setup seamlessly in the event of hardware failure or device replacement.

## 3. Goals & Objectives

*   **Simplicity:** Provide a user-friendly, near "one-click" experience for installing and managing applications.
*   **Security:** Implement a multi-layered security model to protect user data from all threats.
*   **Portability:** Ensure a user's entire application suite and data can be backed up and restored to a new device with minimal effort.
*   **Resilience:** Build a system that is resilient to hardware failure and network disruption.
*   **Extensibility:** Create a platform that can support a growing catalog of applications.

## 4. User Personas

*   **The Privacy-Conscious User:** A non-technical or semi-technical user who wants to de-google their life and own their data. They value privacy and ease of use over complex, feature-rich systems.
*   **The Hobbyist Developer:** A user who wants a simple environment to run personal projects and development tools without managing a full-blown server.

## 5. User Stories / Key Features

*   **As a user, I want to install a new application from a simple definition file so that I can extend the functionality of my device.**
*   **As a user, I want my installed applications to be automatically accessible via a unique, human-readable local URL.**
*   **As a user, I want to see a list of all my installed applications and their current status (e.g., running, stopped).**
*   **As a user, I want to uninstall an application and have all its associated data cleanly and completely removed.**
*   **As a user, I want my device to be locked on startup and require my unique passphrase to unlock my data, ensuring physical security.**
*   **As a user, I want my entire set of applications and data to be restorable on a new device simply by providing my credentials.**

## 6. Functional Requirements

### 6.1. App Definition
*   Applications shall be defined in a simple YAML format (`app.yaml`).
*   The definition must include: `name`, `image`, `subdomain`, and optional `ports`, `volumes`, and `environment` variables.
*   A `type` field (`system` or `user`) must be included to manage boot sequencing.

### 6.2. Application Lifecycle Management
*   `piccolod` will serve as the sole orchestrator for all applications.
*   An API will be exposed for managing apps:
    *   `POST /api/v1/apps`: Install/register a new app.
    *   `GET /api/v1/apps`: List all apps and their status.
    *   `DELETE /api/v1/apps/{id}`: Uninstall an app.
    *   `POST /api/v1/apps/{id}/start|stop`: Control app state.
*   `piccolod` will manage the full container lifecycle (pull, create, start, stop, remove) by interfacing with the system's container runtime.

### 6.3. Storage
*   All persistent data (app database, app volumes) will reside on a federated, distributed storage network.
*   `piccolod` will manage a central SQLite database (`apps.db`) on the federated storage to track all installed applications and their configurations.
*   **Sandboxing:** Each application will be provided a unique, isolated directory on the storage volume. An application will have no filesystem access outside its designated directory.

### 6.4. Access Architecture & Networking

Access is provided via two distinct methods: local and remote. `piccolod` acts as the central internal reverse proxy for both pathways.

**a) Local Access:**
*   Users access apps on the local network via `http://piccolo.local:[PORT]`.
*   Each application is assigned a unique port number, configured in its `app.yaml`.
*   The root URL, `http://piccolo.local`, will serve a dashboard page listing all installed applications and their corresponding local URLs.

**b) Remote Access:**
*   Users are assigned a global domain: `<user>.piccolospace.com`.
*   Each application is accessible at a dedicated subdomain, e.g., `<app-name>.<user>.piccolospace.com`.
*   All remote traffic is encrypted via TLS.
*   A system container (`ingress-proxy`) runs a client (`nexus-proxy-backend-client`) that establishes a persistent, secure tunnel to the central Nexus Proxy network.
*   Remote traffic is routed through this tunnel to the `ingress-proxy`, which forwards it to `piccolod`.

**c) Unified Routing:**
*   `piccolod` inspects incoming requests to determine the target application.
*   For remote traffic, it uses the `Host` header (e.g., `photos.user.piccolospace.com`).
*   For local traffic, it uses the port number the request was received on (e.g., `9001`).
*   This unified routing logic ensures all access policies are managed in one place.

### 6.5. Boot & Unlock Sequence
1.  The device boots, `piccolod` starts.
2.  `piccolod` uses the TPM to unseal a device secret.
3.  `piccolod` authenticates to a central Piccolo server to get bootstrap configuration.
4.  `piccolod` starts the `storage-provider` system container.
5.  `piccolod` presents a web-based UI to prompt the user for their passphrase.
6.  The user-provided passphrase is used with Argon2id to derive an in-memory decryption key.
7.  The key is used to unlock and mount the federated storage volume.
8.  `piccolod` reads the `apps.db` and starts all `user` type applications.

## 7. Non-Functional Requirements

### 7.1. Security
*   **Data-at-Rest Encryption:** All data on the federated storage must be encrypted.
*   **Two-Level Encryption:** Access is protected by a TPM-based device key AND a user-passphrase-derived data key.
*   **Ephemeral Keys:** The user's data decryption key must only be stored in-memory and must be discarded on reboot.
*   **Storage Isolation:** Application data must be strictly sandboxed.

### 7.2. Reliability
*   `piccolod` must gracefully handle transient network failures when communicating with the storage network or central server.
*   The system must be self-updating, with the base OS and `piccolod` being immutable and updatable as a single unit.

## 8. Constraints & Assumptions

*   The base OS is Flatcar Linux or a similar immutable container OS.
*   The device has a functional TPM 2.0 module.
*   A container runtime (e.g., Docker, Podman) is available on the base OS.
*   The device has reliable internet connectivity, especially at boot.

## 9. Out of Scope (for V1)

*   A graphical user interface (GUI) for browsing and managing the App Store. The initial version will be API-driven.
*   Complex inter-app dependency management.
*   Automated application updates.
*   Building/publishing new applications to a catalog.
