# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154,SC2317
# Sourced partial: shares the global namespace of bin/quickget.
# VM: config file generation and the create_vm orchestration.


function make_vm_config() {
    local CONF_FILE=""
    local IMAGE_FILE=""
    local ISO_FILE=""
    local IMAGE_TYPE=""
    local GUEST=""
    if [ "${OPERATION}" == "download" ]; then
        exit 0
    fi
    IMAGE_FILE="${1}"
    ISO_FILE="${2}"
    case "${OS}" in
        batocera)
            GUEST="batocera"
            IMAGE_TYPE="img";;
        custom)
            GUEST="${CUSTOM_OS}"
            IMAGE_TYPE="${CUSTOM_IMAGE_TYPE}";;
        dragonflybsd)
            GUEST="dragonflybsd"
            IMAGE_TYPE="iso";;
        easyos)
            GUEST="linux"
            IMAGE_TYPE="img";;
        freebsd|ghostbsd)
            GUEST="freebsd"
            IMAGE_TYPE="iso";;
        haiku)
            GUEST="haiku"
            IMAGE_TYPE="iso";;
        freedos)
            GUEST="freedos"
            IMAGE_TYPE="iso";;
        kolibrios)
            GUEST="kolibrios"
            IMAGE_TYPE="iso";;
        macos)
            GUEST="macos"
            IMAGE_TYPE="img";;
        netbsd)
            GUEST="netbsd"
            IMAGE_TYPE="iso";;
        openbsd)
            GUEST="openbsd"
            IMAGE_TYPE="iso";;
        openfyde)
            GUEST="linux"
            IMAGE_TYPE="img";;
        openindiana)
            GUEST="solaris"
            IMAGE_TYPE="iso";;
        reactos)
            GUEST="reactos"
            IMAGE_TYPE="iso";;
        templeos)
            GUEST="templeos"
            IMAGE_TYPE="iso";;
        ubuntu*)
            GUEST="linux"
            IMAGE_TYPE="iso"
            # If there is a point in the release, check if it is less than 16.04
            if [[ "${RELEASE}" != *"daily"* ]]; then
                if [ "${RELEASE//./}" -lt 1604 ]; then
                    GUEST="linux_old"
                fi
            fi
            ;;
        windows)
            GUEST="windows"
            IMAGE_TYPE="iso";;
        windows-server)
            GUEST="windows-server"
            IMAGE_TYPE="iso";;
        *)
            GUEST="linux"
            IMAGE_TYPE="iso";;
    esac

    # A downloaded disk image overrides the per-OS IMAGE_TYPE: it becomes the
    # VM disk itself rather than installer media.
    local DISK_FORMAT="qcow2" DISK_LINE="disk_img=\"${VM_PATH}/disk.qcow2\"" TYPE_LINE="${IMAGE_TYPE}=\"${VM_PATH}/${IMAGE_FILE}\""
    case "${IMAGE_FILE,,}" in
        *.qcow2) DISK_FORMAT="qcow2" DISK_LINE="disk_img=\"${VM_PATH}/${IMAGE_FILE}\"" TYPE_LINE="";;
        *.vmdk)  DISK_FORMAT="vmdk"  DISK_LINE="disk_img=\"${VM_PATH}/${IMAGE_FILE}\"" TYPE_LINE="";;
        *.raw)   DISK_FORMAT="raw"   DISK_LINE="disk_img=\"${VM_PATH}/${IMAGE_FILE}\"" TYPE_LINE="";;
        *.img)   DISK_FORMAT="raw"   DISK_LINE="disk_img=\"${VM_PATH}/${IMAGE_FILE}\"" TYPE_LINE="";;
    esac

    CONF_FILE="${VM_PATH}.conf"

    if [ ! -e "${CONF_FILE}" ]; then
        echo "Making ${CONF_FILE}"
        local ARCH_LINE=""
        if [ "${ARCH}" != "${NORMALISED_HOST_ARCH}" ]; then
            if [ "${ARCH}" == "arm64" ]; then
                ARCH_LINE="arch=\"aarch64\""
            else
                ARCH_LINE="arch=\"x86_64\""
            fi
        fi
        cat << EOF > "${CONF_FILE}"
