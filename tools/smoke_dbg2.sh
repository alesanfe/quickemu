#!/usr/bin/env bash
cd ~/src/quickemu
D=/tmp/sdbg2.$$
mkdir -p "$D" && cd "$D"
qemu-img create -f qcow2 disk.qcow2 64M >/dev/null
cat >smoke.conf <<'CONF'
guest_os="linux"
disk_img="disk.qcow2"
ram="256M"
display="none"
monitor="socket"
network="none"
CONF
timeout 20 ~/src/quickemu/bin/quickemu --vm smoke.conf --display none >boot.log 2>&1
echo "=== boot.log ==="; cat boot.log | head -40
echo "=== dir ==="; find . -type s -o -name "*.socket" -o -name "*.pid" | head
