# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154,SC2317
# Sourced partial: shares the global namespace of bin/quickgetc.
# VM: config file generation and the create_vm orchestration.


function make_vm_config() {
    local CONF_FILE
    local ARCH
    local GUEST_OS
    # Add correct logic for disks later
    local DISK_IMG="${VM_PATH}/disk.qcow2"
    
    if [ "${OPERATION}" == "download" ]; then
        exit 0
    fi

    GUEST_OS=$(jq -r '.guest_os' <<< "${ENTRY}")
    ARCH=$(jq -r '.arch' <<< "${ENTRY}")
    RAM=$(jq -r '.ram // ""' <<< "${ENTRY}")

    CONF_FILE="${VM_PATH}.conf"

    if [ ! -e "${CONF_FILE}" ]; then
        echo "Making ${CONF_FILE}"
        cat << EOF > "${CONF_FILE}"
#!$(which quickemu) --vm
guest_os="${GUEST_OS}"
disk_img="${DISK_IMG}"
arch_vm="${ARCH}"
EOF
        echo " - Setting ${CONF_FILE} executable"
        chmod u+x "${CONF_FILE}"
        
        for IMAGE in "${!IMAGES[@]}"; do
            echo "${IMAGE}='"'${IMAGES["${IMAGE}"]}'"'" >> "${CONF_FILE}"
        done

        if [ -n "${RAM}" ]; then
            echo "ram=$((${RAM} / 1024 / 1024 / 1024))" >> "${CONF_FILE}"
        fi
    fi
}


function create_vm() {
    local DISKS

    declare -A IMAGES

    handle_sources
    make_vm_config
}
