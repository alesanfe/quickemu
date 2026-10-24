# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Sourced partial: shares the conf-schema global namespace with bin/quickemu.
# VM lifecycle operations (stop, pause, clone, rename, ...).

function delete_shortcut() {
    local SHORTCUT_DIR="${HOME}/.local/share/applications"
    if [ -e "${SHORTCUT_DIR}/${VMNAME}.desktop" ]; then
        rm "${SHORTCUT_DIR}/${VMNAME}.desktop"
        echo " - Deleted ${SHORTCUT_DIR}/${VMNAME}.desktop"
    fi
}

function delete_vm() {
    echo "Deleting ${VMNAME} completely"
    if [ -d "${VMDIR}" ]; then
        rm -rf "${VMDIR}"
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
    if [ "${monitor}" != "none" ] && [ -S "${SOCKET_MONITOR}" ]; then
        monitor_send_cmd "stop" && echo " - ${VMNAME} (${VM_PID}) paused."
    else
        kill -STOP "${VM_PID}" && echo " - ${VMNAME} (${VM_PID}) paused."
    fi
}

function resume_vm() {
    if ! vm_running "${VMDIR}" "${VMNAME}"; then
        echo " - ${VMNAME} is not running."
        return 1
    fi
    if [ "${monitor}" != "none" ] && [ -S "${SOCKET_MONITOR}" ]; then
        monitor_send_cmd "cont" && echo " - ${VMNAME} (${VM_PID}) resumed."
    else
        kill -CONT "${VM_PID}" && echo " - ${VMNAME} (${VM_PID}) resumed."
    fi
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
    if [ "${monitor}" == "socket" ] && [ ! -S "${SOCKET_MONITOR}" ]; then
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
    local IN="${IMPORT_FILE}" BASE=""
    if [ -z "${IN}" ] || [ ! -f "${IN}" ]; then
        echo "ERROR! '--import' needs an existing .tar.gz file."
        exit 1
    fi
    BASE=$(tar -tzf "${IN}" 2>/dev/null | grep -m1 '\.conf$' | xargs -r basename | sed 's/\.conf$//')
    if [ -z "${BASE}" ]; then
        echo "ERROR! No .conf found inside ${IN}."
        exit 1
    fi
    if [ -e "${BASE}.conf" ] || [ -d "${BASE}" ]; then
        echo "ERROR! ${BASE} already exists."
        exit 1
    fi
    tar -xzf "${IN}"
    echo " - Imported ${BASE} from ${IN}"
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
    sed -i "s|${VMNAME}/|${NEW_NAME}/|g" "${PARENT}/${NEW_NAME}.conf"
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
        qe_safe_rm "${VMDIR}/${VMNAME}.pid"
        qe_safe_rm "${VMDIR}/${VMNAME}.spice"
        qe_safe_rm "${VMDIR}/${VMNAME}.sock"
        stop_virtiofsd
    elif [ -n "${VM_PID}" ]; then
        if kill -9 "${VM_PID}" >/dev/null 2>&1; then
            echo " - ${VMNAME} (${VM_PID}) killed."
            qe_safe_rm "${VMDIR}/${VMNAME}.pid"
            qe_safe_rm "${VMDIR}/${VMNAME}.spice"
            qe_safe_rm "${VMDIR}/${VMNAME}.sock"
            stop_virtiofsd
        else
            echo " - ${VMNAME} (${VM_PID}) was not killed."
        fi
    elif [ ! -r "${VMDIR}/${VMNAME}.pid" ]; then
        echo " - ${VMNAME} has no ${VMDIR}/${VMNAME}.pid"
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
