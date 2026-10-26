#!/usr/bin/env bash
cd ~/src/quickemu
D=/tmp/smokedbg.$$
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
timeout 25 ~/src/quickemu/bin/quickemu --vm smoke.conf --display none 2>&1 | head -30 &
sleep 15
ls -la smoke/ 2>/dev/null
~/src/quickemu/bin/quickemu --vm smoke.conf --status 2>&1 | head -5
