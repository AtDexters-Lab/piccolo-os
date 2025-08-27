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

# 2. Install KIWI with complete dependency system (automatically handles all requirements)
RUN zypper -n in --no-recommends \
    python3-kiwi \
    kiwi-systemdeps \
    parted \
    dolly

# 3. Pre-populate repository cache to speed up builds
RUN zypper -n ref --force && \
    zypper clean --all

# 4. Pre-install common packages to speed up builds (skip problematic MicroOS patterns)
RUN zypper -n in --no-recommends \
    coreutils \
    gawk \
    gzip \
    hostname \
    openssl \
    filesystem \
    glibc-locale-base \
    ca-certificates-mozilla \
    kernel-default \
    NetworkManager \
    chrony \
    podman \
    conmon \
    crun \
    dracut-kiwi-live \
    shim \
    grub2 \
    grub2-x86_64-efi \
    ucode-amd \
    ucode-intel \
    || true

# 5. Set a default command (optional, but good practice)
CMD ["kiwi-ng", "--version"]
