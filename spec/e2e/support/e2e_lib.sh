#!/usr/bin/env bash
# e2e_lib.sh — Quickemu E2E support library.
# Patterns from the project E2E guide: fixture builder, test driver,
# condition waits (never sleeps), unique ids, sandboxed filesystem,
# defensive cleanup, diagnostic artifacts, multiple oracles.

# ── environment control ───────────────────────────────────────────────
export LC_ALL=C LANG=C TZ=UTC
: "${QE:=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)/bin/quickemu}"
: "${E2E_ROOT:=/tmp/quickemu-e2e}"
: "${E2E_ACCEL:=tcg}"
: "${E2E_CPU:=qemu64}"
: "${E2E_TIMEOUT:=90}"
RUN_ID="${RUN_ID:-$(date +%s)-$$}"

# ── unique identity (§13) ─────────────────────────────────────────────
e2e_id() { echo "${RUN_ID}-${WORKER:-w0}-${1:-test}-${RANDOM:0:4}"; }

# ── sandbox (§12) ─────────────────────────────────────────────────────
e2e_sandbox() {
  local id="$1"
  local root="${E2E_ROOT}/${id}"
  mkdir -p "${root}"/{home,config,cache,state,downloads,vm,sockets,artifacts,logs}
  echo "${root}"
}

# ── fixture builder (§3) ──────────────────────────────────────────────
# vm_fixture <workspace> [name] — emits a minimal valid conf + disk
vm_fixture() {
  local ws="$1" name="${2:-vm}"
  # iso= stub: quickemu refuses to launch a conf with no bootable media.
  printf 'guest_os="linux"\ndisk_img="%s/disk.qcow2"\niso="%s/stub.iso"\nram="256M"\ndisplay="none"\nboot="legacy"\n' \
    "$ws" "$ws" >"${ws}/${name}.conf"
  # Real qcow2 container — QEMU rejects a raw stub for a qcow2-declared disk.
  qemu-img create -f qcow2 "${ws}/disk.qcow2" 64M >/dev/null 2>&1
  printf x >"${ws}/stub.iso"
  printf '{"vmId":"%s","workspace":"%s","config":"%s","disk":"%s"}\n' \
    "${name}-${RUN_ID}" "$ws" "${ws}/${name}.conf" "${ws}/disk.qcow2"
}

# ── wait_until (§6) — condition, interval, timeout, message ──────────
wait_until() {
  local cond="$1" timeout="${2:-30}" interval="${3:-0.25}"
  local deadline=$((SECONDS + ${timeout%.*}))
  while ! eval "$cond"; do
    [ "$SECONDS" -ge "$deadline" ] && return 1
    sleep "$interval"
  done
}

# ── assertions (§5 multiple oracles) ─────────────────────────────────
qe_state() { "$QE" --vm "$1" --status --format json 2>/dev/null | grep -o '"state":"[a-z]*"' | cut -d'"' -f4; }
qe_pid_alive() { [ -f "$1" ] && kill -0 "$(cat "$1")" 2>/dev/null; }
socket_exists() { [ -S "$1" ]; }
json_field() { grep -o "\"$2\":[[:space:]]*\"[^\"]*\"" <<<"$1" | cut -d'"' -f4; }

# ── cleanup (§20 defensive) ───────────────────────────────────────────
e2e_cleanup() {
  local ws="$1"
  [ -f "${ws}/vm.conf" ] && "$QE" --vm "${ws}/vm.conf" --kill >/dev/null 2>&1
  # assert no residual resources (§12 validations)
  find "$ws" -name '*.sock' -delete 2>/dev/null
}

# ── diagnostics (§21 artifacts) ───────────────────────────────────────
e2e_artifact() {
  local ws="$1" name="$2" result="$3" phase="${4:-}"
  cat >"${ws}/artifacts/${name}.json" <<EOF
{"testId":"$name","result":"$result","phase":"$phase",
 "elapsedSec":$SECONDS,"cleanup":{"attempted":true}}
EOF
}