#!${QUICKEMU} --vm
guest_os="${GUEST}"
${ARCH_LINE:+${ARCH_LINE}
}${DISK_LINE}
${DISK_FORMAT:+disk_format=\"${DISK_FORMAT}\"}
${TYPE_LINE}
EOF
        sed -i '/^$/d;/^=$/d' "${CONF_FILE}"
        # Do not write disk_format for ordinary installer VMs (only disk images).
        if [ -n "${TYPE_LINE}" ]; then
            sed -i '/^disk_format=/d' "${CONF_FILE}"
        fi
        echo " - Setting ${CONF_FILE} executable"
        chmod u+x "${CONF_FILE}"
        if [ -n "${ISO_FILE}" ]; then
            echo "fixed_iso=\"${VM_PATH}/${ISO_FILE}\"" >> "${CONF_FILE}"
        fi

        # OS specific tweaks
        case ${OS} in
            alma|centos-stream|endless|garuda|gentoo|kali|nixos|omarchy|oraclelinux|popos|rockylinux)
                echo "disk_size=\"32G\"" >> "${CONF_FILE}";;
            archlinux)
                echo "secureboot=\"on\"" >> "${CONF_FILE}"
                echo "tpm=\"on\"" >> "${CONF_FILE}"
                echo "disk_size=\"32G\"" >> "${CONF_FILE}";;
            openindiana)
                echo "boot=\"legacy\"" >> "${CONF_FILE}"
                echo "disk_size=\"32G\"" >> "${CONF_FILE}";;
            batocera)
                echo "disk_size=\"8G\"" >> "${CONF_FILE}";;
            bazzite)
                echo "disk_size=\"64G\"" >> "${CONF_FILE}";;
            qubes)
                echo "disk_size=\"128G\"" >> "${CONF_FILE}";;
            dragonflybsd|haiku|openbsd|netbsd|slackware|slax|tinycore)
                echo "boot=\"legacy\"" >> "${CONF_FILE}";;
            debian)
                if [[ "${RELEASE}" == *"12"* ]]; then
                    echo "tpm=\"on\"" >> "${CONF_FILE}"
                fi
                if [[ "${RELEASE}" == *"13"* ]]; then
                    echo "tpm=\"on\"" >> "${CONF_FILE}"
                fi
                echo "secureboot=\"on\"" >> "${CONF_FILE}"
                ;;
            deepin)
                echo "disk_size=\"64G\"" >> "${CONF_FILE}"
                echo "ram=\"4G\"" >> "${CONF_FILE}"
                ;;
            freedos)
                echo "boot=\"legacy\"" >> "${CONF_FILE}"
                echo "disk_size=\"4G\"" >> "${CONF_FILE}"
                echo "ram=\"256M\"" >> "${CONF_FILE}"
                ;;
            kolibrios)
                echo "boot=\"legacy\"" >> "${CONF_FILE}"
                echo "disk_size=\"2G\"" >> "${CONF_FILE}"
                echo "ram=\"128M\"" >> "${CONF_FILE}"
                ;;
            templeos)
                echo "boot=\"legacy\"" >> "${CONF_FILE}"
                echo "disk_size=\"2G\"" >> "${CONF_FILE}"
                echo "ram=\"512M\"" >> "${CONF_FILE}"
                ;;
            slint)
                echo "disk_size=\"50G\"" >> "${CONF_FILE}"
                ;;
            slitaz)
                echo "boot=\"legacy\"" >> "${CONF_FILE}"
                echo "disk_size=\"4G\"" >> "${CONF_FILE}"
                echo "ram=\"512M\"" >> "${CONF_FILE}"
                ;;
            ubuntu-server)
                # 22.04+ fails on LVM build if disk size is < 10G
                # 22.04.1 fails on auto-install if TPM is disabled
                echo "disk_size=\"10G\"" >> "${CONF_FILE}"
                echo "ram=\"4G\"" >> "${CONF_FILE}"
                if [[ "${RELEASE}" == *"22.04"* ]]; then
                    echo "tpm=\"on\"" >> "${CONF_FILE}"
                fi
                ;;
            vanillaos)
                ## Minimum is 50G for abroot, but a 64GB is allocated to give some headroom
                echo "disk_size=\"64G\"" >> "${CONF_FILE}"
                ;;
            zorin)
                case ${EDITION} in
                    education64|edulite64) echo "disk_size=\"32G\"" >> "${CONF_FILE}";;
                esac;;
            reactos)
                echo "boot=\"legacy\"" >> "${CONF_FILE}"
                echo "disk_size=\"12G\"" >> "${CONF_FILE}"
                echo "ram=\"2048M\"" >> "${CONF_FILE}"
                ;;
            macos)
                echo "disk_size=\"128G\"" >> "${CONF_FILE}"
                echo "macos_release=\"${RELEASE}\"" >> "${CONF_FILE}"
                # https://github.com/quickemu-project/quickemu/issues/438
                if [ "${RELEASE}" == "monterey" ]; then
                    echo "cpu_cores=2" >> "${CONF_FILE}"
                fi
                ;;
            elementary)
                case "${RELEASE}" in
                    8.1)
                        echo "display=\"spice\"" >> "${CONF_FILE}"
                        ;;
                esac
                ;;
            proxmox-ve)
                echo "disk_size=\"20G\"" >> "${CONF_FILE}"
                echo "ram=\"4G\"" >> "${CONF_FILE}"
                ;;
        esac

        if [ "${OS}" == "ubuntu" ] && [[ ${RELEASE} == *"daily"*  ]]; then
            # Minimum to install lobster testing is 18GB but 32GB are allocated for headroom
            echo "disk_size=\"32G\"" >> "${CONF_FILE}"
        fi

        if [[ "${OS}" == "windows"* ]]; then
            echo "disk_size=\"64G\"" >> "${CONF_FILE}"
        fi

        # Enable TPM for Windows 11 and Windows Server 2022
        if [[ "${OS}" == "windows" && "${RELEASE}" == "11" || "${OS}" == "windows-server" && "${RELEASE}" == "2022" ]]; then
            echo "tpm=\"on\"" >> "${CONF_FILE}"
            echo "secureboot=\"off\"" >> "${CONF_FILE}"
        fi
    fi
    if [ -e "${CONF_FILE}" ] && [ "${CLOUD_INIT:-false}" == "true" ]; then
        gen_seed_iso "${VM_PATH}"
    fi
    if [ -e "${CONF_FILE}" ] && [ "${UNATTENDED:-false}" == "true" ]; then
        case "${OS}" in
            ubuntu-server)
                gen_ubuntu_autoinstall "${VM_PATH}" "${VM_PATH}/${IMAGE_FILE}";;
            *)
                echo "WARNING! --unattended is only supported for ubuntu-server; ignoring.";;
        esac
    fi
    echo -e "\nTo start your $(pretty_name "${OS}") virtual machine run:"
    if [ "${OS}" == "slint" ]; then
        echo -e "    quickemu --vm ${CONF_FILE}\nTo start Slint with braille support run:\n    quickemu --vm --braille --display sdl ${CONF_FILE}"
    else
        echo "    quickemu --vm ${CONF_FILE}"
    fi

    echo
    exit 0
}


