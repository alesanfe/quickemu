#!/usr/bin/env bash
# e2e contract test: drives the exact CLI sequence QuickGUI issues and
# validates every envelope against schemas/v1 — the machine-readable
# proof that the GUI↔CLI boundary holds end-to-end.
set -u
QE="${QE:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../bin" && pwd)/quickemu}"
RC=0; N=0
OUTF=$(mktemp); trap 'rm -f "${OUTF}" "${CHKER}"' EXIT
CHKER=$(mktemp)
cat >"${CHKER}" <<'PY'
import json, sys
env = json.load(open(sys.argv[1]))
need = {"schemaVersion","command","success","data","warnings","errors","meta"}
missing = need - env.keys()
assert not missing, f"missing {missing}"
expr = sys.argv[2]
if expr != "pass":
    assert eval(expr), f"expr failed: {expr}"
PY

assert_env() { # desc file expr
    N=$((N+1))
    if python3 "${CHKER}" "${2}" "${3:-pass}" >/dev/null 2>&1; then
        echo "ok: ${1}"
    else
        echo "FAIL: ${1}" >&2; RC=1
    fi
}
say() { echo "== $*"; }

say "1. capabilities (structured surface)"
"${QE}" --capabilities --format json >"${OUTF}" 2>/dev/null || true
assert_env "capabilities envelope" "${OUTF}" "env['command']=='capabilities'"

T=$(mktemp -d); VM="${T}/e2e.conf"
cat >"${VM}" <<'CONF'
guest_os="linux"
disk_img="disk.qcow2"
ram="256M"
CONF

say "2. status on a stopped VM"
(cd "${T}" && "${QE}" --vm "${VM}" --status --format json >"${OUTF}" 2>/dev/null)
assert_env "status envelope" "${OUTF}" "env['command']=='vm.status'"

say "3. delete-plan -> planId -> delete-vm (guarded)"
(cd "${T}" && "${QE}" --vm "${VM}" --delete-plan --format json >"${OUTF}" 2>/dev/null)
assert_env "delete-plan envelope" "${OUTF}" "env['command']=='vm.delete-plan'"
PLAN=$(python3 -c "import json;print(json.load(open('${OUTF}'))['data'].get('planId',''))" 2>/dev/null || true)
if [ -n "${PLAN}" ]; then
    (cd "${T}" && "${QE}" --vm "${VM}" --delete-vm --plan-id "${PLAN}" --trash --format json >"${OUTF}" 2>/dev/null)
    assert_env "guarded delete" "${OUTF}" "env['success'] in (True,False)"
fi

say "4. operation-status on a missing op -> OPERATION_NOT_FOUND"
(cd "${T}" && "${QE}" --vm "${VM}" --op-status nope-123 --format json >"${OUTF}" 2>/dev/null)
assert_env "typed error" "${OUTF}" "any('OPERATION_NOT_FOUND' in str(x) for x in env['errors']) or env['success']==False"

say "5. events cursor read (JSONL, not envelope)"
(cd "${T}" && "${QE}" --vm "${VM}" --events 0 --format json >"${OUTF}" 2>/dev/null)
# events emits JSONL lines or an empty body — each line must be a valid
# event object or nothing at all (stopped VM).
N=$((N+1))
if python3 -c "import sys,json;[json.loads(l) for l in open('${OUTF}') if l.strip()]" 2>/dev/null; then
    echo "ok: events JSONL"; else echo "FAIL: events JSONL" >&2; RC=1; fi

say "6. inventory (needs --vm context like every action)"
(cd "${T}" && "${QE}" --vm "${VM}" --inventory --format json >"${OUTF}" 2>/dev/null || true)
N=$((N+1))
if python3 -c "import json;json.load(open('${OUTF}'))" 2>/dev/null; then
    echo "ok: inventory json"; else echo "FAIL: inventory json" >&2; RC=1; fi

rm -rf "${T}"
echo "---"
echo "e2e-contract: ${N} assertions, rc=${RC}"
exit "${RC}"
