# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# External snapshots: qcow2 overlay chains with persistent metadata at
# <vmdir>/snapshots.json. Each entry: id, tag, desc, createdAt, parent,
# file, type=external. The active node is the current disk_img.

snap_meta_file() { printf '%s/snapshots.json' "${VMDIR}"; }

snap_meta_read() { # -> raw array or []
    local F; F=$(snap_meta_file)
    [ -f "${F}" ] && cat "${F}" || printf '[]'
}

snap_meta_write() { # json-array -> atomic write
    local TMP; TMP="$(snap_meta_file).tmp"
    printf '%s\n' "${1}" >"${TMP}" && mv "${TMP}" "$(snap_meta_file)"
}

snap_next_id() {
    printf 'snap-%s' "$(date -u +%Y%m%d%H%M%S)-$$"
}

# Register a new external snapshot node.
snap_register() { # tag file parent desc
    local ARR ID
    ID=$(snap_next_id)
    ARR=$(snap_meta_read)
    ARR="${ARR%]}"
    [ "${ARR}" != "[" ] && ARR+=","
    ARR+="{\"id\":\"${ID}\",\"tag\":\"$(json_escape "${1}")\",\"file\":\"$(json_escape "${2}")\",\"parent\":\"$(json_escape "${3}")\",\"desc\":\"$(json_escape "${4}")\",\"type\":\"external\",\"createdAt\":\"$(date -u +%Y-%m-%dT%H:%M:%SZ)\"}]"
    snap_meta_write "${ARR}"
}

# --snapshot-create <tag> --external: create a qcow2 overlay backed by
# the current disk, repoint the conf to the overlay, register metadata.
snapshot_create_external() { # tag [desc]
    local TAG="${1}" DESC="${2:-}" PARENT OVERLAY
    [ -z "${TAG}" ] && { echo "ERROR! No snapshot tag." >&2; return 1; }
    [ -f "${disk_img}" ] || { echo "ERROR! ${disk_img} not found." >&2; return 1; }
    if vm_running "${VMDIR}" "${VMNAME}"; then
        echo "ERROR! Stop ${VMNAME} first (external snapshot changes the chain)." >&2; return 1
    fi
    snap_meta_read | grep -q "\"tag\":\"${TAG}\"" && { qe_die "vm.snapshot" SNAPSHOT_CONFLICT "external snapshot '${TAG}' already exists" 12; }
    PARENT="${disk_img}"
    OVERLAY="${VMDIR}/${VMNAME}-${TAG}.qcow2"
    [ -e "${OVERLAY}" ] && { echo "ERROR! ${OVERLAY} already exists." >&2; return 1; }
    require_qemu_img
    ${QEMU_IMG} create -f qcow2 -b "${PARENT}" -F "${disk_format:-qcow2}" "${OVERLAY}" >/dev/null \
        || { echo "ERROR! qemu-img failed to create ${OVERLAY}." >&2; return 1; }
    # Repoint the conf's disk_img at the overlay so the VM boots on it.
    if grep -q "disk_img=" "${VM}" 2>/dev/null; then
        local ESC
        ESC=$(printf '%s' "${disk_img}" | sed 's/[&|\]/\&/g')
        sed -i "s|disk_img=\"${ESC}\"|disk_img=\"${OVERLAY}\"|" "${VM}" 2>/dev/null || true
    fi
    disk_img="${OVERLAY}"
    snap_register "${TAG}" "${OVERLAY}" "${PARENT}" "${DESC}"
    qe_event "vm.snapshot-external" "{\"tag\":\"$(json_escape "${TAG}")\",\"file\":\"$(json_escape "${OVERLAY}")\"}"
    echo " - External snapshot '${TAG}' -> ${OVERLAY} (parent ${PARENT})"
}

# --snapshot-tree: render the overlay chain from metadata.
snapshot_tree() {
    local ARR
    ARR=$(snap_meta_read)
    if [ "${OUTPUT_FORMAT:-}" == "json" ]; then
        qe_ok "vm.snapshot.tree" "{\"active\":\"$(json_escape "${disk_img:-}")\",\"snapshots\":${ARR}}"
        return
    fi
    echo "Snapshot chain for ${VMNAME} (active: ${disk_img:-none}):"
    echo "${ARR}" | grep -o '"tag":"[^"]*"\|"file":"[^"]*"\|"parent":"[^"]*"' | paste - - - 2>/dev/null \
        | sed 's/"tag":"\(.*\)"\s*"file":"\(.*\)"\s*"parent":"\(.*\)"/ - \1\n   \2 <- \3/'
    [ "$(snap_meta_read)" == "[]" ] && echo " - (no external snapshots)"
}

# --snapshot-verify: every registered overlay must exist and its
# qemu-img backing chain must match the recorded parent.
snapshot_verify() {
    local ARR RC=0 F P ACTUAL
    ARR=$(snap_meta_read)
    echo "${ARR}" | grep -o '"file":"[^"]*"\|"parent":"[^"]*"' | paste - - 2>/dev/null | \
    while IFS= read -r LINE; do
        F=$(printf '%s' "${LINE}" | sed -n 's/.*"file":"\([^"]*\)".*/\1/p')
        P=$(printf '%s' "${LINE}" | sed -n 's/.*"parent":"\([^"]*\)".*/\1/p')
        if [ ! -f "${F}" ]; then
            echo "ERROR! Snapshot overlay ${F} missing." >&2; RC=1; continue
        fi
        ACTUAL=$(qemu-img info "${F}" 2>/dev/null | sed -n 's/^backing file: //p')
        [ -n "${P}" ] && [ "${ACTUAL}" != "${P}" ] && {
            echo "ERROR! ${F} backs ${ACTUAL:-nothing}, expected ${P}." >&2; RC=1; }
    done
    [ "${RC}" -eq 0 ] && echo " - Snapshot chain verified."
    return ${RC}
}
