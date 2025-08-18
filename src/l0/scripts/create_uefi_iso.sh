#!/bin/bash
# Piccolo OS UEFI ISO Creation Script
# Creates UEFI-bootable live ISO from Flatcar QEMU UEFI components
# Based on comprehensive research and validated approach

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BASE_DIR="$(dirname "$SCRIPT_DIR")"
BUILD_DIR="$BASE_DIR/build"
WORK_DIR="/tmp/piccolo_uefi_build"
TOOLKIT_SCRIPT="$SCRIPT_DIR/uefi_toolkit.sh"

# Color output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Logging functions
log_info() { echo -e "${BLUE}[INFO]${NC} $*"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $*"; }
log_warning() { echo -e "${YELLOW}[WARNING]${NC} $*"; }
log_error() { echo -e "${RED}[ERROR]${NC} $*"; }
log_step() { echo -e "${GREEN}[STEP]${NC} $*"; }

# Error handling
cleanup() {
    local exit_code=$?
    if [[ $exit_code -ne 0 ]]; then
        log_error "Script failed with exit code $exit_code"
        log_info "Work directory preserved for debugging: $WORK_DIR"
    fi
    
    # Unmount any mounted filesystems
    for mount_point in "$WORK_DIR/efi_mount" "$WORK_DIR/usr_mount"; do
        if mountpoint -q "$mount_point" 2>/dev/null; then
            log_info "Unmounting $mount_point"
            sudo umount "$mount_point" 2>/dev/null || true
        fi
    done
    
    # Detach loop devices
    for loop_dev in $(losetup -a | grep "$WORK_DIR" | cut -d: -f1); do
        log_info "Detaching loop device $loop_dev"
        sudo losetup -d "$loop_dev" 2>/dev/null || true
    done
}

trap cleanup EXIT

# Validate inputs and environment
validate_environment() {
    log_step "Validating environment and dependencies"
    
    # Check toolkit is available
    if [[ ! -x "$TOOLKIT_SCRIPT" ]]; then
        log_error "Toolkit script not found or not executable: $TOOLKIT_SCRIPT"
        exit 1
    fi
    
    # Run dependency check
    if ! "$TOOLKIT_SCRIPT" check-deps >/dev/null 2>&1; then
        log_error "Dependency check failed"
        exit 1
    fi
    
    # Check for sudo capability
    if ! sudo -n true 2>/dev/null; then
        log_error "This script requires sudo access for mounting filesystems"
        log_info "Please run: sudo -v"
        exit 1
    fi
    
    log_success "Environment validation complete"
}

# Find and validate source images
find_source_images() {
    log_step "Locating source images"
    
    # Find QEMU UEFI image
    local qemu_pattern="$BUILD_DIR/work-*/scripts/__build__/images/images/*/piccolo-stable-*/flatcar_production_qemu_uefi_image.img.bz2"
    QEMU_UEFI_IMAGE=$(find $qemu_pattern 2>/dev/null | head -1)
    
    if [[ -z "$QEMU_UEFI_IMAGE" ]]; then
        log_error "QEMU UEFI image not found. Pattern: $qemu_pattern"
        log_info "Have you run the Piccolo OS build (./build.sh)?"
        exit 1
    fi
    
    log_success "Found QEMU UEFI image: $QEMU_UEFI_IMAGE"
    
    # Basic validation - check bzip2 integrity
    log_info "Validating QEMU UEFI image integrity..."
    if ! lbzip2 -t "$QEMU_UEFI_IMAGE" >/dev/null 2>&1; then
        log_error "QEMU UEFI image validation failed - file is corrupted"
        exit 1
    fi
    
    log_success "Source image validation complete"
}

# Extract UEFI components from QEMU image
extract_uefi_components() {
    log_step "Extracting UEFI components from QEMU image"
    
    # Create work directories
    mkdir -p "$WORK_DIR"/{raw_image,efi_mount,usr_mount,iso_build/{EFI/BOOT,flatcar,live}}
    
    # Convert QEMU image to raw format
    log_info "Decompressing QEMU UEFI image..."
    lbzip2 -dc "$QEMU_UEFI_IMAGE" > "$WORK_DIR/raw_image/uefi.qcow2"
    
    log_info "Converting QEMU image to raw format..."
    qemu-img convert -f qcow2 -O raw "$WORK_DIR/raw_image/uefi.qcow2" "$WORK_DIR/raw_image/uefi.img"
    
    # Analyze partition structure
    log_info "Analyzing partition structure..."
    local partition_info
    partition_info=$(fdisk -l "$WORK_DIR/raw_image/uefi.img" 2>/dev/null)
    echo "$partition_info" > "$WORK_DIR/partition_info.txt"
    
    # Extract EFI partition (partition 1, typically starts at sector 4096)
    log_info "Extracting EFI system partition..."
    local efi_start efi_size
    efi_start=$(echo "$partition_info" | awk '/EFI System/ {print $2}')
    efi_size=$(echo "$partition_info" | awk '/EFI System/ {print $4}')
    
    if [[ -z "$efi_start" || -z "$efi_size" ]]; then
        log_error "Could not determine EFI partition boundaries"
        echo "$partition_info"
        exit 1
    fi
    
    # Extract EFI partition to separate file
    dd if="$WORK_DIR/raw_image/uefi.img" of="$WORK_DIR/efi.img" \
       bs=512 skip="$efi_start" count="$efi_size" status=progress
    
    # Mount EFI partition
    log_info "Mounting EFI partition..."
    sudo mount -o loop,ro "$WORK_DIR/efi.img" "$WORK_DIR/efi_mount"
    
    # Copy UEFI bootloader components
    log_info "Copying UEFI bootloader components..."
    sudo cp -r "$WORK_DIR/efi_mount/EFI/boot/"* "$WORK_DIR/iso_build/EFI/BOOT/"
    
    # Extract kernel from EFI partition
    if [[ -f "$WORK_DIR/efi_mount/flatcar/vmlinuz-a" ]]; then
        sudo cp "$WORK_DIR/efi_mount/flatcar/vmlinuz-a" "$WORK_DIR/iso_build/flatcar/vmlinuz"
        log_success "Kernel extracted: vmlinuz-a"
    else
        log_error "Kernel not found in EFI partition"
        exit 1
    fi
    
    # Unmount EFI partition
    sudo umount "$WORK_DIR/efi_mount"
    
    log_success "UEFI components extracted successfully"
}

# Extract USR partition and create live filesystem
create_live_filesystem() {
    log_step "Creating live filesystem from USR partition"
    
    # Extract USR-A partition (partition 3, typically)
    log_info "Extracting USR-A partition..."
    local partition_info usr_start usr_size
    partition_info=$(cat "$WORK_DIR/partition_info.txt")
    
    # Find USR-A partition (partition 3, 1G size, "unknown" type)
    usr_start=$(echo "$partition_info" | awk '/uefi.img3/ {print $2}')
    usr_size=$(echo "$partition_info" | awk '/uefi.img3/ {print $4}')
    
    if [[ -z "$usr_start" || -z "$usr_size" ]]; then
        log_error "Could not determine USR-A partition boundaries"
        echo "$partition_info"
        exit 1
    fi
    
    # Extract USR partition
    dd if="$WORK_DIR/raw_image/uefi.img" of="$WORK_DIR/usr.img" \
       bs=512 skip="$usr_start" count="$usr_size" status=progress
    
    # Mount USR partition
    log_info "Mounting USR partition..."
    sudo mount -o loop,ro "$WORK_DIR/usr.img" "$WORK_DIR/usr_mount"
    
    # Create squashfs live filesystem
    log_info "Creating SquashFS live filesystem..."
    mksquashfs "$WORK_DIR/usr_mount" "$WORK_DIR/iso_build/live/filesystem.squashfs" \
        -comp xz -Xbcj x86 -b 1M -Xdict-size 1M
    
    # Unmount USR partition
    sudo umount "$WORK_DIR/usr_mount"
    
    local squashfs_size
    squashfs_size=$(stat -c%s "$WORK_DIR/iso_build/live/filesystem.squashfs" | numfmt --to=iec)
    log_success "Live filesystem created: $squashfs_size"
}

# Create live boot initrd
create_live_initrd() {
    log_step "Creating live boot initrd"
    
    local initrd_dir="$WORK_DIR/initrd"
    rm -rf "$initrd_dir"  # Clean any previous initrd
    mkdir -p "$initrd_dir"/{bin,sbin,lib,lib64,dev,proc,sys,run,mnt,tmp,etc,var,usr,root}
    
    # Create device nodes
    sudo mknod "$initrd_dir/dev/null" c 1 3 2>/dev/null || true
    sudo mknod "$initrd_dir/dev/zero" c 1 5 2>/dev/null || true
    sudo mknod "$initrd_dir/dev/console" c 5 1 2>/dev/null || true
    
    # Copy busybox and create symlinks
    local busybox_path
    busybox_path=$(which busybox)
    cp "$busybox_path" "$initrd_dir/bin/"
    
    # Create busybox symlinks for essential commands
    for cmd in sh mount umount mkdir rmdir cp mv rm ls cat grep awk sed cut tr sort uniq head tail find xargs; do
        ln -sf busybox "$initrd_dir/bin/$cmd"
    done
    
    # Copy essential libraries (only if busybox is dynamically linked)
    if ldd "$busybox_path" >/dev/null 2>&1; then
        log_info "Busybox is dynamically linked, copying libraries..."
        local libs
        libs=$(ldd "$busybox_path" | awk '/=>/ {print $3}' | grep -v "not found")
        for lib in $libs; do
            if [[ -f "$lib" ]]; then
                local lib_dir lib_name
                lib_dir=$(dirname "$lib")
                lib_name=$(basename "$lib")
                mkdir -p "$initrd_dir$lib_dir"
                cp "$lib" "$initrd_dir$lib_dir/"
            fi
        done
        
        # Copy dynamic linker
        local ld_linux
        ld_linux=$(ldd "$busybox_path" | awk '/ld-linux/ {print $1}')
        if [[ -f "$ld_linux" ]]; then
            cp "$ld_linux" "$initrd_dir/lib64/"
        fi
    else
        log_info "Busybox is statically linked, no libraries needed"
    fi
    
    # Create init script
    cat > "$initrd_dir/init" << 'EOF'
#!/bin/sh
# Piccolo OS Live Boot Init Script

# Mount essential filesystems
/bin/mount -t proc proc /proc
/bin/mount -t sysfs sysfs /sys
/bin/mount -t devtmpfs devtmpfs /dev
/bin/mount -t tmpfs tmpfs /run

echo "Piccolo OS Live Boot Starting..."

# Find and mount ISO device
ISO_DEV=""
for dev in /dev/sr0 /dev/cdrom /dev/dvd $(ls /dev/sd* 2>/dev/null); do
    if [ -b "$dev" ]; then
        echo "Trying to mount $dev..."
        if /bin/mount -o ro "$dev" /mnt 2>/dev/null; then
            if [ -f "/mnt/live/filesystem.squashfs" ]; then
                ISO_DEV="$dev"
                echo "Found Piccolo OS live filesystem on $dev"
                break
            fi
            /bin/umount /mnt
        fi
    fi
done

if [ -z "$ISO_DEV" ]; then
    echo "ERROR: Could not find Piccolo OS live filesystem"
    echo "Dropping to emergency shell..."
    exec /bin/sh
fi

# Mount live filesystem
echo "Mounting live filesystem..."
/bin/mkdir -p /live/lower /live/upper /live/work /live/merged

if ! /bin/mount -o loop,ro /mnt/live/filesystem.squashfs /live/lower; then
    echo "ERROR: Failed to mount live filesystem"
    exec /bin/sh
fi

# Create overlay filesystem
/bin/mount -t tmpfs tmpfs /live/upper
/bin/mkdir -p /live/upper/upper /live/upper/work

if ! /bin/mount -t overlay overlay \
    -o lowerdir=/live/lower,upperdir=/live/upper/upper,workdir=/live/upper/work \
    /live/merged; then
    echo "ERROR: Failed to create overlay filesystem"
    exec /bin/sh
fi

# Switch to live system
echo "Switching to live system..."
cd /live/merged

# Mount essential filesystems in new root
/bin/mount --move /proc proc/
/bin/mount --move /sys sys/
/bin/mount --move /dev dev/
/bin/mount --move /run run/

# Switch root and start system
exec /bin/busybox switch_root . /sbin/init
EOF
    
    chmod +x "$initrd_dir/init"
    
    # Create initrd archive
    log_info "Creating initrd archive..."
    (cd "$initrd_dir" && find . | cpio -o -H newc | gzip -9) > "$WORK_DIR/iso_build/flatcar/cpio.gz"
    
    local initrd_size
    initrd_size=$(stat -c%s "$WORK_DIR/iso_build/flatcar/cpio.gz" | numfmt --to=iec)
    log_success "Live initrd created: $initrd_size"
}

# Create GRUB configuration
create_grub_config() {
    log_step "Creating GRUB configuration for UEFI boot"
    
    mkdir -p "$WORK_DIR/iso_build/flatcar/grub"
    
    cat > "$WORK_DIR/iso_build/flatcar/grub/grub.cfg" << 'EOF'
set timeout=10
set default=0

menuentry "Piccolo OS Live (UEFI)" {
    echo "Loading Piccolo OS Live kernel..."
    linux /flatcar/vmlinuz console=tty0 console=ttyS0,115200n8 ro noswap flatcar.oem.id=qemu flatcar.autologin
    echo "Loading Piccolo OS Live initrd..."
    initrd /flatcar/cpio.gz
    echo "Booting Piccolo OS Live..."
}

menuentry "Piccolo OS Live (UEFI) - Debug Mode" {
    echo "Loading Piccolo OS Live kernel (debug)..."
    linux /flatcar/vmlinuz console=tty0 console=ttyS0,115200n8 ro noswap flatcar.oem.id=qemu flatcar.autologin debug systemd.log_level=debug
    echo "Loading Piccolo OS Live initrd..."
    initrd /flatcar/cpio.gz
    echo "Booting Piccolo OS Live (debug mode)..."
}
EOF
    
    log_success "GRUB configuration created"
}

# Create UEFI ISO
create_uefi_iso() {
    log_step "Creating UEFI ISO image"
    
    local version="${1:-1.0.0}"
    local output_dir="$BUILD_DIR/output/$version"
    local iso_file="$output_dir/piccolo-os-uefi-$version.iso"
    
    mkdir -p "$output_dir"
    
    # Create ISO with UEFI boot support
    log_info "Building ISO with xorriso..."
    xorriso -as mkisofs \
        -iso-level 3 \
        -volid "PICCOLO_OS" \
        -appid "Piccolo OS UEFI Live" \
        -publisher "Piccolo OS Project" \
        -preparer "Piccolo OS Build System" \
        -eltorito-alt-boot \
        -e EFI/BOOT/bootx64.efi \
        -no-emul-boot \
        -isohybrid-gpt-basdat \
        -output "$iso_file" \
        "$WORK_DIR/iso_build"
    
    if [[ ! -f "$iso_file" ]]; then
        log_error "ISO creation failed"
        exit 1
    fi
    
    # Validate created ISO
    log_info "Validating created ISO..."
    if ! "$TOOLKIT_SCRIPT" analyze-iso "$iso_file" >/dev/null 2>&1; then
        log_warning "ISO validation warnings detected"
    fi
    
    local iso_size
    iso_size=$(stat -c%s "$iso_file" | numfmt --to=iec)
    log_success "UEFI ISO created: $iso_file ($iso_size)"
    
    echo
    log_success "=== PICCOLO OS UEFI ISO CREATION COMPLETE ==="
    log_info "ISO Location: $iso_file"
    log_info "ISO Size: $iso_size"
    log_info "Boot Type: UEFI-only"
    log_info "Ready for testing with:"
    log_info "  QEMU UEFI: qemu-system-x86_64 -bios /usr/share/ovmf/OVMF.fd -cdrom $iso_file"
    log_info "  Real hardware: Boot from USB/CD in UEFI mode"
}

# Main execution
main() {
    local version="${1:-1.0.0}"
    
    echo "Piccolo OS UEFI ISO Creation Script"
    echo "===================================="
    echo
    
    validate_environment
    find_source_images
    extract_uefi_components
    create_live_filesystem
    create_live_initrd
    create_grub_config
    create_uefi_iso "$version"
    
    # Cleanup work directory on success
    if [[ -d "$WORK_DIR" ]]; then
        rm -rf "$WORK_DIR"
        log_info "Work directory cleaned up"
    fi
}

# Execute main function
main "$@"