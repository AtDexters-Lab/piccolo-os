### TODO for Piccolo OS (L0)

1.  **Disable Flatcar Automatic Updates:**
    *   **Action:** Locate the update configuration file (likely `update.conf`) within the Flatcar build scripts.
    *   **Action:** Modify the configuration to disable automatic updates from the public Flatcar repository. This might involve changing `SERVER` or `GROUP` settings.
    *   **Verification:** Build a new Piccolo OS image and verify that the update service is disabled or points to a local/private repository.

2.  **Harden `piccolod` Service:**
    *   **Action:** Create a systemd service file for `piccolod` (e.g., `piccolod.service`).
    *   **Action:** Configure the service to restart automatically on failure (`Restart=always`).
    *   **Action:** Integrate the `piccolod.service` file into the L0 build process so it's included in the final image and enabled by default.
    *   **Testing:** Create a test plan to simulate failures (e.g., kill the `piccolod` process) and verify that the service restarts as expected.

3.  **Document `update_engine` Usage:**
    *   **Action:** Research the `update_engine_client` command-line utility in Flatcar.
    *   **Action:** Create a "how-to" guide that documents the process of:
        *   Manually triggering an update.
        *   Pointing the `update_engine` to a custom update server.
        *   Checking the status of an update.
    *   **Location:** This guide should be added to the project's documentation.

4.  **Configure `piccolod` to Run as Root:**
    *   **Action:** Ensure `piccolod.service` systemd configuration runs the service as root user.
    *   **Rationale:** Required for update architecture implementation:
        *   Direct disk access (`/dev/sdX`) for USB→SSD installation
        *   D-Bus system bus access for `update_engine` communication
        *   TPM device access (`/dev/tpm0`) for hardware-based authentication
        *   UEFI variable modification (`/sys/firmware/efi/efivars`)
        *   Bootloader installation and partition management
    *   **Security:** Implement principle of least privilege within code, comprehensive audit logging
    *   **Reference:** See `../../docs/architecture/update-system.md` for complete technical justification

5.  **Implement Health Check Integration:**
    *   **Action:** Create systemd service dependency for `update-engine` to wait for `piccolod` success
    *   **Implementation:** Add `/etc/systemd/system/update-engine.service.d/10-piccolod-dependency.conf`
    *   **Content:** `[Unit]\nAfter=piccolod.service\nRequires=piccolod.service`
    *   **Testing:** Verify automatic rollback works when `piccolod` fails to start
    *   **Reference:** See `../../docs/operations/health-checks.md` for complete implementation

6.  **Disable Flatcar Auto-Updates (Compile-time):**
    *   **Action:** Modify build system to generate `/etc/flatcar/update.conf` with `GROUP=disabled`
    *   **Rationale:** `piccolod` takes full control of update process (no Omaha protocol needed)
    *   **Implementation:** Update `generate_update_config()` in `build_piccolo.sh`
    *   **Testing:** Verify Flatcar update_engine doesn't automatically check for updates

7.  **Implement JWT-based TPM Authentication:**
    *   **Action:** Develop TPM attestation and JWT token generation for Piccolo server communication
    *   **Libraries:** `google/go-tpm-tools`, `golang-jwt/jwt/v5`
    *   **Features:** Device registration, subscription validation, hardware binding
    *   **Reference:** See `../../docs/development/api-design.md` for complete API specification

8.  **Add TPM Disk Encryption Support:**
    *   **Action:** Implement optional TPM-sealed disk encryption during installation
    *   **Tools:** `systemd-cryptenroll`, LUKS2, automated re-sealing for updates
    *   **Integration:** Enhance installer and trust agent components
    *   **Reference:** See `../../docs/security/tpm-encryption.md` for implementation details

9.  **Enhanced Testing Framework:**
    *   **Action:** Extend `test_piccolo_os_image.sh` with new test scenarios:
        *   Health check integration and rollback testing
        *   TPM functionality validation
        *   Update flow with JWT authentication
        *   Disk encryption installation and unlock testing
    *   **VM Setup:** Add virtual TPM support for comprehensive testing
