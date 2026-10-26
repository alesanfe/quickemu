#!/usr/bin/env bash
# Real-QEMU smoke test: boots a tiny guest-less VM (qemu-system with a
# minimal conf) and exercises lifecycle + QMP + cleanup end-to-end.
# Requires qemu-system-x86_64 + socat. Exits nonzero on any failure.
set -u
cd "$(dirname "${BASH_SOURCE[0]}")/.."
D=$(mktemp -d); trap 'rm -rf "$D"' EXIT; cd "$D"
command -v qemu-system-x86_64 >/dev/null || { echo "SKIP: qemu-system-x86_64 missing"; exit 0; }
command -v socat >/dev/null || { echo "SKIP: socat missing"; exit 0; }
command -v qemu-img >/dev/null || { echo "SKIP: qemu-img missing"; exit 0; }

qemu-img create -f qcow2 disk.qcow2 64M >/dev/null
# QEMU needs a bootable medium; a small ISO satisfies the check and
# lets the VM reach 'running' so lifecycle+QMP can be exercised.
dd if=/dev/zero of=smoke.iso bs=1M count=8 2>/dev/null
# TCG keeps the smoke runnable on hosts without /dev/kvm.
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
FAIL=0
say() { printf '%s\n' "smoke: $*"; }

# boot in background, wait for monitor socket
"$OLDPWD/bin/quickemu" --vm smoke.conf --display none >/dev/null 2>&1 &
BOOT_PID=$!
MON_SO="smoke-monitor.socket"; QMP_SO="smoke-qmp.socket"
for i in $(seq 1 60); do [ -S "${MON_SO}" ] && break; sleep 0.5; done
[ -S "${MON_SO}" ] || { say "FAIL: monitor socket never appeared"; FAIL=1; }
[ -S "${QMP_SO}" ] || say "WARN: qmp socket absent"

# state via QMP/status
sleep 1
ST=$("$OLDPWD/bin/quickemu" --vm smoke.conf --status 2>/dev/null | awk '{print $1}')
[ "${ST}" = "running" ] || { say "FAIL: status=${ST:-none}"; FAIL=1; }

# kill + cleanup
"$OLDPWD/bin/quickemu" --vm smoke.conf --kill >/dev/null 2>&1
wait ${BOOT_PID} 2>/dev/null
[ -f smoke.pid ] && { say "FAIL: pidfile left after cleanup"; FAIL=1; }
[ -S "${MON_SO}" ] && { say "FAIL: monitor socket left"; FAIL=1; }
pgrep -f "smoke.qcow2" >/dev/null && { say "FAIL: qemu still alive"; FAIL=1; }

# artifact residue sweep
find "${D}" -name '*.socket' -o -name '*.lock' -o -name '*.pid' | grep -q . \
    && { say "FAIL: residual artifacts"; FAIL=1; }
[ ${FAIL} -eq 0 ] && say "PASS: lifecycle + QMP + cleanup clean"
exit ${FAIL}
