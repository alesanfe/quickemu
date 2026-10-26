#!/usr/bin/env bash
cd ~/src/quickemu
D=/tmp/sdbg3.$$
mkdir -p "$D" && cd "$D"
qemu-img create -f qcow2 disk.qcow2 64M >/dev/null
# minimal bootable-ish ISO: QEMU just needs the device present
mkisofs -o smoke.iso -V SMOKE /dev/null 2>/dev/null || xorriso -as mkisofs -o smoke.iso /dev/null 2>/dev/null || dd if=/dev/zero of=smoke.iso bs=1M count=8 2>/dev/null
cat >smoke.conf <<'CONF'
guest_os="linux"
disk_img="disk.qcow2"
iso="smoke.iso"
ram="256M"
display="none"
monitor="socket"
network="none"
CONF
timeout 30 ~/src/quickemu/bin/quickemu --vm smoke.conf --display none >boot.log 2>&1 &
sleep 12
echo "=== sockets ==="; find . -type s | head
~/src/quickemu/bin/quickemu --vm smoke.conf --status 2>&1 | head -3
sleep 2; tail -15 boot.log
