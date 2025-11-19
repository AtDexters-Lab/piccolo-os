#!/usr/bin/env bash
set -euo pipefail

TEMPLATE=piccolo-template
VDI=${1:?Usage: $0 /path/to/disk.vdi}

VM_NAME="piccolo-ephemeral-$(date +%s)"

echo "Creating ephemeral VM: $VM_NAME from template: $TEMPLATE"

# Clone template
VBoxManage clonevm "$TEMPLATE" \
  --name "$VM_NAME" \
  --register

# Attach the given disk (controller name must match template)
if ! VBoxManage storageattach "$VM_NAME" \
  --storagectl "SATA" \
  --port 0 --device 0 \
  --type hdd --medium "$VDI"; then
  echo "Failed to attach disk. Cleaning up VM: $VM_NAME"
  VBoxManage unregistervm "$VM_NAME" --delete
  exit 1
fi

# Start VM
VBoxManage startvm "$VM_NAME"

echo "VM $VM_NAME started."
echo "To destroy later:"
echo "  VBoxManage controlvm $VM_NAME poweroff; sleep 2; VBoxManage unregistervm $VM_NAME --delete"
