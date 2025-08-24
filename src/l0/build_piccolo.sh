#!/usr/bin/env bash
set -euo pipefail

# ------------------------------------------------------------------------------
# Piccolo OS – MicroOS-based ISO builder (UEFI + Secure Boot + Live/Self-Install)
# Uses KIWI NG. Runs inside a container if docker/podman is available.
#
# USAGE:
#   ./build_piccolo.sh /abs/path/to/piccolod [VERSION] [ARCH]
#
# DEFAULTS:
#   VERSION = 0.1.0
#   ARCH    = x86_64   (use aarch64 for Raspberry Pi UEFI boot flows)
#
# OUTPUT:
#   ./dist/piccolo-os-<ARCH>-<VERSION>.iso
#
# NOTES:
# - This script uses a persistent builder container to avoid re-installing
#   dependencies on every run, making builds much faster.
# - The builder image is created automatically on the first run.
# ------------------------------------------------------------------------------

# -------- parameters ----------
PICCOLOD_BIN="${1:-}"
VERSION="${2:-0.1.0}"
ARCH="${3:-x86_64}"

if [[ -z "${PICCOLOD_BIN}" ]] || [[ ! -f "${PICCOLOD_BIN}" ]]; then
  echo "ERROR: Provide path to built 'piccolod' binary as the first arg."
  echo "Example: ./build_piccolo.sh /home/me/build/piccolod 0.1.0 x86_64"
  exit 1
fi

# -------- env & paths ----------
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WORK_DIR="${ROOT_DIR}/.work"
KIWI_DIR="${ROOT_DIR}/kiwi"
OVERLAY_DIR="${KIWI_DIR}/root_overlay"
DIST_DIR="${ROOT_DIR}/dist"
IMAGE_NAME="piccolo-os"
IMAGE_LABEL="${IMAGE_NAME}-${ARCH}-${VERSION}"

mkdir -p "${WORK_DIR}" "${DIST_DIR}" "${OVERLAY_DIR}"

# -------- detect container runtime ----------
RUNTIME=""
if command -v podman >/dev/null 2>&1; then
  RUNTIME="podman"
elif command -v docker >/dev/null 2>&1; then
  RUNTIME="docker"
fi

# -------- check kiwi locally if no container ----------
function have_kiwi_local() {
  command -v kiwi-ng >/dev/null 2>&1
}

# -------- scaffold KIWI config on first run ----------
CONFIG_XML="${KIWI_DIR}/config.xml"

if [[ ! -f "${CONFIG_XML}" ]]; then
  cat > "${CONFIG_XML}" <<'XML'
<!--
  Piccolo OS – MicroOS-based live/self-install ISO
  Schema: KIWI NG (v9). This profile targets:
    - UEFI boot
    - Secure Boot (shim + signed kernel from repos)
    - Live rootfs with ability to install to disk (SelfInstall workflow)
-->
<image schemaversion="7.4" name="piccolo-os">
  <description type="system">
    <author>Piccolo</author>
    <contact>oss@piccolospace.com</contact>
    <specification>MicroOS-based headless appliance with piccolod</specification>
  </description>

  <preferences>
    <type image="iso" filesystem="btrfs" firmware="uefi" bootloader="grub2" hybrid="true"/>
    <version>0.1.0</version>
    <rpm-excludedocs>true</rpm-excludedocs>
    <rpm-check-signatures>true</rpm-check-signatures>
    <locale>en_US</locale>
    <keytable>us</keytable>
    <timezone>UTC</timezone>
    <packagemanager>zypper</packagemanager>
  </preferences>

  <repository type="rpm-md">
    <source path="https://download.opensuse.org/tumbleweed/repo/oss/"/>
  </repository>
  <repository type="rpm-md">
    <source path="https://download.opensuse.org/tumbleweed/repo/non-oss/"/>
  </repository>
  <repository type="rpm-md">
    <source path="https://download.opensuse.org/update/tumbleweed/"/>
  </repository>

  <packages type="bootstrap">
    <package>openSUSE-release</package>
    <package>patterns-base-minimal_base</package>
  </packages>

  <packages type="image">
    <package>openSUSE-release-microos</package>
    <package>microos-release</package>
    <package>filesystem</package>
    <package>bash</package>
    <package>coreutils</package>
    <package>shadow</package>
    <package>ca-certificates</package>
    <package>transactional-update</package>
    <package>rebootmgr</package>
    <package>selinux-policy-targeted</package>
    <package>policycoreutils</package>
    <package>NetworkManager</package>
    <package>NetworkManager-nmcli</package>
    <package>nftables</package>
    <package>chrony</package>
    <package>podman</package>
    <package>conmon</package>
    <package>crun</package>
    <package>skopeo</package>
    <package>shim</package>
    <package>grub2</package>
    <package>grub2-x86_64-efi</package>
    <package>mokutil</package>
    <package>kernel-default</package>
    <package>kernel-firmware</package>
    <package>ucode-amd</package>
    <package>ucode-intel</package>
    <package>tpm2-tools</package>
    <package>tpm2-tss</package>
    <package>ima-evm-utils</package>
    <package>kiwi-live</package>
    <package>yast2-installation</package>
    <package>yast2-firstboot</package>
  </packages>

  <overlaydir>root_overlay</overlaydir>
