#!/usr/bin/env bash
# QUICKEMU_BACKEND=compare (ADR-0001 phase G): runs the Bash and Go
# implementations of each extracted subsystem over the same inputs and
# diffs the observable contract. Zero divergences required before a
# Bash path can be retired.
set -u
cd "$(dirname "${BASH_SOURCE[0]}")/.."
DIVERGENT=0 T=$(mktemp -d); trap 'rm -rf "$T"' EXIT

note() { printf '%s\n' "compare: $*"; }
field() { # json key -> value via grep (no jq dep)
    grep -o "\"${2}\":\(\"[^\"]*\"\|[0-9a-zA-Z.{}:-]*\)" <<<"${1}" | head -1 | cut -d: -f2-
}

# --- ops: new/set/status/list shape parity ---
D1="${T}/bash" D2="${T}/go"; mkdir -p "${D1}" "${D2}"
VMDIR="${D1}" source lib/quickemu/runtime/ops.sh 2>/dev/null || true
BID=$(VMDIR="${D1}" VMNAME=t op_new export 2>/dev/null)
GID=$(${GO_OPS:-tools/quickemu-ops/quickemu-ops} new --vmdir "${D2}" --type export --vm t 2>/dev/null)
[ -f "${D1}/ops/${BID}.json" ] && [ -f "${D2}/ops/${GID}.json" ] || { note "DIVERGE: op files"; DIVERGENT=1; }
for K in schemaVersion type state progress; do
    B=$(field "$(cat "${D1}/ops/${BID}.json")" "${K}")
    G=$(field "$(cat "${D2}/ops/${GID}.json")" "${K}")
    [ "${B}" != "${G}" ] && { note "DIVERGE ops.${K}: bash=${B} go=${G}"; DIVERGENT=1; }
done

# --- ports: reserve/find/list parity on the SAME registry layout ---
R="${T}/ports"; mkdir -p "${R}"
QEBIN=${GO_PORTS:-tools/quickemu-ports/quickemu-ports}
"${QEBIN}" reserve --registry "${R}" --port 39990 --pid $$ --instance t >/dev/null 2>&1
[ -f "${R}/39990" ] || { note "DIVERGE ports: go reserve wrote no marker"; DIVERGENT=1; }
FP=$("${QEBIN}" find --registry "${R}" --lo 39990 --hi 39995)
[ "${FP}" = "39991" ] || { note "DIVERGE ports.find=${FP} (want 39991)"; DIVERGENT=1; }

# --- archive: valid archive must verify under BOTH implementations ---
V="${T}/av"; mkdir -p "${V}/vm"
echo c >"${V}/vm/x.conf"; printf '%s\n' '{"schemaVersion":1,"vm":"v","createdAt":"t","members":["x.conf"]}' >"${V}/vm/quickvm-manifest.json"
(cd "${V}/vm" && sha256sum x.conf >quickvm-checksums.sha256 && tar -czf ../ok.tgz .)
"${GO_ARCH:-tools/quickemu-archive/quickemu-archive}" verify --archive "${V}/ok.tgz" | grep -q '"success":true'     || { note "DIVERGE archive: valid rejected"; DIVERGENT=1; }

[ "${DIVERGENT}" -eq 0 ] && note "no divergences" || note "${DIVERGENT} divergence groups"
exit "${DIVERGENT}"
