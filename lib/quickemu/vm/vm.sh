# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Sourced partial: shares the conf-schema global namespace with bin/quickemu.
# VM domain object: identity, lifecycle operations, boot orchestration,
# firmware and snapshots.

# ---- identity.sh ----
# VM identity: what a VM is — name, config dir, disk dir, artefact paths.
vm_identify() { # conf_path -> sets VMNAME VMDIR VMPATH SOCKET_MONITOR SOCKET_SERIAL
    VMNAME=$(basename "${1}" .conf)         # name of the VM
    VMPATH=$(realpath "$(dirname "${1}")")  # path to the top-level VM directory
    if [ -n "${disk_img}" ]; then
        VMDIR=$(dirname "${disk_img}")      # directory the VM disk and state files are stored
    else
        # No disk_img configured: follow the quickget convention of a per-VM
        # directory named after the conf, alongside it. (Previously this
        # resolved to '.', silently mixing VM state into the current dir.)
        VMDIR="${VMPATH}/${VMNAME}"
    fi
    SOCKET_MONITOR="${VMDIR}/${VMNAME}-monitor.socket"
    SOCKET_SERIAL="${VMDIR}/${VMNAME}-serial.socket"
}

# ---- lifecycle.sh ----
# VM lifecycle operations (stop, pause, clone, rename, ...).

function delete_shortcut() {
    local SHORTCUT_DIR="${HOME}/.local/share/applications"
    if [ -e "${SHORTCUT_DIR}/${VMNAME}.desktop" ]; then
        rm "${SHORTCUT_DIR}/${VMNAME}.desktop"
        echo " - Deleted ${SHORTCUT_DIR}/${VMNAME}.desktop"
    fi
}

function delete_vm() {
    if vm_running "${VMDIR}" "${VMNAME}"; then
        qe_die "vm.delete" VM_MUST_BE_STOPPED "Stop ${VMNAME} before deleting it." 5
    fi
    # Confinement: only ever delete the canonical <confdir>/<VMNAME> tree.
    # VMDIR derives from disk_img inside the sourced conf, so a crafted or
    # typo'd conf must not be able to redirect rm -rf to HOME, / or elsewhere.
    if [ -d "${VMDIR}" ]; then
        local EXPECTED RESOLVED
        EXPECTED="$(cd "$(dirname "${VM}")" && pwd -P)/${VMNAME}"
        RESOLVED="$(cd "${VMDIR}" && pwd -P)"
        if [ "${RESOLVED}" != "${EXPECTED}" ]; then
            qe_die "vm.delete" PERMISSION_DENIED "quickemu refuses to delete '${RESOLVED}': not the VM directory ${EXPECTED}." 12
        fi
    fi
    # Transaction guard: when --plan-id was given, the plan must still
    # match the filesystem — anything added/changed since --delete-plan
    # invalidates it (DELETE_PLAN_STALE).
    if [ -n "${DELETE_PLAN_ID:-}" ]; then
        local ITEMS=() ACTUAL
        delete_plan_items
        ACTUAL=$(delete_plan_id)
        if [ "${ACTUAL}" != "${DELETE_PLAN_ID}" ]; then
            qe_die "vm.delete" DELETE_PLAN_STALE "delete plan is stale (expected ${DELETE_PLAN_ID}, got ${ACTUAL}); regenerate with --delete-plan" 5
        fi
    fi
    echo "Deleting ${VMNAME} completely"
    if [ -d "${VMDIR}" ]; then
        rm -rf -- "${VMDIR}"
        rm "${VM}"
        echo " - Deleted ${VM} and ${VMDIR}/"
        delete_shortcut
    else
        echo " - ${VMDIR} not found. Doing nothing."
    fi
}

function pause_vm() {
    if ! vm_running "${VMDIR}" "${VMNAME}"; then
        echo " - ${VMNAME} is not running."
        return 1
    fi
    # Prefer the QEMU monitor 'stop' command: it suspends the vCPUs while
    # keeping the monitor socket responsive (screenshots, --monitor-cmd).
    if [ "${monitor}" != "none" ] && vm_monitor_socket_ready; then
        monitor_send_cmd "stop" || return 1
    else
        kill -STOP "${VM_PID}" || return 1
    fi
    # The marker lets status_vm/list_vms report 'paused' for monitor-suspended
    # VMs, whose process state stays S/R rather than T. vm_runtime_cleanup
    # removes it when the VM exits or is killed.
    : >"${VMDIR}/${VMNAME}.paused"
    echo " - ${VMNAME} (${VM_PID}) paused."
}

function shutdown_vm() {
    # Graceful ACPI powerdown via the QEMU monitor (system_powerdown).
    if ! vm_running "${VMDIR}" "${VMNAME}"; then
        echo " - ${VMNAME} is not running."
        return 1
    fi
    if [ "${monitor}" != "none" ] && vm_monitor_socket_ready; then
        monitor_send_cmd "system_powerdown" \
            && echo " - ${VMNAME} (${VM_PID}) shutdown requested."
    else
        echo " - ${VMNAME}: no monitor channel; use --kill to force stop."
        return 1
    fi
}

function reset_vm() {
    # Immediate reboot via the QEMU monitor (system_reset); equivalent to
    # pressing the machine's reset button — unsaved guest state is lost.
    if ! vm_running "${VMDIR}" "${VMNAME}"; then
        echo " - ${VMNAME} is not running."
        return 1
    fi
    if [ "${monitor}" != "none" ] && vm_monitor_socket_ready; then
        monitor_send_cmd "system_reset" \
            && echo " - ${VMNAME} (${VM_PID}) reset requested."
    else
        echo " - ${VMNAME}: no monitor channel; use --kill to force stop."
        return 1
    fi
}

function resume_vm() {
    if ! vm_running "${VMDIR}" "${VMNAME}"; then
        echo " - ${VMNAME} is not running."
        return 1
    fi
    if [ "${monitor}" != "none" ] && vm_monitor_socket_ready; then
        monitor_send_cmd "cont" || return 1
    else
        kill -CONT "${VM_PID}" || return 1
    fi
    rm -f "${VMDIR}/${VMNAME}.paused"
    echo " - ${VMNAME} (${VM_PID}) resumed."
}

function screenshot_vm() {
    local OUT="${SCREENSHOT_OUT:-${VMDIR}/${VMNAME}-$(date +%Y%m%d-%H%M%S).ppm}"
    local PPM="${OUT}"
    if ! vm_running "${VMDIR}" "${VMNAME}"; then
        echo " - ${VMNAME} is not running."
        return 1
    fi
    if [ "${monitor}" == "none" ]; then
        echo " - ERROR! No QEMU monitor channel for ${VMNAME} (monitor is disabled)."
        return 1
    fi
    if [ "${monitor}" == "socket" ] && ! vm_monitor_socket_ready; then
        echo " - ERROR! No QEMU monitor socket for ${VMNAME}."
        return 1
    fi
    if ! command -v socat &>/dev/null; then
        echo " - ERROR! socat is required for --screenshot."
        return 1
    fi
    # screendump always writes PPM; convert when a .png filename was requested.
    if [[ "${OUT}" == *.png ]]; then
        PPM="${OUT%.png}.ppm"
    fi
    monitor_send_cmd "screendump ${PPM}"
    sleep 0.5
    if [ ! -s "${PPM}" ]; then
        echo " - ERROR! Screenshot was not created at ${PPM}."
        return 1
    fi
    if [[ "${OUT}" == *.png ]]; then
        if command -v convert &>/dev/null; then
            convert "${PPM}" "${OUT}" && rm -f "${PPM}"
        elif command -v pnmtopng &>/dev/null; then
            pnmtopng "${PPM}" >"${OUT}" && rm -f "${PPM}"
        else
            OUT="${PPM}"
            echo " - WARNING! 'convert'/'pnmtopng' not found; saved PPM instead."
        fi
    fi
    echo " - Screenshot saved to ${OUT}"
}

