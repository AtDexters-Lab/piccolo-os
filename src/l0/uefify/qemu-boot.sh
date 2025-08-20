isodir="$(dirname "$(realpath $0)")/../build/output/uefi"
isopath=${isodir}/$(ls -t ${isodir} | head -1)
echo "Using ISO path: $isopath"
sudo qemu-system-x86_64 \
  -m 2048 \
  -cpu qemu64 \
  -smp 2 \
  -drive if=pflash,format=raw,readonly=on,file=/usr/share/OVMF/OVMF_CODE_4M.fd \
  -drive if=pflash,format=raw,file=/usr/share/OVMF/OVMF_VARS_4M.fd \
  -cdrom ${isopath} \
  -boot d \
  -nic user,model=virtio-net-pci \
  -serial mon:stdio