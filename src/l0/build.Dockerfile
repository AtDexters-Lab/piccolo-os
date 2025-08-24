# Piccolo OS Builder Image
# This Dockerfile creates a reusable container with all KIWI dependencies.
# It's used by build_piccolo.sh to accelerate the build process.

# Allow the architecture to be passed in during the build.
ARG ARCH=x86_64

FROM registry.opensuse.org/opensuse/tumbleweed:latest

# Re-declare the ARG here to make it available to subsequent RUN instructions.
ARG ARCH

# 1. Set up base repositories
RUN zypper -n --gpg-auto-import-keys ar -f https://download.opensuse.org/tumbleweed/repo/oss/ oss && \
    zypper -n --gpg-auto-import-keys ar -f https://download.opensuse.org/tumbleweed/repo/non-oss/ non-oss && \
    zypper -n --gpg-auto-import-keys ar -f https://download.opensuse.org/update/tumbleweed/ update && \
    zypper -n ref

# 2. Install all build dependencies
# The ARCH argument is used here to install the correct EFI packages.
RUN zypper -n in --no-recommends \
    python3-kiwi \
    dracut \
    grub2 \
    grub2-${ARCH}-efi \
    shim \
    mokutil \
    btrfsprogs \
    dosfstools \
    e2fsprogs \
    parted \
    kpartx \
    xz \
    gzip \
    cpio \
    xorriso \
    squashfs \
    selinux-policy-targeted \
    policycoreutils \
    tpm2.0-tools

# 3. Set a default command (optional, but good practice)
CMD ["kiwi-ng", "--version"]
