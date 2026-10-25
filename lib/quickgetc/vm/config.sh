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

    GUEST_OS=$(jq -r '.guest_os // ""' <<< "${ENTRY}")
    if [ -z "${GUEST_OS}" ] && [ -n "${OS_ENTRY}" ]; then
        GUEST_OS=$(jq -r '.guest_os // ""' <<< "${OS_ENTRY}")
    fi
    GUEST_OS="${GUEST_OS:-linux}"
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
            echo "${IMAGE}='${IMAGES["${IMAGE}"]}'" >> "${CONF_FILE}"
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


# Build a VM config around an existing local or remote image
# (--create-config <name> <path|url>).
function create_config() {
    local VM_PATH="${1}"
    local INPUT="${2}"
    local IMAGE_KEY=""
    local GUEST_OS="linux"

    if [ -z "${VM_PATH}" ] || [ -z "${INPUT}" ]; then
        echo "ERROR! --create-config requires a VM name and a path or URL to an image."
        exit 1
    fi

    if ! mkdir "${VM_PATH}" 2>/dev/null; then
        echo "ERROR! Could not create directory: ${VM_PATH}. Please verify that it does not already exist."
        exit 1
    fi

    if [[ "${INPUT}" == "http://"* ]] || [[ "${INPUT}" == "https://"* ]]; then
        INPUT="$(web_redirect "${INPUT}")"
        case "${INPUT,,}" in
            *.iso|*.img|*.qcow2) ;;
            *)
                echo "ERROR! Only ISO, IMG and QCOW2 file types are supported for --create-config"
                exit 1;;
        esac
        web_get "${INPUT}" "${VM_PATH}"
        INPUT="${VM_PATH}/${INPUT##*/}"
    fi

    if [ ! -f "${INPUT}" ]; then
        echo "ERROR! The input must be a valid URL or path to an ISO, IMG, or QCOW2 file."
        exit 1
    fi

    case "${INPUT,,}" in
        *.iso)   IMAGE_KEY="iso";;
        *.img)   IMAGE_KEY="img";;
        *.qcow2) IMAGE_KEY="disk_img";;
        *)
            echo "ERROR! Only ISO, IMG and QCOW2 file types are supported for --create-config"
            exit 1;;
    esac

    mv "${INPUT}" "${VM_PATH}/" 2>/dev/null
    INPUT="$(basename "${INPUT}")"

    case "${INPUT,,}" in
        *windows*|*win*) GUEST_OS="windows";;
        *macos*)         GUEST_OS="macos";;
        *bsd*)           GUEST_OS="freebsd";;
    esac

    echo "Creating ${VM_PATH}.conf for ${INPUT} (guest_os=${GUEST_OS})."
    cat << EOF > "${VM_PATH}.conf"
#!$(which quickemu) --vm
guest_os="${GUEST_OS}"
${IMAGE_KEY}="${VM_PATH}/${INPUT}"
EOF
    chmod u+x "${VM_PATH}.conf"
    echo -e "\nTo start your virtual machine run:\n    quickemu --vm ${VM_PATH}.conf"
    exit 0
}
