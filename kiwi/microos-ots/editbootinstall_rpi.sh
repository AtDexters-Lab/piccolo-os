#!/bin/bash
set -euxo pipefail

diskname=$1
devname="$2"
loopname="${devname%*p?}"
loopdev=${loopname#/dev/mapper/*}

#==========================================
# copy Raspberry Pi firmware to EFI partition
#------------------------------------------
echo "RPi EFI system, installing firmware on ESP"
mkdir -p ./mnt-pi
mount ${loopname}p1 ./mnt-pi
( cd boot/vc; tar c . ) | ( cd ./mnt-pi/; tar x )
umount ./mnt-pi
rmdir ./mnt-pi

#==========================================
# Change partition label type to MBR
#------------------------------------------
#
# The RPi4 bootrom only boots SD cards with MBR partition tables.
# GPT SD card boot is unreliable (rpi-eeprom#654). Convert GPT→MBR
# and set the ESP partition to type 0xc (W95 FAT32 LBA) so the
# firmware detects it as the boot partition.
#
# Use tabs, "<<-" strips tabs, but no other whitespace!
cat > gdisk.tmp <<-'EOF'
		x
		r
		g
		t
		1
		c
		w
		y
	EOF
dd if=$loopdev of=mbrid.bin bs=1 skip=440 count=4
gdisk $loopdev < gdisk.tmp
dd of=$loopdev if=mbrid.bin bs=1 seek=440 count=4
rm -f mbrid.bin
rm -f gdisk.tmp
