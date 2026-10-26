#!/usr/bin/env bash
cd ~/src/quickemu
D=/tmp/qdbg.$$
mkdir -p "$D" && cd "$D"
qemu-img create -f qcow2 disk.qcow2 64M >/dev/null
dd if=/dev/zero of=smoke.iso bs=1M count=8 2>/dev/null
cat >smoke.conf <<'CONF'
guest_os="linux"
disk_img="disk.qcow2"
iso="smoke.iso"
ram="256M"
display="none"
monitor="socket"
network="none"
CONF
~/src/quickemu/bin/quickemu --vm smoke.conf --display none >boot.log 2>&1 &
BPID=$!
for i in $(seq 1 40); do
    [ -S smoke/smoke-monitor.socket ] && { echo "SOCKET UP after ${i}x0.5s"; break; }
    sleep 0.5
done
echo "=== sockets/files ==="; find . -type s; ls smoke/ 2>/dev/null
echo "=== qemu procs ==="; pgrep -af qemu-system | head -3
echo "=== boot tail ==="; tail -8 boot.log
[ -S smoke/smoke-monitor.socket ] && echo "PROBE: $(echo 'info status' | socat -t 2 - unix-connect:smoke/smoke-monitor.socket 2>&1 | head -2)"
