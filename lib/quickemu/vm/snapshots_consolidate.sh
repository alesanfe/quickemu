# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Leaf consolidation for external snapshot chains (TD-007): commits a
# leaf overlay's diff into its backing parent and removes the node.

snapshot_consolidate() { # tag -> qemu-img commit leaf into parent
    local TAG="${1:-${SNAPSHOT_TAG}}"
    [ -z "${TAG}" ] && { echo "ERROR! 'snapshot consolidate' needs a tag." >&2; return 1; }
    vm_running "${VMDIR}" "${VMNAME}" && { echo "ERROR! Stop ${VMNAME} first." >&2; return 1; }
    local NODE_FILE PARENT
    NODE_FILE=$(snap_meta_read | sed -n "s/.*\"tag\":\"${TAG}\",\"file\":\"\([^\"]*\)\",\"parent\":\"\([^\"]*\)\".*/\1/p" | head -1)
    PARENT=$(snap_meta_read | sed -n "s/.*\"tag\":\"${TAG}\",\"file\":\"[^\"]*\",\"parent\":\"\([^\"]*\)\".*/\1/p" | head -1)
    [ -z "${NODE_FILE}" ] && { qe_die "vm.snapshot" SNAPSHOT_CONFLICT "external snapshot '${TAG}' not found" 5; }
    # Refuse non-leaf consolidation: children would lose their backing.
    snap_meta_read | grep -q "\"parent\":\"${TAG}\""         && { qe_die "vm.snapshot" SNAPSHOT_CONFLICT "'${TAG}' has children — consolidate leaves only" 12; }
    qemu-img commit "${VMDIR}/${NODE_FILE}" &&         rm -f "${VMDIR}/${NODE_FILE}" &&         snap_meta_remove "${TAG}" &&         qe_event "vm.snapshot-consolidate" "{\"tag\":\"$(json_escape "${TAG}")\",\"into\":\"$(json_escape "${PARENT}")\"}" &&         echo " - Consolidated '${TAG}' into ${PARENT}."
}
