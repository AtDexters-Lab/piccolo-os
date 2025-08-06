# Building Piccolo OS

This guide covers building the complete Piccolo OS system from source.

## Prerequisites

### System Requirements
- **OS**: Linux (Ubuntu 20.04+ or similar)
- **CPU**: x86_64 with KVM virtualization support  
- **RAM**: 16GB+ recommended for builds
- **Storage**: 100GB+ free space for build artifacts
- **Network**: Internet access for downloading dependencies

### Required Tools
```bash
# Ubuntu/Debian
sudo apt-get update
sudo apt-get install -y \
    build-essential \
    git \
    curl \
    qemu-system-x86 \
    docker.io \
    gpg

# Add user to docker group
sudo usermod -aG docker $USER
# Log out and back in for group changes to take effect
```

### Go Development (for L1 only)
```bash
# Install Go 1.21+
curl -L https://go.dev/dl/go1.21.0.linux-amd64.tar.gz | sudo tar -C /usr/local -xzf -
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc
```

## Repository Setup

```bash
# Clone the repository
git clone https://github.com/your-org/piccolo-os.git
cd piccolo-os

# Review build configuration
cat src/l0/piccolo.env
```

## Building Components

### Option 1: Complete System Build

Build the entire Piccolo OS including ISO generation:

```bash
cd src/l0
./build.sh
```

This process:
1. Builds the `piccolod` binary from Layer 1
2. Creates a custom Flatcar Linux image with `piccolod` integrated
3. Generates bootable ISO and update artifacts  
4. Signs artifacts with GPG (if configured)
5. Places outputs in `build/output/{version}/`

**Build time**: 60-90 minutes on first run, 20-30 minutes on subsequent runs.

### Option 2: Layer 1 Only (Development)

For faster iteration during `piccolod` development:

```bash
cd src/l1/piccolod

# Build binary
./build.sh [VERSION]

# Or use Go directly
go build ./cmd/piccolod
go test ./...
go mod tidy
```

## Build Configuration

### Environment Variables

Edit `src/l0/piccolo.env` to configure the build:

```bash
# GPG signing (optional)
GPG_SIGNING_KEY_ID=your-key-id

# Update server configuration  
PICCOLO_UPDATE_SERVER=https://your-update-server.com
PICCOLO_UPDATE_GROUP=piccolo-stable

# Build options
FLATCAR_VERSION=3815.2.0
PICCOLO_VERSION=1.0.0
```

### GPG Signing Setup

To enable artifact signing:

```bash
# Generate a signing key (if needed)
gpg --full-generate-key

# List keys and note the key ID
gpg --list-secret-keys --keyid-format LONG

# Configure in piccolo.env
echo "GPG_SIGNING_KEY_ID=YOUR_KEY_ID" >> src/l0/piccolo.env
```

## Build Outputs

### Complete Build Artifacts

After a successful build, find artifacts in `src/l0/build/output/{version}/`:

```bash
build/output/1.0.0/
├── piccolo-os-live-1.0.0.iso      # Bootable live ISO
├── piccolo-os-live-1.0.0.iso.sig  # GPG signature
├── piccolo-os-1.0.0.bin           # Update payload
├── piccolo-os-1.0.0.bin.sig       # GPG signature  
├── checksums.txt                  # SHA256 checksums
└── build-info.json               # Build metadata
```

### Layer 1 Build Artifacts  

`piccolod` binary builds are placed in `src/l1/piccolod/build/`:

```bash
src/l1/piccolod/build/
├── piccolod-linux-amd64-1.0.0    # Statically linked binary
└── version.json                   # Version metadata
```

## Testing Builds

### Automated Testing

Test the complete OS build in a VM:

```bash
cd src/l0
./test_piccolo_os_image.sh \
    --build-dir ./build/output/1.0.0 \
    --version 1.0.0
```

This script:
- Boots the ISO in QEMU  
- Verifies `piccolod` binary and service status
- Tests HTTP API functionality
- Validates container runtime integration
- Confirms proper root user execution

### Manual Testing

Boot the ISO manually for interactive testing:

```bash
# Boot in QEMU with console access
qemu-system-x86_64 \
    -m 2048 \
    -cpu host \
    -enable-kvm \
    -netdev user,id=eth0,hostfwd=tcp::2222-:22,hostfwd=tcp::8080-:8080 \
    -device virtio-net-pci,netdev=eth0 \
    -cdrom build/output/1.0.0/piccolo-os-live-1.0.0.iso \
    -boot d
```

Access via SSH or HTTP:
```bash  
# SSH access (after boot completes)
ssh -p 2222 core@localhost

# HTTP API access
curl http://localhost:8080/api/v1/version
```

## Development Workflow

### Rapid Iteration

For development, use the Layer 1 build for faster iteration:

```bash
# Make changes to src/l1/piccolod/
cd src/l1/piccolod
go build ./cmd/piccolod

# Test changes
go test ./...

# Full integration test (slower)
cd ../../l0
./build.sh && ./test_piccolo_os_image.sh --build-dir ./build/output/1.0.0 --version 1.0.0
```

### Debugging Builds

Common build issues and solutions:

```bash
# Clean build artifacts  
cd src/l0
rm -rf build/

# Verbose build output
VERBOSE=1 ./build.sh

# Check Docker daemon status
sudo systemctl status docker

# Verify KVM acceleration
ls -l /dev/kvm
```

## Cross-Platform Builds

Currently only `linux/amd64` is supported. ARM64 support is planned for future releases.

For more information, see:
- [Testing Procedures](testing.md)
- [API Design](api-design.md) 
- [Architecture Overview](../architecture/overview.md)