</image>
XML

  mkdir -p "${OVERLAY_DIR}/etc/systemd/system" \
           "${OVERLAY_DIR}/usr/local/piccolo/v1/bin" \
           "${OVERLAY_DIR}/etc/piccolo"

  cat > "${OVERLAY_DIR}/etc/systemd/system/piccolod.service" <<'UNIT'
[Unit]
Description=Piccolo Orchestrator (piccolod)
After=network-online.target
Wants=network-online.target

[Service]
Type=notify
ExecStart=/usr/local/piccolo/current/bin/piccolod serve --config /etc/piccolo/config.yaml
Restart=always
RestartSec=2s
NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=yes
PrivateTmp=yes
LockPersonality=yes

[Install]
WantedBy=multi-user.target
UNIT

  cat > "${OVERLAY_DIR}/etc/piccolo/config.yaml" <<'YAML'
# Piccolo default config (edit in your repo)
listen: "0.0.0.0:443"
data_dir: "/var/lib/piccolo"
YAML
fi

# -------- copy piccolod into overlay ----------
install -m 0755 "${PICCOLOD_BIN}" "${OVERLAY_DIR}/usr/local/piccolo/v1/bin/piccolod"
if [[ -L "${OVERLAY_DIR}/usr/local/piccolo/current" ]]; then
  rm -f "${OVERLAY_DIR}/usr/local/piccolo/current"
fi
ln -sfn ../v1 "${OVERLAY_DIR}/usr/local/piccolo/current"

mkdir -p "${OVERLAY_DIR}/etc/systemd/system/multi-user.target.wants"
ln -sfn /etc/systemd/system/piccolod.service \
  "${OVERLAY_DIR}/etc/systemd/system/multi-user.target.wants/piccolod.service"

# -------- bump version in config.xml to match CLI arg ----------
if command -v python3 >/dev/null 2>&1; then
  python3 - <<PY >/dev/null 2>&1 || true
from pathlib import Path
p=Path("${CONFIG_XML}")
s=p.read_text()
s=s.replace("<version>0.1.0</version>", "<version>${VERSION}</version>")
p.write_text(s)
PY
fi

# --- containerized build (preferred for portability) ---
if [[ -n "${RUNTIME}" ]]; then
  BUILDER_IMG_TAG="piccolo-os-builder:${ARCH}"
  BUILDER_DOCKERFILE="${ROOT_DIR}/build.Dockerfile"

  echo "==> Using container runtime '${RUNTIME}'"
  if ${RUNTIME} image inspect "${BUILDER_IMG_TAG}" >/dev/null 2>&1; then
    echo "--> Found existing builder image: ${BUILDER_IMG_TAG}"
  else
    echo "--> Builder image not found. Building it now (this will take a few minutes)..."
    ${RUNTIME} build \
      -t "${BUILDER_IMG_TAG}" \
      -f "${BUILDER_DOCKERFILE}" \
      --build-arg "ARCH=${ARCH}" \
      "${ROOT_DIR}"
    echo "--> Builder image created successfully."
  fi

  echo "==> Running KIWI build using pre-built image"
  ${RUNTIME} run --rm \
    -v "${KIWI_DIR}:/build/kiwi:Z" \
    -v "${DIST_DIR}:/build/result:Z" \
    --env KIWI_DEBUG=1 \
    --privileged \
    "${BUILDER_IMG_TAG}" \
    kiwi-ng --color-output --debug --logfile /build/result/kiwi.log --target-arch "${ARCH}" \
      system build \
      --description /build/kiwi \
      --target-dir /build/result

elif have_kiwi_local; then
  echo "==> Using local kiwi-ng"
  kiwi-ng --color-output --debug --logfile "${DIST_DIR}/kiwi.log" --target-arch "${ARCH}" \
    system build \
    --description "${KIWI_DIR}" \
    --target-dir "${DIST_DIR}"
else
  echo "ERROR: Neither podman/docker nor kiwi-ng found."
  exit 1
fi

# -------- collect artefacts ----------
ISO_SRC="$(ls -t "${DIST_DIR}"/*.iso 2>/dev/null | head -n1 || true)"
if [[ -z "${ISO_SRC}" ]]; then
  echo "ERROR: No ISO produced. Check ${DIST_DIR}/kiwi.log for details."
  exit 1
fi

FINAL_ISO="${DIST_DIR}/${IMAGE_LABEL}.iso"
mv -f "${ISO_SRC}" "${FINAL_ISO}"

echo
echo "✔ Build complete"
echo "ISO: ${FINAL_ISO}"
echo "Log: ${DIST_DIR}/kiwi.log"
echo
echo "Next steps:"
echo "  - Test in UEFI/QEMU: qemu-system-x86_64 -enable-kvm -m 2048 -cpu host -machine q35,accel=kvm -bios /usr/share/OVMF/OVMF_CODE.fd -cdrom ${FINAL_ISO}"
echo "  - Install to disk, boot with Secure Boot enabled (shim+signed kernel from repo)."
echo