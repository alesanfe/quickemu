#!/usr/bin/env bash
cd ~/src/quickemu
D=/tmp/qd2.$$
mkdir -p "$D" && cd "$D"
qemu-img create -f qcow2 disk.qcow2 64M >/dev/null
dd if=/dev/zero of=smoke.iso bs=1M count=8 2>/dev/null
export QEMU_ACCEL=tcg
cat >smoke.conf <<'CONF'
guest_os="linux"
cpu_model="qemu64"
disk_img="disk.qcow2"
iso="smoke.iso"
ram="256M"
display="none"
monitor="socket"
network="none"
CONF
~/src/quickemu/bin/quickemu --vm smoke.conf --display none >boot.log 2>&1 &
for i in $(seq 1 40); do [ -S smoke-monitor.socket ] && break; sleep 0.5; done
[ -S smoke-monitor.socket ] && echo SOCKET-OK || echo NO-SOCKET
pgrep -af qemu-system | grep -o 'accel [a-z]*' | head -1
tail -3 boot.log