function clone_vm() {
    local NEW_NAME="${CLONE_NAME}"
    local PARENT="" NEW_DIR="" ITEM=""
    if [ -z "${NEW_NAME}" ]; then
        echo "ERROR! '--clone' needs a name for the new VM."
        exit 1
    fi
    PARENT="$(dirname "${VMDIR}")"
    NEW_DIR="${PARENT}/${NEW_NAME}"
    if [ -e "${NEW_DIR}" ] || [ -e "${PARENT}/${NEW_NAME}.conf" ]; then
        echo "ERROR! ${NEW_NAME} already exists."
        exit 1
    fi
    if vm_running "${VMDIR}" "${VMNAME}"; then
        echo "ERROR! Stop ${VMNAME} before cloning."
        exit 1
    fi
    mkdir -p "${NEW_DIR}"
    # Copy the config, rewriting references to the old VM directory.
    sed "s|${VMNAME}/|${NEW_NAME}/|g" "${VM}" >"${PARENT}/${NEW_NAME}.conf"
    # Copy firmware vars, TPM state and auxiliary files; skip the disk and
    # runtime artefacts (pidfiles, sockets, logs, port maps).
    for ITEM in "${VMDIR}"/*; do
        case "$(basename "${ITEM}")" in
        "$(basename "${disk_img}")" | *.pid | *.sock | *.socket | *.ports | *.log | *.sh | *.spice | *.monitor) continue ;;
        esac
        if [ -d "${ITEM}" ]; then
            cp -a "${ITEM}" "${NEW_DIR}/"
        elif [ -f "${ITEM}" ]; then
            cp "${ITEM}" "${NEW_DIR}/"
        fi
    done
    if [ -f "${disk_img}" ]; then
        require_qemu_img
        if [ -n "${CLONE_FULL}" ]; then
            ${QEMU_IMG} convert -O "${disk_format}" "${disk_img}" "${NEW_DIR}/$(basename "${disk_img}")"
            echo " - Cloned ${VMNAME} to ${NEW_NAME} (full disk copy)."
        else
            ${QEMU_IMG} create -f qcow2 -b "${disk_img}" -F "${disk_format}" "${NEW_DIR}/$(basename "${disk_img}")" >/dev/null
            echo " - Cloned ${VMNAME} to ${NEW_NAME} (shared backing disk)."
        fi
    else
        echo " - Cloned ${VMNAME} to ${NEW_NAME} (config only; no disk image found)."
    fi
}

function export_vm() {
    local OUT="${EXPORT_FILE:-${VMNAME}.quickvm.tar.gz}" PARENT=""
    local MEMBERS=("${VMNAME}.conf")
    if vm_running "${VMDIR}" "${VMNAME}"; then
        echo "ERROR! Stop ${VMNAME} before exporting."
        exit 1
    fi
    PARENT="$(dirname "${VMDIR}")"
    if [ -d "${PARENT}/${VMNAME}" ]; then
        MEMBERS+=("${VMNAME}")
    fi
    tar -C "${PARENT}" \
        --exclude="${VMNAME}/*.pid" --exclude="${VMNAME}/*.sock" \
        --exclude="${VMNAME}/*.socket" --exclude="${VMNAME}/*.ports" \
        --exclude="${VMNAME}/*.spice" --exclude="${VMNAME}/*.monitor" \
        -czf "${OUT}" "${MEMBERS[@]}"
    echo " - Exported ${VMNAME} to ${OUT}"
}

function import_vm() {
    local IN="${IMPORT_FILE}" BASE="" TMPDIR_IM="" MEMBER="" ENTRY=""
    if [ -z "${IN}" ] || [ ! -f "${IN}" ]; then
        echo "ERROR! '--import' needs an existing .tar.gz file."
        exit 1
    fi
    # Member whitelist: refuse absolute paths, traversal and links/devices
    # BEFORE writing anything (tar slip / symlink overwrite attacks).
    while IFS= read -r MEMBER; do
        case "${MEMBER}" in
        /* | '..' | ../* | */.. | */../*)
            qe_die "vm.import" UNSAFE_ARCHIVE "Unsafe path in archive: '${MEMBER}'" 12
            ;;
        esac
    done < <(tar -tzf "${IN}" 2>/dev/null)
    if tar -tzvf "${IN}" 2>/dev/null | awk '$1 ~ /^[lhbcp]/ { exit 42 }'; [ $? -eq 42 ]; then
        qe_die "vm.import" UNSAFE_ARCHIVE "Archive contains links or special files; refusing import." 12
    fi
    BASE=$(tar -tzf "${IN}" 2>/dev/null | grep -m1 '\.conf$' | xargs -r basename | sed 's/\.conf$//')
    if [ -z "${BASE}" ]; then
        echo "ERROR! No .conf found inside ${IN}."
        exit 1
    fi
    case "${BASE}" in
    "" | . | .. | */* | -*)
        echo "ERROR! Unsafe VM name '${BASE}' in archive."
        exit 1
        ;;
    esac
    if [ -e "${BASE}.conf" ] || [ -d "${BASE}" ]; then
        echo "ERROR! ${BASE} already exists."
        exit 1
    fi
    # Extract to a private staging dir first; move into place only on success.
    TMPDIR_IM=$(mktemp -d "${TMPDIR:-/tmp}/quickemu-import.XXXXXX") || exit 1
    if ! tar -xzf "${IN}" -C "${TMPDIR_IM}" 2>/dev/null; then
        rm -rf -- "${TMPDIR_IM}"
        echo "ERROR! Failed to extract ${IN}."
        exit 1
    fi
    for ENTRY in "${TMPDIR_IM}"/* "${TMPDIR_IM}"/.[!.]*; do
        [ -e "${ENTRY}" ] || continue
        if ! mv -- "${ENTRY}" .; then
            rm -rf -- "${TMPDIR_IM}"
            echo "ERROR! Failed to install ${ENTRY}."
            exit 1
        fi
    done
    rm -rf -- "${TMPDIR_IM}"
    echo " - Imported ${BASE} from ${IN}"
    echo " - WARNING! ${BASE}.conf is executed as code on launch; review it before running the VM."
}

function rename_vm() {
    local NEW_NAME="${RENAME_TO}" PARENT=""
    if [ -z "${NEW_NAME}" ]; then
        echo "ERROR! '--rename' needs a new name."
        exit 1
    fi
    PARENT="$(dirname "${VMDIR}")"
    if [ -e "${PARENT}/${NEW_NAME}" ] || [ -e "${PARENT}/${NEW_NAME}.conf" ]; then
        echo "ERROR! ${NEW_NAME} already exists."
        exit 1
    fi
    if vm_running "${VMDIR}" "${VMNAME}"; then
        echo "ERROR! Stop ${VMNAME} before renaming."
        exit 1
    fi
    [ -d "${VMDIR}" ] && mv "${VMDIR}" "${PARENT}/${NEW_NAME}"
    mv "${VM}" "${PARENT}/${NEW_NAME}.conf"
    # Escape sed metacharacters in the names before rewriting the conf.
    local OLD_ESC NEW_ESC
    OLD_ESC=$(printf '%s' "${VMNAME}/" | sed 's/[&|\\]/\\&/g')
    NEW_ESC=$(printf '%s' "${NEW_NAME}/" | sed 's/[&|\\]/\\&/g')
    sed -i "s|${OLD_ESC}|${NEW_ESC}|g" "${PARENT}/${NEW_NAME}.conf"
    echo " - Renamed ${VMNAME} to ${NEW_NAME}"
}

function send_keys() {
    monitor_send_cmd "sendkey ${SEND_KEYS_ARG}"
}

function type_text() {
    local TEXT="${TYPE_TEXT}" C="" KEY="" i=0
    for ((i = 0; i < ${#TEXT}; i++)); do
        C="${TEXT:i:1}"
        # shellcheck disable=SC1003 # '\' is a literal backslash pattern
        case "${C}" in
        [a-z0-9]) KEY="${C}" ;;
        [A-Z]) KEY="shift-${C,,}" ;;
        ' ') KEY="spc" ;;
        '.') KEY="dot" ;; ',') KEY="comma" ;;
        '-') KEY="minus" ;; '=') KEY="equal" ;;
        '/') KEY="slash" ;; '\') KEY="backslash" ;;
        ';') KEY="semicolon" ;; "'") KEY="apostrophe" ;;
        '[') KEY="bracket_left" ;; ']') KEY="bracket_right" ;;
        '`') KEY="grave_accent" ;;
        ':') KEY="shift-semicolon" ;;
        '!') KEY="shift-1" ;; '@') KEY="shift-2" ;;
        '#') KEY="shift-3" ;; '$') KEY="shift-4" ;;
        '%') KEY="shift-5" ;; '^') KEY="shift-6" ;;
        '&') KEY="shift-7" ;; '*') KEY="shift-8" ;;
        '(') KEY="shift-9" ;; ')') KEY="shift-0" ;;
        '_') KEY="shift-minus" ;; '+') KEY="shift-equal" ;;
        '<') KEY="shift-comma" ;; '>') KEY="shift-dot" ;;
        '?') KEY="shift-slash" ;; '|') KEY="shift-backslash" ;;
        '"') KEY="shift-apostrophe" ;;
        '{') KEY="shift-bracket_left" ;;
        '}') KEY="shift-bracket_right" ;;
        '~') KEY="shift-grave_accent" ;;
        *)
            echo " - Skipping unmapped char '${C}'"
            continue
            ;;
        esac
        monitor_send_cmd "sendkey ${KEY}"
        sleep 0.05
    done
}

function autostart_create() {
    local UNIT_DIR="${XDG_CONFIG_HOME:-${HOME}/.config}/systemd/user"
    local UNIT="${UNIT_DIR}/quickemu-${VMNAME}.service"
    local SELF="" CONF_PATH=""
    SELF=$(readlink -f "$0" 2>/dev/null || realpath "$0")
    CONF_PATH=$(readlink -f "${VM}" 2>/dev/null || realpath "${VM}")
    if [ ! -d "${UNIT_DIR}" ]; then
        mkdir -p "${UNIT_DIR}"
    fi
    cat <<EOFD >"${UNIT}"
[Unit]
Description=Quickemu VM ${VMNAME}
After=default.target

[Service]
ExecStart=${SELF} --vm ${CONF_PATH}
Restart=on-failure

[Install]
WantedBy=default.target
EOFD
    if command -v systemctl >/dev/null 2>&1; then
        systemctl --user daemon-reload
        systemctl --user enable "quickemu-${VMNAME}.service"
    fi
    echo " - Autostart unit written: ${UNIT}"
    echo "             Start now with: systemctl --user start quickemu-${VMNAME}.service"
}

function kill_vm() {
    echo "Killing ${VMNAME}"
    if [ -z "${VM_PID}" ]; then
        echo " - ${VMNAME} is not running."
        vm_runtime_cleanup
    elif kill -9 "${VM_PID}" >/dev/null 2>&1; then
        echo " - ${VMNAME} (${VM_PID}) killed."
        vm_runtime_cleanup
    else
        echo " - ${VMNAME} (${VM_PID}) was not killed."
    fi
}

function shortcut_create {
    local dirname="${HOME}/.local/share/applications"
    local filename="${HOME}/.local/share/applications/${VMNAME}.desktop"
    echo "Creating ${VMNAME} desktop shortcut file"

    if [ ! -d "${dirname}" ]; then
        mkdir -p "${dirname}"
    fi
    cat <<EOF >"${filename}"
[Desktop Entry]
Version=1.0
Type=Application
Terminal=false
Exec=$(basename "${0}") --vm ${VM} ${SHORTCUT_OPTIONS}
Path=${VMPATH}
Name=${VMNAME}
Icon=qemu
EOF
    echo " - ${filename} created."
}

# ---- boot.sh ----
# VM launch orchestration: config -> argv -> spawn.

vm_boot_media() {

    if [ -n "${iso}" ] && [ "${guest_os}" == "freedos" ]; then
        # FreeDOS reboots after partitioning the disk, and QEMU tries to boot from disk after first restart
        # This flag sets the boot order to cdrom,disk. It will persist until powering down the VM
        args+=(-boot order=dc)
    elif [ -n "${iso}" ] && [ "${guest_os}" == "kolibrios" ]; then
        # Since there is bug (probably) in KolibriOS: cdrom indexes 0 or 1 make system show an extra unexisting iso, so we use index=2
        # shellcheck disable=SC2054
        args+=(-drive media=cdrom,index=2,file="${iso}")
        iso=""
    elif [ -n "${iso}" ] && [ "${guest_os}" == "reactos" ]; then
        # https://reactos.org/wiki/QEMU
        # shellcheck disable=SC2054
        args+=(-boot order=d
            -drive if=ide,index=2,media=cdrom,file="${iso}")
        iso=""
    elif [ -n "${iso}" ] && [[ "${guest_os}" == "windows"* ]] && [ -e "${VMDIR}/unattended.iso" ]; then
        # Attach the unattended configuration to Windows guests when booting from ISO
        # shellcheck disable=SC2054
        args+=(-drive media=cdrom,index=2,file="${VMDIR}/unattended.iso")
    fi

    if [ -n "${floppy}" ]; then
        # shellcheck disable=SC2054
        args+=(-drive if=floppy,format=raw,file="${floppy}")
    fi

    # ARM64: create virtio-scsi controller if any CD-ROM ISOs are present
    # (virt machine has no IDE controller)
    if [ "${ARCH_VM}" == "aarch64" ] && { [ -n "${iso}" ] || [ -n "${fixed_iso}" ]; }; then
        # shellcheck disable=SC2054
        args+=(-device virtio-scsi-pci,id=scsi0)
    fi

    if [ -n "${iso}" ]; then
        if [ "${ARCH_VM}" == "aarch64" ]; then
            # ARM64: bootindex=1 ensures UEFI boots from CD-ROM first during installation
            # shellcheck disable=SC2054
            args+=(-device scsi-cd,drive=cd0,bus=scsi0.0,bootindex=1
                -drive id=cd0,if=none,format=raw,media=cdrom,readonly=on,file="${iso}")
        else
            # shellcheck disable=SC2054
            args+=(-drive media=cdrom,index=0,file="${iso}")
        fi
    fi

    if [ -n "${fixed_iso}" ]; then
        if [ "${ARCH_VM}" == "aarch64" ]; then
            # ARM64: attach second ISO to virtio-scsi controller
            # shellcheck disable=SC2054
            args+=(-device scsi-cd,drive=cd1,bus=scsi0.0,bootindex=3
                -drive id=cd1,if=none,format=raw,media=cdrom,readonly=on,file="${fixed_iso}")
        else
            # shellcheck disable=SC2054
            args+=(-drive media=cdrom,index=1,file="${fixed_iso}")
        fi
    fi
}

vm_boot_fileshare() {
    # File sharing: prefer virtiofs (shared memory, lower latency) over 9p when
    # virtiofsd is available; virtiofsd must already be running at this point.
    # https://wiki.qemu.org/Documentation/9psetup
    # https://askubuntu.com/questions/772784/9p-libvirt-qemu-share-modes
    if [ "${guest_os}" != "windows" ] || [ "${guest_os}" == "windows-server" ] && [ -n "${PUBLIC}" ]; then
        if [ -n "${VIRTIOFSD_SOCKET}" ]; then
            # Verify QEMU supports vhost-user-fs-pci before using it; older QEMU
            # builds silently lack the device and would abort VM startup.
            if ! "${QEMU}" -device help 2>&1 | grep -q '"vhost-user-fs-pci"'; then
                echo " - WARNING! QEMU does not support vhost-user-fs-pci; falling back to 9p."
                stop_virtiofsd
                VIRTIOFSD_SOCKET=""
            fi
        fi
        if [ -n "${VIRTIOFSD_SOCKET}" ]; then
            # virtiofs requires a shared-memory backend; the size mirrors the VM RAM.
            # shellcheck disable=SC2054
            args+=(-object "memory-backend-file,id=mem,size=${RAM_VM},mem-path=/dev/shm,share=on"
                -numa node,memdev=mem
                -chardev "socket,id=char0,path=${VIRTIOFSD_SOCKET}"
                -device "vhost-user-fs-pci,queue-size=1024,chardev=char0,tag=${PUBLIC_TAG}")
        else
            # shellcheck disable=SC2054
            args+=(-fsdev local,id=fsdev0,path="${PUBLIC}",security_model=mapped-xattr
                -device virtio-9p-pci,fsdev=fsdev0,mount_tag="${PUBLIC_TAG}")
        fi
    fi
}

vm_boot_passthru() {
    if [ -n "${USB_PASSTHROUGH}" ]; then
        # shellcheck disable=SC2054,SC2206
        args+=(-device ${USB_HOST_PASSTHROUGH_CONTROLLER},id=hostpass
            ${USB_PASSTHROUGH})
    fi

    if [ -n "${gpu_passthrough}" ]; then
        # GPU passthrough requires IOMMU enabled on the host and the device
        # bound to vfio-pci. Multiple devices can be passed, space separated.
        if [ ! -d /sys/kernel/iommu_groups ] || [ -z "$(ls -A /sys/kernel/iommu_groups 2>/dev/null)" ]; then
            echo " - WARNING! No IOMMU groups found. Enable IOMMU (intel_iommu=on / amd_iommu=on) for GPU passthrough."
        fi
        for GPU_DEV in ${gpu_passthrough}; do
            if [ -L "/sys/bus/pci/devices/${GPU_DEV}/driver" ]; then
                GPU_DRIVER="$(basename "$(readlink -f "/sys/bus/pci/devices/${GPU_DEV}/driver")")"
                if [ "${GPU_DRIVER}" != "vfio-pci" ]; then
                    echo " - WARNING! ${GPU_DEV} is bound to '${GPU_DRIVER}', not vfio-pci."
                fi
            fi
            echo " - GPU:      Passthrough ${GPU_DEV}"
            args+=(-device "vfio-pci,host=${GPU_DEV}")
        done
    fi

    if [ "${tpm}" == "on" ] && swtpm_socket_ready; then
        # shellcheck disable=SC2054
        if [ "${ARCH_VM}" == "aarch64" ]; then
            # ARM64 uses tpm-tis-device (system bus) instead of tpm-tis (ISA/LPC bus)
            args+=(-chardev socket,id=chrtpm,path="${VMDIR}/${VMNAME}.swtpm-sock"
                -tpmdev emulator,id=tpm0,chardev=chrtpm
                -device tpm-tis-device,tpmdev=tpm0)
        else
            args+=(-chardev socket,id=chrtpm,path="${VMDIR}/${VMNAME}.swtpm-sock"
                -tpmdev emulator,id=tpm0,chardev=chrtpm
                -device tpm-tis,tpmdev=tpm0)
        fi
    fi
}

vm_boot_channels() {
    if [ "${monitor}" == "none" ]; then
        args+=(-monitor none)
        echo " - Monitor:  (off)"
    elif [ "${monitor}" == "telnet" ]; then
        # Find a free port to expose monitor-telnet to the guest
        TEMP_PORT="$(get_port "${monitor_telnet_port}" 9)"
        if [ -z "${TEMP_PORT}" ]; then
            echo " - Monitor:  All Monitor-Telnet ports have been exhausted."
        else
            monitor_telnet_port="${TEMP_PORT}"
            # shellcheck disable=SC2054
            args+=(-monitor telnet:"${monitor_telnet_host}:${monitor_telnet_port}",server,nowait)
            echo " - Monitor:  On host:  telnet ${monitor_telnet_host} ${monitor_telnet_port}"
            ports_record "monitor-telnet,${monitor_telnet_port},${monitor_telnet_host}"
        fi
    elif [ "${monitor}" == "socket" ]; then
        # shellcheck disable=SC2054,SC2206
        args+=(-monitor unix:${SOCKET_MONITOR},server,nowait)
        if command -v socat &>/dev/null; then
            echo " - Monitor:  On host:  socat -,echo=0,icanon=0 unix-connect:${SOCKET_MONITOR}"
        elif command -v nc &>/dev/null; then
            echo " - Monitor:  On host:  nc -U \"${SOCKET_MONITOR}\""
        fi
    else
        echo "ERROR! \"${monitor}\" is an unknown monitor option."
        exit 1
    fi

    if [ "${serial}" == "none" ]; then
        args+=(-serial none)
        # No log output when serial is disabled - it's the default for macOS/Windows
        # and provides no useful information to the user
    elif [ "${serial}" == "telnet" ]; then
        # Find a free port to expose serial-telnet to the guest
        TEMP_PORT="$(get_port "${serial_telnet_port}" 9)"
        if [ -z "${TEMP_PORT}" ]; then
            echo " - Serial:   All Serial Telnet ports have been exhausted."
        else
            serial_telnet_port="${TEMP_PORT}"
            # shellcheck disable=SC2054,SC2206
            args+=(-serial telnet:${serial_telnet_host}:${serial_telnet_port},server,nowait)
            echo " - Serial:   On host:  telnet ${serial_telnet_host} ${serial_telnet_port}"
            ports_record "serial-telnet,${serial_telnet_port},${serial_telnet_host}"
        fi
    elif [ "${serial}" == "socket" ]; then
        # shellcheck disable=SC2054,SC2206
        args+=(-serial unix:${SOCKET_SERIAL},server,nowait)
        if command -v socat &>/dev/null; then
            echo " - Serial:   On host:  socat -,echo=0,icanon=0 unix-connect:${SOCKET_SERIAL}"
        elif command -v nc &>/dev/null; then
            echo " - Serial:   On host:  nc -U \"${SOCKET_SERIAL}\""
        fi
    else
        echo "ERROR! \"${serial}\" is an unknown serial option."
        exit 1
    fi
}

function vm_boot() {
    AUDIO_DEV=""
    BALLOON="-device virtio-balloon"
    BOOT_STATUS=""
    CPU=""
    CORE_MAPPING=""
    DISK_USED=""
    DISPLAY_DEVICE=""
    DISPLAY_RENDER=""
    EFI_CODE=""
    EFI_MODE="pflash"
    EFI_VARS=""
    GUEST_CPU_CORES=""
    GUEST_CPU_LOGICAL_CORES=""
    GUEST_CPU_THREADS=""
    HOST_CPU_CORES=""
    HOST_CPU_SMT=""
    HOST_CPU_SOCKETS=""
    HOST_CPU_VENDOR=""
    GUEST_TWEAKS=""
    KERNEL_NAME="Unknown"
    KERNEL_NODE=""
    KERNEL_VER="?"
    OS_RELEASE="Unknown OS"
    MACHINE_TYPE="${MACHINE_TYPE:-q35}"
    MAC_BOOTLOADER=""
    MAC_MISSING=""
    MAC_DISK_DEV="${MAC_DISK_DEV:-ide-hd,bus=ahci.2}"
    NET_DEVICE="${NET_DEVICE:-virtio-net-pci}"
    SOUND=""
    SPICE=""
    SMM="${SMM:-off}"
    local TEMP_PORT=""
    USB_HOST_PASSTHROUGH_CONTROLLER="qemu-xhci"
    VIDEO=""

    KERNEL_NAME="$(uname -s)"
    KERNEL_NODE="$(uname -n | cut -d'.' -f 1)"
    KERNEL_VER="$(uname -r)"

    if [ "${OS_KERNEL}" == "Darwin" ]; then
        # Get macOS product name and version using swvers
        if [ -x "$(command -v sw_vers)" ]; then
            OS_RELEASE="$(sw_vers -productName) $(sw_vers -productVersion)"
        fi
    elif [ -e /etc/os-release ]; then
        OS_RELEASE=$(grep PRETTY_NAME /etc/os-release | cut -d'"' -f2)
    fi

    echo "Quickemu ${VERSION} using ${QEMU} v${QEMU_VER_LONG}"
    echo " - Host:     ${OS_RELEASE} running ${KERNEL_NAME} ${KERNEL_VER} ${KERNEL_NODE}"

    # Force to lowercase.
    boot=${boot,,}
    guest_os=${guest_os,,}
    args=()

    # Warn on unrecognised guest_os values (a typo silently gets generic
    # defaults). 'custom' configs may use arbitrary values, so warn only.
    case ${guest_os} in
    batocera | freedos | haiku | kolibrios | linux | linux_old | macos | reactos | solaris | templeos | windows | windows-server | *bsd) ;;
    *) echo " - WARNING!  '${guest_os}' is not a recognised guest_os value; using generic defaults." ;;
    esac

    # $bridge backwards compatibility for Quickemu <= 4.0. Resolved before
    # configure_ports so SSH forward output reflects the real network mode.
    if [ -n "${bridge}" ]; then
        network="${bridge}"
    fi
    # Set the hostname of the VM
    NET="user,hostname=${VMNAME}"

    runtime_launcher_init

    configure_cpu
    configure_ram
    plan_check_tsc
    configure_bios
    configure_os_quirks
    configure_storage
    start_virtiofsd
    configure_display
    configure_audio
    configure_ports
    configure_file_sharing
    configure_usb
    configure_tpm

    # Changing process name is not supported on macOS
    if [ "${OS_KERNEL}" == "Linux" ]; then
        # shellcheck disable=SC2054,SC2206,SC2140
        args+=(-name ${VMNAME},process=${VMNAME},debug-threads=on)
    fi

    # Build machine arguments - SMM and vmport are x86-only options
    # SMM (System Management Mode) is an x86-specific CPU mode used for firmware operations
    # and is required for Secure Boot. ARM64 uses different mechanisms for firmware security.
    # vmport emulates VMware's I/O port for guest tools, which is also x86-specific.
    #
    # TCG-specific optimisations for cross-architecture emulation:
    # - Use -accel tcg,... to specify tb-size and thread options
    # - QEMU does not allow both -machine accel= and -accel simultaneously
    # - For KVM/HVF, continue using -machine accel= (simpler, no extra options needed)

    # ARM64 pflash firmware exposes the code and variable stores as named
    # blockdev nodes referenced by the virt machine.
    if [ "${ARCH_VM}" == "aarch64" ] &&
        [[ "${boot}" == *"efi"* ]] &&
        [ "${EFI_MODE}" == "pflash" ]; then
        MACHINE_TYPE+=",pflash0=rom,pflash1=efivars"
    fi

    if [ "${QEMU_ACCEL}" == "tcg" ]; then
        local HOST_RAM_GB=$(( ($(host_total_mem_mb) + 512) / 1024 ))
        # Use larger translation cache on hosts with 16GB+ RAM
        local TCG_TB_SIZE=256
        if [ "${HOST_RAM_GB}" -ge 16 ]; then
            TCG_TB_SIZE=512
        fi
        # shellcheck disable=SC2054,SC2206
        args+=(-accel tcg,tb-size=${TCG_TB_SIZE},thread=multi)

        if [ "${ARCH_VM}" == "aarch64" ]; then
            # ARM64 uses 'virt' machine type without x86-specific options
            # shellcheck disable=SC2054,SC2206,SC2140
            args+=(-machine ${MACHINE_TYPE} ${GUEST_TWEAKS}
                ${CPU} ${SMP}
                -m ${RAM_VM} ${BALLOON}
                -pidfile "${VMDIR}/${VMNAME}.pid")
        else
            # x86_64 includes SMM (System Management Mode) and vmport options
            # shellcheck disable=SC2054,SC2206,SC2140
            args+=(-machine ${MACHINE_TYPE},smm=${SMM},vmport=off ${GUEST_TWEAKS}
                ${CPU} ${SMP}
                -m ${RAM_VM} ${BALLOON}
                -pidfile "${VMDIR}/${VMNAME}.pid")
        fi
    else
        # KVM/HVF: use -machine accel= (no extra options needed)
        if [ "${ARCH_VM}" == "aarch64" ]; then
            # ARM64 uses 'virt' machine type without x86-specific options
            # shellcheck disable=SC2054,SC2206,SC2140
            args+=(-machine ${MACHINE_TYPE},accel=${QEMU_ACCEL} ${GUEST_TWEAKS}
                ${CPU} ${SMP}
                -m ${RAM_VM} ${BALLOON}
                -pidfile "${VMDIR}/${VMNAME}.pid")
        else
            # x86_64 includes SMM (System Management Mode) and vmport options
            # shellcheck disable=SC2054,SC2206,SC2140
            args+=(-machine ${MACHINE_TYPE},smm=${SMM},vmport=off,accel=${QEMU_ACCEL} ${GUEST_TWEAKS}
                ${CPU} ${SMP}
                -m ${RAM_VM} ${BALLOON}
                -pidfile "${VMDIR}/${VMNAME}.pid")
        fi
    fi

    if [ "${guest_os}" == "windows" ] || [ "${guest_os}" == "windows-server" ] || [ "${guest_os}" == "reactos" ] || [ "${guest_os}" == "freedos" ]; then
        # shellcheck disable=SC2054
        args+=(-rtc base=localtime,clock=host,driftfix=slew)
    else
        # shellcheck disable=SC2054
        args+=(-rtc base=utc,clock=host)
    fi

    # shellcheck disable=SC2206
    args+=(${VIDEO} -display ${DISPLAY_RENDER})
    # Only enable SPICE is using SPICE display
    if [ "${display}" == "none" ] || [ "${display}" == "spice" ] || [ "${display}" == "spice-app" ]; then
        # shellcheck disable=SC2054
        args+=(-spice "${SPICE}"
            -device virtio-serial-pci
            -chardev socket,id=agent0,path="${VMDIR}/${VMNAME}-agent.sock",server=on,wait=off
            -device virtserialport,chardev=agent0,name=org.qemu.guest_agent.0
            -chardev spicevmc,id=vdagent0,name=vdagent
            -device virtserialport,chardev=vdagent0,name=com.redhat.spice.0
            -chardev spiceport,id=webdav0,name=org.spice-space.webdav.0
            -device virtserialport,chardev=webdav0,name=org.spice-space.webdav.0)
    fi

    # shellcheck disable=SC2054
    args+=(-device virtio-rng-pci,rng=rng0 -object rng-random,id=rng0,filename=/dev/urandom)

    # macOS doesn't support SPICE
    if [ "${OS_KERNEL}" == "Linux" ]; then
        # shellcheck disable=SC2054
        args+=(-device "${USB_HOST_PASSTHROUGH_CONTROLLER}",id=spicepass
            -chardev spicevmc,id=usbredirchardev1,name=usbredir
            -device usb-redir,chardev=usbredirchardev1,id=usbredirdev1
            -chardev spicevmc,id=usbredirchardev2,name=usbredir
            -device usb-redir,chardev=usbredirchardev2,id=usbredirdev2
            -chardev spicevmc,id=usbredirchardev3,name=usbredir
            -device usb-redir,chardev=usbredirchardev3,id=usbredirdev3
            -device pci-ohci,id=smartpass
            -device usb-ccid)

        if ${QEMU} -device help | grep -q "passthrough smartcard"; then
            # shellcheck disable=SC2054
            args+=(-chardev spicevmc,id=ccid,name=smartcard
                -device ccid-card-passthru,chardev=ccid)
        else
            echo " - WARNING!  ${QEMU} or SPICE was not compiled with support for smartcard devices"
        fi
    fi

    # setup usb-controller
    if [ "${usb_controller}" == "ehci" ]; then
        # shellcheck disable=SC2054
        args+=(-device usb-ehci,id=input)
    elif [ "${usb_controller}" == "xhci" ]; then
        # shellcheck disable=SC2054
        args+=(-device qemu-xhci,id=input)
    elif [ "${usb_controller}" == "none" ]; then
        # add nothing
        :
    else
        echo " - WARNING!  Unknown usb-controller value: '${usb_controller}'"
    fi

    # setup keyboard
    # @INFO: must be set after usb-controller
    if [ "${keyboard}" == "usb" ]; then
        # shellcheck disable=SC2054
        args+=(-device usb-kbd,bus=input.0)
    elif [ "${keyboard}" == "virtio" ]; then
        # shellcheck disable=SC2054
        args+=(-device virtio-keyboard)
    elif [ "${keyboard}" == "ps2" ]; then
        # add nothing, default is ps/2 keyboard
        :
    else
        echo " - WARNING!  Unknown keyboard value: '${keyboard}'; Fallback to ps2"
    fi

    # setup keyboard_layout
    # @INFO: When using the VNC display, you must use the -k parameter to set the keyboard layout if you are not using en-us.
    if [ -n "${keyboard_layout}" ]; then
        args+=(-k "${keyboard_layout}")
    fi

    # Braille requires SDL, so disable for macOS
    if [ -n "${BRAILLE}" ] && [ "${OS_KERNEL}" == "Linux" ]; then
        if ${QEMU} -chardev help | grep -q braille; then
            # shellcheck disable=SC2054
            #args+=(-chardev braille,id=brltty
            #       -device usb-braille,id=usbbrl,chardev=brltty)
            args+=(-usbdevice braille)
        else
            echo " - WARNING!  ${QEMU} does not support -chardev braille "
        fi
    fi

    # Validate input of the CPU_PINNING, pinning is not supported on macOS
    if [ -n "${CPU_PINNING}" ] && [ "${OS_KERNEL}" == "Linux" ]; then
        if ! [[ "${CPU_PINNING}" =~ ^[0-9]+(,[0-9]+)*$ ]]; then
            echo " - ERROR!  Couldn't parse CPU pinning: '${CPU_PINNING}', only comma-separated list is supported"
            exit 1
        fi

        IFS=',' read -r -a CORE_MAPPING <<<"$CPU_PINNING"

        NUM_CORE_MAPPING=${#CORE_MAPPING[@]}
        if [ "$NUM_CORE_MAPPING" -ne "$GUEST_CPU_LOGICAL_CORES" ]; then
            echo " - ERROR!  Number of host cores for pinning should be equal to VM core count ($NUM_CORE_MAPPING != $GUEST_CPU_LOGICAL_CORES)"
            exit 1
        fi
    fi

    # setup mouse
    # @INFO: must be set after usb-controller
    if [ "${mouse}" == "usb" ]; then
        # shellcheck disable=SC2054
        args+=(-device usb-mouse,bus=input.0)
    elif [ "${mouse}" == "tablet" ]; then
        # shellcheck disable=SC2054
        args+=(-device usb-tablet,bus=input.0)
    elif [ "${mouse}" == "virtio" ]; then
        # shellcheck disable=SC2054
        args+=(-device virtio-mouse)
    elif [ "${mouse}" == "ps2" ]; then
        # add nothing, default is ps/2 mouse
        :
    else
        echo " - WARNING!  Unknown mouse value: '${mouse}'; Falling back to ps2"
    fi

    # setup audio
    # @INFO: must be set after usb-controller; in case usb-audio is used
    # shellcheck disable=SC2206
    args+=(-audiodev ${AUDIO_DEV} ${SOUND})

    # A configured MAC address applies to the NIC device in every mode that
    # creates one. With network=none there is no NIC to attach it to.
    local MAC=""
    if [ -n "${macaddr}" ]; then
        if [ "${network}" == "none" ]; then
            echo " - WARNING!  macaddr '${macaddr}' is ignored because networking is disabled."
        else
            MAC=",mac=${macaddr}"
        fi
    fi

    if [ "${network}" == "none" ]; then
        # Disable all networking
        echo " - Network:  Disabled"
        args+=(-nic none)
    elif [ "${network}" == "restrict" ]; then
        echo " - Network:  Restricted (${NET_DEVICE})"
        # shellcheck disable=SC2054,SC2206
        args+=(-device ${NET_DEVICE},netdev=nic${MAC} -netdev ${NET},restrict=y,id=nic)
    elif [ "${network}" == "passt" ]; then
        # passt provides unprivileged user-mode networking; QEMU 10.1+
        echo " - Network:  passt (${NET_DEVICE})"
        if [ "${QEMU_VER_SHORT}" -lt 101 ]; then
            echo "ERROR! 'passt' networking requires QEMU 10.1 or newer."
            exit 1
        fi
        # shellcheck disable=SC2054,SC2206
        args+=(-device ${NET_DEVICE},netdev=nic${MAC} -netdev passt,id=nic)
    elif [ -n "${network}" ]; then
        # Enable bridge mode networking
        echo " - Network:  Bridged (${network})"

        local BRIDGE_HELPER=""
        BRIDGE_HELPER=$(type -p qemu-bridge-helper)
        # shellcheck disable=SC2054,SC2206
        args+=(-nic bridge,br=${network},helper=${BRIDGE_HELPER},model=${NET_DEVICE}${MAC})
    else
        echo " - Network:  User (${NET_DEVICE})"
        # shellcheck disable=SC2054,SC2206
        args+=(-device ${NET_DEVICE},netdev=nic${MAC} -netdev ${NET},id=nic)
    fi

    # Add the disks
    # - https://turlucode.com/qemu-disk-io-performance-comparison-native-or-threads-windows-10-version/
    # Optimise disk I/O: enable TRIM/discard, zero detection for thin provisioning,
    # writeback caching and threaded async I/O
    DRIVE_OPTIMISATIONS="discard=unmap,detect-zeroes=unmap,cache=writeback,aio=threads"

    if [[ "${boot}" == *"efi"* ]]; then
        if [ "${ARCH_VM}" == "aarch64" ] &&
            [ "${EFI_MODE}" == "bios" ]; then
            # ARM64 monolithic EDK2 firmware.
            # QEMU loads this directly without a persistent pflash vars store.
            args+=(-bios "${EFI_CODE}")
        else
            QCOW2CODE=$(is_firmware_qcow2 "${EFI_CODE}")
            QCOW2VARS=$(is_firmware_qcow2 "${EFI_VARS}")

            if [ "${QCOW2CODE}" = "true" ]; then
                EFI_CODE_FORMAT="qcow2"
            else
                EFI_CODE_FORMAT="raw"
            fi

            if [ "${QCOW2VARS}" = "true" ]; then
                EFI_VARS_FORMAT="qcow2"
            else
                EFI_VARS_FORMAT="raw"
            fi

            if [ "${ARCH_VM}" == "aarch64" ]; then
                # ARM64 pflash uses named blockdev nodes referenced by
                # pflash0/pflash1 on the virt machine.
                # Do not use cfi.pflash01 secure properties; those are
                # specific to the x86 SMM implementation.
                # shellcheck disable=SC2054
                args+=(
                    -blockdev node-name=rom,driver=file,filename="${EFI_CODE}",read-only=true
                    -blockdev node-name=efivars,driver=file,filename="${EFI_VARS}"
                )
            else
                # x86 uses traditional pflash drives with Secure Boot support.
                # shellcheck disable=SC2054
                args+=(
                    -global driver=cfi.pflash01,property=secure,value=on
                    -drive if=pflash,format="${EFI_CODE_FORMAT}",unit=0,file="${EFI_CODE}",readonly=on
                    -drive if=pflash,format="${EFI_VARS_FORMAT}",unit=1,file="${EFI_VARS}"
                )
            fi
        fi
    fi
    vm_boot_media

    if [ "${guest_os}" == "macos" ]; then
        # shellcheck disable=SC2054
        args+=(-device ahci,id=ahci
            -device ide-hd,bus=ahci.0,drive=BootLoader,bootindex=0
            -drive id=BootLoader,if=none,format=qcow2,file="${MAC_BOOTLOADER}")

        if [ -n "${img}" ]; then
            # shellcheck disable=SC2054
            args+=(-device ide-hd,bus=ahci.1,drive=RecoveryImage
                -drive id=RecoveryImage,if=none,format=raw,file="${img}")
        fi

        # shellcheck disable=SC2054,SC2206
        args+=(-device ${MAC_DISK_DEV},drive=SystemDisk
            -drive id=SystemDisk,if=none,format=${disk_format},file="${disk_img}",${DRIVE_OPTIMISATIONS} ${STATUS_QUO})
    elif [ "${guest_os}" == "kolibrios" ]; then
        # shellcheck disable=SC2054,SC2206
        args+=(-device ahci,id=ahci
            -device ide-hd,bus=ahci.0,drive=SystemDisk
            -drive id=SystemDisk,if=none,format=${disk_format},file="${disk_img}",${DRIVE_OPTIMISATIONS} ${STATUS_QUO})

    elif [ "${guest_os}" == "batocera" ]; then
        # shellcheck disable=SC2054,SC2206
        args+=(-device virtio-blk-pci,drive=BootDisk
            -drive id=BootDisk,if=none,format=raw,file="${img}"
            -device virtio-blk-pci,drive=SystemDisk
            -drive id=SystemDisk,if=none,format=${disk_format},file="${disk_img}",${DRIVE_OPTIMISATIONS} ${STATUS_QUO})

    elif [ "${guest_os}" == "reactos" ]; then
        # https://reactos.org/wiki/QEMU
        # shellcheck disable=SC2054,SC2206
        args+=(-drive if=ide,index=0,media=disk,file="${disk_img}" ${STATUS_QUO})

    elif [ "${guest_os}" == "templeos" ]; then
        # TempleOS supports ATA/IDE disks only
        # shellcheck disable=SC2054,SC2206
        args+=(-drive if=ide,index=0,media=disk,file="${disk_img}" ${STATUS_QUO})

    elif [ "${guest_os}" == "windows-server" ]; then
        # shellcheck disable=SC2054,SC2206
        args+=(-device ide-hd,drive=SystemDisk
            -drive id=SystemDisk,if=none,format=${disk_format},file="${disk_img}",${DRIVE_OPTIMISATIONS} ${STATUS_QUO})

    else
        if [ "${ARCH_VM}" == "aarch64" ]; then
            # ARM64: bootindex=2 ensures disk boots after CD-ROM (bootindex=1) during installation
            # shellcheck disable=SC2054,SC2206
            args+=(-device virtio-blk-pci,drive=SystemDisk,bootindex=2
                -drive id=SystemDisk,if=none,format=${disk_format},file="${disk_img}",${DRIVE_OPTIMISATIONS} ${STATUS_QUO})
        else
            # shellcheck disable=SC2054,SC2206
            args+=(-device virtio-blk-pci,drive=SystemDisk
                -drive id=SystemDisk,if=none,format=${disk_format},file="${disk_img}",${DRIVE_OPTIMISATIONS} ${STATUS_QUO})
        fi
    fi

    vm_boot_fileshare

    vm_boot_passthru

    vm_boot_channels

    if [ -n "${extra_args}" ]; then
        # shellcheck disable=SC2206
        args+=(${extra_args})
    fi

    # The OSK parameter contains parenthesis, they need to be escaped in the shell
    # scripts. The vendor name, Quickemu Project, contains a space. It needs to be
    # double-quoted.

    if [ "${display}" == "vnc" ]; then
        args+=(-vnc "${vnc_listen}")
        echo " - VNC:      Listening on ${vnc_listen}"
    fi

    # Kernel/initrd override (e.g. unattended installs, direct kernel boot).
    if [ -n "${kernel}" ]; then
        args+=(-kernel "${kernel}")
        [ -n "${initrd}" ] && args+=(-initrd "${initrd}")
        [ -n "${boot_args}" ] && args+=(-append "${boot_args}")
    fi

    # NoCloud/cloud-init seed image attached as a cdrom.
    if [ -n "${seed}" ]; then
        if [ -e "${seed}" ]; then
            # shellcheck disable=SC2054
            args+=(-drive file="${seed}",media=cdrom,if=virtio,format=raw)
        else
            echo " - WARNING! seed '${seed}' not found, skipping."
        fi
    fi

    SHELL_ARGS="${args[*]}"
    SHELL_ARGS="${SHELL_ARGS//\(/\\(}"
    SHELL_ARGS="${SHELL_ARGS//)/\\)}"
    SHELL_ARGS="${SHELL_ARGS//Quickemu Project/\"Quickemu Project\"}"

}

# ---- firmware.sh ----
# VM firmware artefacts (OVMF/AAVMF vars).

function efi_vars() {
    local VARS_IN=""
    local VARS_OUT=""
    VARS_IN="${1}"
    VARS_OUT="${2}"

    if [ ! -e "${VARS_OUT}" ]; then
        if [ -e "${VARS_IN}" ]; then
            cp "${VARS_IN}" "${VARS_OUT}"
        else
            echo "ERROR! ${VARS_IN} was not found. Please install edk2."
            exit 1
        fi
    fi
}

function is_firmware_qcow2() {
    # Check for the magic bytes that indicate the firmware is in qcow2 format,
    # otherwise default to assuming firmware files are in raw format.
    # Use od to read bytes as hex to avoid null byte warnings in command substitution.
    local magic
    magic=$(od -An -tx1 -N4 "$1" 2>/dev/null | tr -d ' ')
    [ "$magic" = "514649fb" ] && echo "true" || echo "false"
}

# ---- snapshots.sh ----
# VM snapshot operations.

function snapshot_apply() {
    echo "Snapshot apply to ${disk_img}"
    local TAG="${1}"
    if [ -z "${TAG}" ]; then
        echo " - ERROR! No snapshot tag provided."
        exit
    fi
    # qemu-img snapshot writes the image metadata; doing so while QEMU holds
    # the disk corrupts the running guest's view of it.
    if vm_running "${VMDIR}" "${VMNAME}"; then
        echo " - ERROR! Stop ${VMNAME} before managing snapshots."
        exit 1
    fi

    if [ -e "${disk_img}" ]; then
        if ${QEMU_IMG} snapshot -q -a "${TAG}" "${disk_img}"; then
            echo " - Applied snapshot '${TAG}' to ${disk_img}"
        else
            echo " - ERROR! Failed to apply snapshot '${TAG}' to ${disk_img}"
        fi
    else
        echo " - NOTE! ${disk_img} not found. Doing nothing."
    fi
}

function snapshot_create() {
    echo "Snapshotting ${disk_img}"
    local TAG="${1}"
    if [ -z "${TAG}" ]; then
        echo "- ERROR! No snapshot tag provided."
        exit
    fi
    if vm_running "${VMDIR}" "${VMNAME}"; then
        echo " - ERROR! Stop ${VMNAME} before managing snapshots."
        exit 1
    fi

    if [ -e "${disk_img}" ]; then
        if ${QEMU_IMG} snapshot -q -c "${TAG}" "${disk_img}"; then
            echo " - Created snapshot '${TAG}' for ${disk_img}"
        else
            echo " - ERROR! Failed to create snapshot '${TAG}' for ${disk_img}"
        fi
    else
        echo " - NOTE! ${disk_img} not found. Doing nothing."
    fi
}

function snapshot_delete() {
    echo "Snapshot removal ${disk_img}"
    local TAG="${1}"
    if [ -z "${TAG}" ]; then
        echo " - ERROR! No snapshot tag provided."
        exit
    fi
    if vm_running "${VMDIR}" "${VMNAME}"; then
        echo " - ERROR! Stop ${VMNAME} before managing snapshots."
        exit 1
    fi

    if [ -e "${disk_img}" ]; then
        if ${QEMU_IMG} snapshot -q -d "${TAG}" "${disk_img}"; then
            echo " - Deleted snapshot '${TAG}' from ${disk_img}"
        else
            echo " - ERROR! Failed to delete snapshot '${TAG}' from ${disk_img}"
        fi
    else
        echo " - NOTE! ${disk_img} not found. Doing nothing."
    fi
}

function snapshot_info() {
    echo
    if [ -e "${disk_img}" ]; then
        ${QEMU_IMG} info "${disk_img}"
    fi
}

# Structured snapshot list for '--snapshot info|list --format json'.
# 'qemu-img snapshot -l --output json' is rejected by some builds, but
# 'qemu-img info --output=json' embeds the same snapshot table.
# Keep this in sync with schemas/v1/vm-snapshots.schema.json.
function snapshot_list_json() {
    local SNAPS="[]"
    if [ -e "${disk_img}" ] && command -v jq >/dev/null 2>&1; then
        SNAPS="$(${QEMU_IMG} info --output=json "${disk_img}" \
            | jq -c '[.snapshots[]? | {
                id: .id,
                name: .name,
                date: (.["date-sec"] + ((."date-nsec" // 0) / 1000000000)),
                "vm-state-size": (."vm-state-size" // 0)
              }]')" || SNAPS="[]"
    fi
    qe_ok "vm.snapshot.list" "$(printf '{"vm":"%s","snapshots":%s}' \
        "$(json_escape "${VMNAME}")" "${SNAPS:-[]}")"
}
