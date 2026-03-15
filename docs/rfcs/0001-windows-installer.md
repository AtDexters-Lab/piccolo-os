# RFC: Piccolo OS Windows Installer

## Context

Piccolo OS currently requires users to download a raw/ISO image and flash it to a USB drive using third-party tools (Balena Etcher, `dd`), then manually navigate BIOS/UEFI boot menus to boot from USB. This flow is acceptable for technical users but creates significant friction for non-technical users who are the target audience for Piccolo OS on repurposed home laptops.

This RFC proposes a purpose-built Windows installer application that automates the entire install flow — downloading the image, flashing to USB, and booting from USB — with minimal user interaction and no BIOS navigation.

## Goals

1. A single Windows `.exe` that handles the complete Piccolo OS installation flow
2. User never needs to enter BIOS/UEFI settings or navigate boot menus
3. Graceful degradation when automation isn't possible, falling back to guided manual instructions
4. Support for the common case: a non-technical user repurposing an old home laptop

## Non-Goals

1. Eliminating USB entirely — our research showed that every USB-less approach (WebUSB, Wubi-style loopback, direct DD from Windows) has fundamental blockers
2. Enterprise/managed machine support — corporate endpoint protection, custom Secure Boot keys, and third-party encryption are out of scope
3. Linux/macOS installer — the target user is on Windows (Linux users are technical enough to use existing tools)
4. Network boot (PXE) as the primary path — requires PXE enabled in BIOS (often disabled on consumer laptops) and a second computer running a PXE server; may be explored as a future enhancement

## Background: Approaches Explored and Ruled Out

| Approach | Why Ruled Out |
|----------|--------------|
| **WebUSB** | W3C spec blocklists USB mass storage (interface class 0x08). Browsers cannot access USB flash drives. |
| **Wubi-style loopback install** | Piccolo requires full-disk ownership (btrfs root, read-only snapshots, LVM expansion on first boot). Cannot run from a loopback file on NTFS. |
| **DD from within Windows + reboot into RAM-based installer** | Source and destination are the same physical disk. Loading a 2-3GB image into RAM before DD is unsafe on 4GB machines. |
| **Windows .exe + BootNext to USB (no pre-check)** | USB boot entries are ephemeral — only created by firmware at POST if USB is present. BootNext to a non-existent entry fails silently. |
| **Windows .exe + WinRE (Advanced Startup)** | WinRE's "Choose an option" screen shows confusing options ("Troubleshoot", "Reset your PC", "Windows recovery DVD"). Non-technical users may panic, choose wrong, or abandon. Dealbreaker for UX. |
| **Network boot (PXE)** | Consumer laptops often ship with PXE disabled in BIOS. Requires a second computer running a PXE server. Good for fleet deployment, too complex for single-machine consumer installs. |
| **UEFI HTTP Boot** | Consumer laptop firmware support is spotty. Enterprise-only in practice. |

## Proposed Solution

A stateful Windows installer that flashes the Piccolo **SelfInstall ISO** to USB and automates the boot-from-USB step by **programmatically creating a UEFI boot entry** and setting `BootNext`, with detection and recovery at every phase.

### Key Insight

The UEFI specification does **not** require firmware to create `Boot####` variables for removable USB media. Many firmware implementations boot removable media via the fallback path (`\EFI\BOOT\BOOTX64.EFI`) without ever creating an enumerable boot entry. Rather than relying on firmware to create entries, the installer **constructs and writes its own `Boot####` UEFI variable** with the correct EFI device path pointing to the USB's EFI System Partition. This is the same approach used by `efibootmgr` on Linux. On Windows, the `i255/efiboot` project demonstrates reading and setting `BootNext` for existing entries; this RFC extends that approach to also **create** new `Boot####` entries via `SetFirmwareEnvironmentVariable` with manually constructed `EFI_LOAD_OPTION` structures. This is novel on Windows — no existing consumer installer does this — and requires careful implementation and hardware testing.

Constructing the correct EFI device path for a USB device from Windows requires mapping from Windows device paths to UEFI device paths, which varies by firmware. The installer first attempts to create a boot entry; if `BootNext` fails (detected on next Windows boot), it falls back to a registration reboot strategy, then to guided manual boot instructions.

### Architecture: Stateful Multi-Phase Installer