function create_vm() {
    # shellcheck disable=SC2206
    local URL_HASH=(${1// / })
    local URL="${URL_HASH[0]}"
    local HASH="${URL_HASH[1]}"
    local ISO="${URL##*/}"
    #echo "${URL}"
    #echo "${ISO}"
    #echo "${HASH}"
    web_get "${URL}" "${VM_PATH}"
    if [ -n "${HASH}" ]; then
        check_hash "${ISO}" "${HASH}"
    fi

    image_normalize
    make_vm_config "${ISO}"
}


function create_config() {
    local VM_PATH="${1}"
    local INPUT="${2}"
    local FIXED_ISO=""

    OS="custom"
    if ! mkdir "${VM_PATH}" 2>/dev/null; then
        echo "ERROR! Could not create directory: ${VM_PATH}. Please verify that it does not already exist"
        exit 1
    fi
    if [[ "${INPUT}" == "http://"* ]] || [[ "${INPUT}" == "https://"* ]]; then
        INPUT="$(web_redirect "${INPUT}")"
        if [[ "${INPUT}" == *".iso" ]] || [[ "${INPUT}" == *".img" ]]; then
            web_get "${INPUT}" "${VM_PATH}"
            INPUT="${INPUT##*/}"
        else
            echo "ERROR! Only ISO,IMG and QCOW2 file types are supported for --create-config"
            exit 1
        fi
    fi

    if [ ! -f "${INPUT}" ]; then
        echo "ERROR! The input must be a valid URL or path to an ISO, IMG, or QCOW2 file."
        exit 1
    elif [[ "${INPUT}" == *".iso" ]]; then
        echo "Moving image to VM dir" && mv "${INPUT}" "${VM_PATH}"
        CUSTOM_IMAGE_TYPE="iso"
    elif [[ "${INPUT}" == *".img" ]]; then
        echo "Moving image to VM dir" && mv "${INPUT}" "${VM_PATH}"
        CUSTOM_IMAGE_TYPE="img"
    elif [[ "${INPUT}" == *".qcow2" ]]; then
        echo "Moving image to VM dir" && mv "${INPUT}" "${VM_PATH}"
        CUSTOM_IMAGE_TYPE="qcow2"
    elif [[ "${INPUT}" == *".vmdk" ]] || [[ "${INPUT}" == *".raw" ]] || [[ "${INPUT}" == *".vdi" ]]; then
        echo "Moving image to VM dir" && mv "${INPUT}" "${VM_PATH}"
        CUSTOM_IMAGE_TYPE="disk"
    else
        echo "ERROR! Only ISO,IMG and QCOW2 file types are supported for --create-config"
        exit 1
    fi
    INPUT="$(basename "${INPUT}")"

    echo "Creating custom VM config for ${INPUT##*/}."
    case "${INPUT,,}" in
        *freebsd*) CUSTOM_OS="freebsd";;
        *kolibrios*) CUSTOM_OS="kolibrios";;
        *reactos*) CUSTOM_OS="reactos";;
        *windows-server*|*eval_oemret_x*|*eval_x*) CUSTOM_OS="windows-server";;
        *windows*|win*)
            CUSTOM_OS="windows"
            # Older windows 10 ISOs use the year followed by the month rather than the year & half). Match any text for language.
            if [ "${3}" != "--disable-unattended" ] && ( [ "${3}" == "--unattended" ] || grep -E -q 'Win(10|11)_([0-9]{2}H(1|2)|[0-9]{4})_[^.]*?(x64|x32)(v[0-9])?.iso' <<< "${INPUT}" ); then
                echo "Creating unattended Windows installation files. To disable, pass --disable-unattended"
                echo
                echo "Downloading VirtIO drivers..."
                web_get "https://fedorapeople.org/groups/virt/virtio-win/direct-downloads/stable-virtio/virtio-win.iso" "${VM_PATH}"
                FIXED_ISO="virtio-win.iso"
                rm -f "${VM_PATH}/unattended.iso"
                local WIN_DRV="w11"
                case "${INPUT,,}" in
                    *win*10*) WIN_DRV="w10";;
                esac
                unattended_windows "${VM_PATH}" "${WIN_DRV}"
            fi
            ;;

        *) CUSTOM_OS="linux";;
    esac
    echo -e "Selecting OS: ${CUSTOM_OS}. If this is incorrect, please modify the config file to include the correct OS.\n"
    make_vm_config "${INPUT}" "${FIXED_ISO}"
}