The installer persists state to disk (`%APPDATA%\piccolo-installer\state.json`) with an HMAC integrity tag (keyed to the installer binary's own hash) to detect corruption. Note: this does not prevent deliberate tampering by a privileged process (the key material is on disk), but it guards against accidental corruption and casual modification. The installer registers itself for auto-start via a Windows Registry `HKLM\...\Run` key (system-wide, since the installer already runs elevated). Each launch, it validates the state file integrity, reads its phase, and resumes.

## Detailed Design

### Image Format

The installer downloads and flashes the **Piccolo SelfInstall ISO** (`.iso`). This is the kiwi-built OEM install image (`installiso="true"`) which contains the disk image and the `dracut-kiwi-oem-dump` installer. When booted from USB, the kiwi OEM dump mechanism presents a target disk selection and automatically DDs the embedded disk image to the internal drive. The ISO is a hybrid image — it can be DD'd directly to a USB drive and boots as a self-contained installer.

The ISO is downloaded as-is (not xz-compressed), so the flash step is a direct raw write to the USB drive without decompression.

**Download URLs (from OBS):**
- x86_64: `https://download.opensuse.org/repositories/home:/atdexterslab:/piccolo-os/home_atdexterslab_atdexterslab_tumbleweed/iso/piccolo-os.x86_64-SelfInstall.iso`
- aarch64: `https://download.opensuse.org/repositories/home:/atdexterslab:/piccolo-os/home_atdexterslab_atdexterslab_tumbleweed/iso/piccolo-os.aarch64-SelfInstall.iso`

The USB is reusable — after installation, it remains bootable and can install Piccolo on additional machines.

### Phase 0: Pre-flight Checks

**Trigger:** Every launch, before any other action.

1. **Verify UEFI mode:**
   - Call `GetFirmwareType()` or check `HKLM\SYSTEM\CurrentControlSet\Control\SecureBoot\State`
   - If Legacy BIOS → show "This computer uses Legacy BIOS. Piccolo OS requires UEFI." with manual instructions and exit

2. **Detect Secure Boot status:**
   - `GetFirmwareEnvironmentVariable("SecureBoot", "{8be4df61-...}", ...)`
   - Record for Phase 4 diagnostics (not a blocker — Piccolo's shim is Microsoft-signed)

3. **Detect BitLocker / Device Encryption:**
   - Query `Win32_EncryptableVolume` WMI class for all volumes
   - Detect specifically whether this is full BitLocker (Pro/Enterprise) or Device Encryption (Home/24H2 auto-enabled)
   - Check for Microsoft Account presence (for recovery key backup status)
   - If third-party encryption detected (VeraCrypt, etc.) → warn and suggest user consult their encryption tool's documentation

4. **Detect security software:**
   - Check Windows Defender Controlled Folder Access status via `Get-MpPreference` / WMI
   - Enumerate installed antivirus via `HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall` and Security Center WMI (`AntiVirusProduct`)
   - If Controlled Folder Access is enabled → warn: "Windows Controlled Folder Access may block the installer. Please temporarily disable it in Windows Security settings."
   - If known-problematic AV detected (Norton, McAfee, Kaspersky — commonly preinstalled) → warn: "Your antivirus software may interfere. Consider temporarily disabling real-time protection."

5. **Detect laptop manufacturer:**
   - `Win32_ComputerSystem.Manufacturer` via WMI
   - Store for fallback boot key instructions (Dell=F12, HP=F9, Lenovo=F12, ASUS=F8/ESC, Acer=F12)

6. **Check state file:**
   - If exists and HMAC valid → resume from stored phase
   - If exists and HMAC invalid → discard (possible tampering), start fresh
   - If `phase_attempts > 2` or state is older than 24 hours → clean up and start fresh

### Phase 1: Download and Flash

**Trigger:** First launch (no prior state) and pre-flight passed.

1. **Enumerate removable USB drives:**
   - List physical drives via `SetupDiGetClassDevs` / `DeviceIoControl`
   - Filter to removable USB mass storage devices
   - Identify each drive by **USB serial number and VID/PID** (stable across reboots, unlike `PhysicalDriveN` which may be reassigned)
   - Display drive label, size, and manufacturer
   - **Check capacity:** refuse if drive is smaller than the ISO image size
   - Require user confirmation: "This will erase all data on [SanDisk Ultra 16GB]. Continue?"

2. **Download the Piccolo OS image:**
   - Fetch image manifest from Piccolo CDN over HTTPS
   - Manifest includes: image URL, size, SHA-256 checksum, **detached Ed25519 signature**
   - **Verify manifest signature** against a public key embedded in the installer binary (protects against CDN compromise)
   - Download image using HTTPS with resume support (`Content-Range` header for resume-on-failure)
   - Single connection — home internet bandwidth is the bottleneck, not connection concurrency. Parallel chunks add complexity without meaningful speed improvement.
   - Show progress bar with speed and ETA
   - Verify SHA-256 checksum after download
   - If a previously downloaded image exists and checksum matches, skip download

3. **Flash image to USB:**
   - Dismount all volumes on the target USB (`FSCTL_DISMOUNT_VOLUME`, `FSCTL_LOCK_VOLUME`)
   - Write ISO directly to raw disk via `CreateFile("\\.\PhysicalDriveN", ...)` with sequential writes. The ISO is a hybrid image and can be DD'd directly — no decompression needed.
   - If I/O error (USB removed mid-flash) → detect, inform user ("USB was disconnected during write. Please re-insert and try again."), allow retry
   - After write, verify by reading back and checking checksum of the flashed content
   - The USB now has a valid EFI System Partition with Piccolo's shim + GRUB

4. **Transition to Phase 2:**
   - Save state: `{ "phase": 2, "usb_serial": "...", "usb_vid_pid": "...", "attempt": 0 }`
   - Proceed immediately to Phase 2 (no reboot yet)

### Phase 2: Boot Entry Setup

**Trigger:** Phase 1 completed, or auto-start after registration reboot.

1. **Locate the USB drive:**
   - Find the USB by serial number and VID/PID (stable identifiers, not drive letter or PhysicalDriveN)
   - If USB not found → "Please plug in the Piccolo USB drive and try again." Wait or allow retry.

2. **Attempt to create a UEFI boot entry:**
   - Construct an `EFI_LOAD_OPTION` structure with the USB device's EFI device path pointing to `\EFI\BOOT\BOOTX64.EFI` (or `\EFI\BOOT\shimx64.efi` for Secure Boot)
   - Map the Windows device path to a UEFI device path. Reference: `i255/efiboot` (for reading/setting BootNext on existing entries) and the UEFI 2.10 spec sections 3.4-3.5 and 10.3 (for `EFI_LOAD_OPTION` and device path structures).
   - Find an unused `Boot####` variable number
   - Write the boot entry via `SetFirmwareEnvironmentVariable`
   - Requires `SeSystemEnvironmentPrivilege` (obtained via `AdjustTokenPrivileges`)

3. **If boot entry creation succeeds:**
   - Transition to Phase 3

4. **If boot entry creation fails** (privilege denied, firmware rejects the variable, device path mapping fails):
   - **Fallback strategy A: Search for existing firmware-created USB boot entry**
     - Enumerate `Boot####` variables, search for entries matching the USB device
     - If found → use it, transition to Phase 3
   - **Fallback strategy B: Registration reboot**
     - If `attempt < 2` and no prior registration reboot → reboot with USB present so firmware creates an entry at POST
     - Register for auto-start: `HKLM\Software\Microsoft\Windows\CurrentVersion\Run\PiccoloInstaller` with installer path
     - Save state: `{ "phase": 2, "attempt": attempt + 1 }`
     - Show message: "Your computer needs to restart to prepare the USB drive. Please leave the USB plugged in. Do not move it to a different USB port."
     - Trigger reboot: `ExitWindowsEx(EWX_REBOOT, ...)`
     - On next boot, installer auto-launches, re-enters Phase 2, re-attempts boot entry creation/detection
   - **Fallback strategy C: Manual boot instructions**
     - If `attempt >= 2` → firmware doesn't cooperate
     - Show manufacturer-specific manual boot instructions (see Fallback section)
     - Clean up state and registry

### Phase 3: BootNext and Final Reboot

**Trigger:** USB boot entry confirmed (created or found).

1. **Ethernet check (blocking confirmation):**
   - "Is an Ethernet cable connected to this laptop? Piccolo OS requires a wired Ethernet connection for initial setup. You will not be able to set up Piccolo without it."
   - [Yes, Ethernet is connected] / [No, I need to connect it first]
   - If No → wait for user, don't proceed

2. **Suspend BitLocker / Device Encryption (if active):**
   - Use `Win32_EncryptableVolume.SuspendProtection` WMI method with `RebootCount=2` (accounts for possible registration reboot in the flow)
   - For Device Encryption (Windows 11 24H2 Home): same API, but show user-friendly messaging: "Windows has automatically encrypted your drive. We need to temporarily pause this to restart your computer. This is safe — your data remains protected until installation begins."
   - If no Microsoft Account detected → warn: "Your drive encryption recovery key may not be backed up. If anything goes wrong before installation completes, you may need this key to access Windows. You can find it at aka.ms/myrecoverykey"
   - If suspension fails → do not proceed. Show: "Unable to pause drive encryption. This may be due to a security policy on this computer." + manual fallback instructions
   - **State ordering:** Write `{ "bitlocker_suspended": true }` to state file BEFORE calling the suspension API. On any subsequent launch, if state says `bitlocker_suspended: true` but install hasn't completed, attempt to re-enable BitLocker via `Win32_EncryptableVolume.ResumeProtection` during cleanup.

3. **Set BootNext:**
   - Write the boot entry number to the `BootNext` UEFI variable via `SetFirmwareEnvironmentVariable`
   - Save state: `{ "phase": 4, "boot_next_set": true, "boot_next_target": "Boot00XX", "bitlocker_suspended": true }`
   - Keep Registry auto-start entry for Phase 4 failure detection

4. **Show final confirmation:**
   - "Ready to install Piccolo OS. Your computer will restart and installation will begin automatically."
   - "Do not unplug the USB drive or the Ethernet cable."
   - "This will erase all data on this computer's internal drive."
   - [Restart and Install] / [Cancel]

5. **If user cancels at any point:** see Cancellation section.

6. **Trigger reboot:**
   - `ExitWindowsEx(EWX_REBOOT, ...)`
   - Firmware honors `BootNext`, boots from USB
   - Kiwi OEM dump takes over, DDs Piccolo to internal disk, reboots into Piccolo
   - On first boot, user accesses `piccolo.local` via Ethernet

### Phase 4: Failure Detection

**Trigger:** Installer auto-launches after a BootNext reboot, meaning the USB boot **did not happen** — the machine booted back into Windows.

1. **Re-enable BitLocker** if it was suspended (check state file `bitlocker_suspended` flag):
   - Call `Win32_EncryptableVolume.ResumeProtection`
   - This ensures data-at-rest protection is restored regardless of what happens next

2. **Diagnose the failure:**
   - **USB not detected** (by serial number) → "The USB drive was not found. It may have been unplugged. Please re-insert it and try again." Allow retry (go back to Phase 2).
   - **Secure Boot enabled** → likely cause is bootloader signature rejection → "Secure Boot may be preventing installation. You can try disabling Secure Boot in BIOS settings, or boot manually from USB." + manufacturer-specific BIOS key instructions
   - **BootNext was consumed but USB didn't boot** → firmware bug → fall back to manual instructions
   - **USB detected, Secure Boot off** → unknown firmware issue → fall back to manual instructions

3. **Clean up:**
   - Remove the installer-created `Boot####` UEFI variable (if we created one)
   - Remove Registry auto-start entry
   - Clear state file
   - Offer manual fallback instructions

### Fallback: Guided Manual Boot

When automation fails, the installer provides the best possible manual experience:

1. **Detect laptop manufacturer** via WMI (`Win32_ComputerSystem.Manufacturer`)
2. **Show the exact boot menu key** for that manufacturer:
   - Dell: F12
   - HP: F9
   - Lenovo: F12
   - ASUS: F8 or ESC
   - Acer: F12
   - Others: show common keys (F12, F9, ESC, F2)
3. **Show step-by-step instructions:**
   - "1. Your USB drive is ready. Restart your computer."
   - "2. Immediately press [F12] repeatedly until you see a boot menu."
   - "3. Select [USB device name] from the list."
   - "4. Piccolo will install automatically. Do not remove the USB or Ethernet cable."
   - "5. After installation, your computer will restart into Piccolo OS."

### Cancellation and Cleanup

The user can cancel at any phase. Cancellation always performs full cleanup:

- **Phase 1 (download/flash):** Stop download or flash. Partial USB write leaves USB in unusable state — inform user they may need to reformat it.
- **Phase 2 (boot entry):** Remove any created `Boot####` UEFI variable. Remove Registry auto-start entry. Clear state file.
- **Phase 3 (BootNext set, BitLocker suspended):** Remove `BootNext` variable. Re-enable BitLocker via `ResumeProtection`. Remove Registry auto-start entry. Clear state file.
- **Any phase:** Remove state file and Registry entry. Log cancellation reason.

A "Cancel Installation" button is visible on every screen.

### Post-Installation: USB and Disk Behavior

- **USB remains bootable** after installation. It can be reused to install Piccolo on additional machines.
- **User should remove USB** after Piccolo reboots into first-boot setup. The kiwi OEM dump mechanism removes itself after the first successful install, so re-booting from USB would re-trigger the install flow. The Piccolo first-boot screen should remind the user to remove the USB.
- **Internal disk selection:** The kiwi OEM installer's `oem-device-filter` determines which internal disk to install to. Currently it only filters out `/dev/ram`. If multiple internal disks are present, kiwi selects the first eligible one. This is an existing Piccolo/kiwi behavior, not introduced by this RFC. **Recommendation:** Consider adding a disk selection prompt to the kiwi installer for multi-disk systems in a future RFC.

## State Machine Summary

```
┌──────────────┐
│   Phase 0    │
│ Pre-flight   │──── Legacy BIOS ────▶ [Show manual instructions, exit]
│ checks       │──── AV warning ────▶ [Show guidance, allow continue]
└──────┬───────┘
       │ passed
       ▼
┌──────────────┐
│   Phase 1    │──── USB removed ───▶ [Error, allow retry]
│ Download +   │──── I/O error ─────▶ [Error, allow retry]
│ Flash USB    │
└──────┬───────┘
       │ success
       ▼
┌──────────────┐     entry created    ┌──────────────┐
│   Phase 2    │ ─── or found ──────▶ │   Phase 3    │
│ Create/find  │                      │ Ethernet chk │
│ boot entry   │                      │ BitLocker    │
└──────┬───────┘                      │ BootNext     │
       │                              │ Reboot       │
       │ creation failed,             └──────┬───────┘
       │ no existing entry                   │
       ▼                                     │
┌──────────────┐                             ▼
│ Registration │                  ┌──────────────────┐
│ Reboot       │                  │ Boots from USB?  │
│ (attempt < 2)│                  │                  │
└──────┬───────┘                  │  YES → Piccolo   │
       │                          │        installs  │
       │ retry Phase 2            │                  │
       ▼                          │  NO → Phase 4    │
┌──────────────┐                  └────────┬─────────┘
│ Still fails? │                           │
│ attempt >= 2 │                           ▼
│              │                  ┌──────────────────┐
│ → Manual     │                  │   Phase 4        │
│   fallback   │                  │ Re-enable BL     │
└──────────────┘                  │ Diagnose failure │
                                  │ Offer remediation│
                                  │ or manual        │
                                  │ fallback         │
                                  └──────────────────┘
```

Every phase has a [Cancel] path → full cleanup (see Cancellation section).

## User Experience

### Happy Path (Boot Entry Creation Works — Expected on Most Hardware)

```
User sees                                     Behind the scenes
─────────                                     ─────────────────
"Welcome to Piccolo Installer"                Pre-flight checks pass
"Select your USB drive: [SanDisk 16GB ▼]"     Enumerates removable drives
"Downloading Piccolo OS... 67%"               HTTPS download with resume
"Flashing to USB... 45%"                      Direct ISO → raw write to USB
"Is Ethernet connected to this laptop?"       Blocking confirmation
"Ready! Click 'Restart and Install'"          Boot entry created, BootNext set
                                              ── reboot ──
                                              Boots from USB automatically
"Installing Piccolo OS..."                    Kiwi OEM dump DDs to internal disk
                                              ── reboot ──
                                              Piccolo first boot
                                              User accesses piccolo.local
```

### Registration Reboot Path (Boot Entry Creation Fails, Firmware Creates Entry at POST)

Same as above, but with one extra automated reboot between flashing and the "Ready!" screen. User sees: "Your computer needs to restart to prepare the USB drive. Please leave the USB plugged in."

### Fallback Path (Automation Fails Entirely)

User sees manufacturer-specific instructions: "Press F12 repeatedly on restart to open the boot menu, then select your USB drive." The USB is already flashed and ready — user just needs to boot from it manually.

### Device Encryption Path (Windows 11 24H2 Home)

User sees: "Windows has automatically encrypted your drive. We need to temporarily pause this to restart your computer. This is safe." Installer handles suspension transparently.

## Logging

All installer actions are logged to `%APPDATA%\piccolo-installer\installer.log` with timestamps:

- Phase transitions
- Hardware detection results (manufacturer, Secure Boot, BitLocker, AV)
- UEFI variable operations (read/write attempts, successes, failures)
- USB operations (enumeration, serial numbers, flash progress, verification)
- Download progress and errors
- Errors and stack traces

On failure, the installer offers: "Something went wrong. You can share the log file to get help: [Open log file location]"

A `--debug` flag enables verbose logging and a dry-run mode (simulates BootNext without actually rebooting) for development and hardware testing.

## Tech Stack

### Recommended: Rust + egui

- **Rust:** Single static binary, no runtime dependencies, cross-compiles to Windows x86_64. Small binary size (~10-15MB including egui).
- **egui:** Immediate-mode GUI library. Simple, fast, no dependency on Windows UI frameworks. Renders via GPU or software fallback.
- **reqwest:** HTTP client with Range/resume support.
- **windows-rs:** Official Microsoft Rust bindings for Win32 and WMI APIs.
- **ed25519-dalek:** Manifest signature verification.

## Security Considerations

1. **Admin privileges:** Required for raw disk writes, UEFI variable access, BitLocker suspension. The app requests UAC elevation on launch.
2. **Image integrity:** SHA-256 checksum + Ed25519 signature verification. The public key is embedded in the installer binary. Even if the CDN is compromised, a forged image cannot pass signature verification.
3. **Secure Boot:** Piccolo ships shim signed by Microsoft's third-party UEFI CA. Works on most consumer machines. If firmware only trusts the Windows CA (not third-party), Phase 4 detects and guides.
4. **BitLocker / Device Encryption:**
   - Suspended via `Win32_EncryptableVolume.SuspendProtection` with `RebootCount=2`
   - This auto-expires after 2 reboots (standard Windows mechanism, used by Windows Update)
   - If the installer crashes or user cancels after suspension, the protection auto-resumes after the reboot count expires
   - Additionally, on any subsequent installer launch, if state indicates suspension was attempted, the installer proactively calls `ResumeProtection`
5. **State file integrity:** HMAC tag using a key derived from the installer binary's hash. Detects corruption between reboots (not a security boundary against privileged attackers, but guards against accidental corruption and casual modification).
6. **Registry auto-start:** Uses `HKLM\...\Run` (system-wide, works regardless of which user logs in). Cleaned up on completion, cancellation, or after 24-hour timeout.
7. **Code signing and SmartScreen:** The installer `.exe` should be code-signed when a certificate is procured. Note: as of 2024, EV certificates no longer bypass SmartScreen — Microsoft requires reputation building through download volume regardless of certificate type. The installer will initially trigger SmartScreen warnings. The piccolo.dev download page will include clear instructions for users to bypass SmartScreen ("click More info → Run anyway"). This is acceptable for the initial launch.

## Constraints and Prerequisites

1. **UEFI required.** Legacy BIOS machines are not supported by the automated flow. The installer detects Legacy BIOS and shows manual instructions immediately.
2. **Ethernet connection required.** Piccolo OS requires a wired Ethernet connection to the router for first-boot configuration (`piccolo.local`). This is an existing Piccolo requirement. The installer enforces this with a blocking confirmation before the final reboot.
3. **USB drive required.** Minimum capacity: the installer checks USB size against the ISO size and refuses to proceed if insufficient. Expected minimum: 4GB+.
4. **Internet connection required.** For downloading the Piccolo OS image during Phase 1.
5. **Windows 10 or 11.** The installer uses UEFI variable APIs available since Windows 8, but is tested on Windows 10 and 11.

## Failure Modes and Mitigations

| Failure | Detection | Mitigation |
|---------|-----------|------------|
| USB removed mid-flash | I/O error on write | Inform user, allow retry after re-inserting |
| Power loss during flash | State says Phase 1 in progress, USB checksum fails on next launch | Offer to re-flash |
| Power loss after BitLocker suspension but before BootNext | State says `bitlocker_suspended: true` | On next launch, call `ResumeProtection`. Also: `SuspendProtection` auto-expires after `RebootCount` reboots. |
| Different user logs in after reboot | N/A — Registry key is in `HKLM` (system-wide) | Installer auto-starts for any user |
| Windows Update reboots between phases | State file is source of truth | Installer resumes from correct phase. USB at POST means firmware may create boot entry (bonus). |
| Firmware ignores BootNext | Phase 4 detects (installer is back in Windows) | Re-enable BitLocker, diagnose, fall back to manual |
| Slow USB 2.0 drive (30+ min flash) | Detect USB speed via device properties | Show estimated time: "This may take 20-30 minutes with a USB 2.0 drive. A USB 3.0 drive is much faster." |
| Installer .exe deleted or moved | Registry auto-start path fails | State file includes installer path. On orphaned state detection (>24 hours old), cleanup script can be provided. |
| Multiple USB drives connected | Track USB by serial number + VID/PID | Unambiguous identification. If tracked USB not found among connected drives, ask user to re-insert it. |
| Antivirus blocks raw disk write | Detect AV in pre-flight, catch I/O errors | Pre-flight warning. If write fails, suggest temporarily disabling AV. |
| User moves USB to different port | Boot entry may reference specific port path | If BootNext fails (Phase 4), guide user to keep USB in same port and retry, or fall back to manual. |

## Open Questions

1. **Code signing** — The installer will initially trigger SmartScreen warnings. Documentation on piccolo.dev will guide users past SmartScreen ("click More info → Run anyway"). Code signing certificate procurement is TBD but not a launch blocker.
2. **Manifest signing infrastructure** — Images are hosted on OBS (`download.opensuse.org`), which supports HTTP Range requests. Ed25519 signing infrastructure for the manifest needs to be set up. The signing key pair should be generated and the private key secured separately from the CDN.
3. **Installer distribution** — Direct download from piccolo.dev, linking to GitHub Releases for the `.exe` artifact.
4. **Telemetry / error reporting** — Should the installer report anonymous failure statistics (phase, manufacturer, Secure Boot status, AV detected) to improve success rates? If yes, opt-in only with clear privacy disclosure. At minimum, provide a "generate support bundle" button that creates a shareable log file.
5. **Image versioning** — The installer always downloads the latest image from the manifest endpoint. If the manifest format changes, old installer versions break. Consider embedding a manifest schema version and showing "Please download the latest installer from piccolo.dev" if the schema is unrecognized.
6. **Multi-disk systems** — If the target laptop has multiple internal drives, the kiwi OEM installer currently auto-selects the first eligible one. Consider adding a disk selection prompt to kiwi in a follow-up RFC to prevent accidental data loss on a secondary drive.
7. **Future: Network boot path** — PXE-based install as an optional fast path for machines with PXE enabled, using a companion cross-platform `piccolo-server` app on a second computer. Deferred to a follow-up RFC.

## Verification Plan

1. **Unit tests:** State machine transitions, UEFI variable parsing, EFI device path construction, boot entry creation/deletion, HMAC state file validation
2. **Integration tests on real hardware:**
   - Test matrix: Dell, HP, Lenovo, ASUS, Acer — both with and without Secure Boot enabled
   - Test boot entry creation success rate across firmware vendors
   - Verify BootNext is honored after programmatic boot entry creation
   - Test BitLocker/Device Encryption suspension and re-enablement
   - Test with Windows Defender Controlled Folder Access enabled
   - Test with common preinstalled AV (Norton, McAfee)
   - Test USB 2.0 vs 3.0 flash speeds
3. **VM testing:** VirtualBox/QEMU with OVMF firmware for rapid iteration on the state machine and UEFI variable handling
4. **Edge case testing:**
   - USB inserted after POST (registration reboot path)
   - Secure Boot rejection (Phase 4 diagnosis)
   - Multiple USB drives connected
   - USB removed mid-flow
   - Installer cancelled at each phase
   - Power loss simulation at each phase
   - Different user login after reboot
   - Windows Update reboot between phases
5. **Debug mode:** `--debug` flag for verbose logging and dry-run (simulates BootNext, doesn't actually reboot)
