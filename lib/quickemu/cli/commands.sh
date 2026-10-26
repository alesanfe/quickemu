# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Sourced partial: shares the conf-schema global namespace with bin/quickemu.
# CLI commands: exit codes, per-option validation and command implementations.

# ---- exit-codes.sh ----
# shellcheck disable=SC2034 # constants consumed by the main script
# Stable exit codes (public contract).
readonly QE_EXIT_OK=0
readonly QE_EXIT_ERROR=1

# ---- param-checks.sh ----
# Command-line parameter validation (--display, --viewer, ...).

function display_param_check() {
    # Braille support requires SDL. Override $display if braille was requested.
    if [ -n "${BRAILLE}" ]; then
        display="sdl"
        # XHCI supports USB 1.1/2.0/3.0; required for full-speed braille devices
        usb_controller="xhci"
    fi

    # Fallback to SDL if GTK display is not available. QEMU may support the
    # gtk backend without a reachable display server (e.g. headless hosts),
    # in which case GTK would fail to initialise at runtime.
    if [ "${display}" == "gtk" ]; then
        if ! "${QEMU}" -display help 2>&1 | grep -q "^gtk$" || { [ -z "${DISPLAY}" ] && [ -z "${WAYLAND_DISPLAY}" ]; }; then
            echo " - NOTE: GTK display not available, falling back to SDL"
            display="sdl"
        fi
    fi

    if [ "${OS_KERNEL}" == "Darwin" ]; then
        if [ "${display}" != "cocoa" ] && [ "${display}" != "none" ]; then
            echo "ERROR! Requested output '${display}' but only 'cocoa' and 'none' are avalible on macOS."
            exit 1
        fi
    else
        if [ "${display}" != "gtk" ] && [ "${display}" != "none" ] && [ "${display}" != "sdl" ] && [ "${display}" != "spice" ] && [ "${display}" != "spice-app" ] && [ "${display}" != "vnc" ]; then
            echo "ERROR! Requested output '${display}' is not recognised."
            exit 1
        fi
    fi

    # Set the default 3D acceleration.
    if [ -z "${gl}" ]; then
        if command -v glxinfo &>/dev/null; then
            GLSL_VER=$(glxinfo | grep "OpenGL ES GLSL" | awk '{print $NF}')
            case ${GLSL_VER} in
            1* | 2*) gl="off" ;;
            *) gl="on" ;;
            esac
        else
            gl="on"
        fi
    fi

    # Enable grab-on-hover for SDL: https://github.com/quickemu-project/quickemu/issues/541
    case "${display}" in
    sdl) export SDL_MOUSE_FOCUS_CLICKTHROUGH=1 ;;
    esac
}

function resolution_param_check() {
    if [ -n "${width}" ] && [ -z "${height}" ]; then
        echo "ERROR! '--width ${width}' requires '--height'."
        exit 1
    fi
    if [ -n "${height}" ] && [ -z "${width}" ]; then
        echo "ERROR! '--height ${height}' requires '--width'."
        exit 1
    fi
    if [ -n "${width}" ] && ! is_numeric "${width}"; then
        echo "ERROR! --width must be a number!"
        exit 1
    fi
    if [ -n "${height}" ] && ! is_numeric "${height}"; then
        echo "ERROR! --height must be a number!"
        exit 1
    fi
    if [ -n "${width}" ] && { [ "${width}" -lt 1 ] || [ "${width}" -gt 16384 ]; }; then
        echo "ERROR! --width must be between 1 and 16384."
        exit 1
    fi
    if [ -n "${height}" ] && { [ "${height}" -lt 1 ] || [ "${height}" -gt 16384 ]; }; then
        echo "ERROR! --height must be between 1 and 16384."
        exit 1
    fi
}

# guest_os drives firmware/disk/display heuristics; a value outside the
# charset quickget produces is either a typo or a crafted value. The full
# OS enum lives in quickget (os_info), so here we check shape, not
# membership: empty or non [a-z0-9-] values are rejected.
function guest_os_param_check() {
    if [ -z "${guest_os}" ]; then
        echo "WARNING! guest_os is not set in ${VM}; assuming 'linux' heuristics." >&2
        return 0
    fi
    if [[ ! "${guest_os}" =~ ^[a-z0-9-]+$ ]]; then
        echo "ERROR! '${guest_os}' is not a valid guest_os (expected [a-z0-9-]+)."
        exit 1
    fi
}

# ACCESS/SSH_ACCESS accept a scope keyword ('local'|'remote') or a single
# host/IP token to bind listeners to. Anything else (spaces, shell
# metacharacters) would produce a malformed hostfwd/bind address.
function access_param_check() {
    local VAR="" VALUE=""
    for VAR in ACCESS SSH_ACCESS; do
        VALUE="${!VAR}"
        [ -z "${VALUE}" ] && continue
        case "${VALUE}" in
        local | remote) ;;
        *)
            if [[ ! "${VALUE}" =~ ^[a-zA-Z0-9._-]+$ ]]; then
                echo "ERROR! '${VALUE}' is not recognised for ${VAR}. Use 'local', 'remote' or a host/IP."
                exit 1
            fi
            ;;
        esac
    done
}

# macaddr is applied to the QEMU NIC model string; an unchecked value could
# inject extra ,key=val options into -device/-nic arguments.
function network_param_check() {
    if [ -n "${macaddr}" ] && [[ ! "${macaddr}" =~ ^([0-9A-Fa-f]{2}:){5}[0-9A-Fa-f]{2}$ ]]; then
        echo "ERROR! '${macaddr}' is not a valid MAC address for macaddr."
        exit 1
    fi
}

function ports_param_check() {
    local FORWARD=""
    for FORWARD in "${port_forwards[@]}"; do
        if [[ ! "${FORWARD}" =~ ^[0-9]+:[0-9]+$ ]]; then
            echo "ERROR! '${FORWARD}' is not a valid port forward (expected host:guest numbers)."
            exit 1
        fi
    done
    if [ -n "${ssh_port}" ] && ! is_numeric "${ssh_port}"; then
        echo "ERROR: ssh_port must be a number!"
        exit 1
    fi

    if [ -n "${spice_port}" ] && ! is_numeric "${spice_port}"; then
        echo "ERROR: spice_port must be a number!"
        exit 1
    fi

    if [ -n "${monitor_telnet_port}" ] && ! is_numeric "${monitor_telnet_port}"; then
        echo "ERROR: telnet port must be a number!"
        exit 1
    fi

    # -vnc host:display (display 0-99 maps to TCP 5900+display); host may be
    # empty (listen on all interfaces) or a hostname/IP. Reject anything
    # else so the value cannot smuggle QEMU options into -vnc.
    if [ -n "${vnc_listen}" ] && [[ ! "${vnc_listen}" =~ ^([A-Za-z0-9._-]+)?:[0-9]{1,2}$ ]] && [[ ! "${vnc_listen}" =~ ^\[[0-9A-Fa-f:]+\]:[0-9]{1,2}$ ]]; then
        echo "ERROR! '${vnc_listen}' is not a valid VNC listen address (expected host:display)."
        exit 1
    fi

    if [ -n "${serial_telnet_port}" ] && ! is_numeric "${serial_telnet_port}"; then
        echo "ERROR: serial port must be a number!"
        exit 1
    fi
}

function sound_card_param_check() {
    if [ -n "${sound_card}" ] && [ "${sound_card}" != "ac97" ] && [ "${sound_card}" != "es1370" ] && [ "${sound_card}" != "ich9-intel-hda" ] && [ "${sound_card}" != "intel-hda" ] && [ "${sound_card}" != "sb16" ] && [ "${sound_card}" != "usb-audio" ] && [ "${sound_card}" != "virtio-sound-pci" ] && [ "${sound_card}" != "none" ]; then
        echo "ERROR! Requested sound card '${sound_card}' is not recognised."
        exit 1
    fi

    # USB audio requires xhci controller
    if [ "${sound_card}" == "usb-audio" ]; then
        usb_controller="xhci"
    fi

    #name "hda-duplex", bus HDA, desc "HDA Audio Codec, duplex (line-out, line-in)"
    #name "hda-micro", bus HDA, desc "HDA Audio Codec, duplex (speaker, microphone)"
    #name "hda-output", bus HDA, desc "HDA Audio Codec, output-only (line-out)"
    if [ "${sound_duplex}" != "hda-duplex" ] && [ "${sound_duplex}" != "hda-micro" ] && [ "${sound_duplex}" != "hda-output" ]; then
        echo "ERROR! Requested sound duplex '${sound_duplex}' is not recognised."
        exit 1
    fi
}

function tpm_param_check() {
    if [ "${tpm}" == "on" ]; then
        SWTPM=$(command -v swtpm)
        if [ ! -e "${SWTPM}" ]; then
            echo "ERROR! TPM is enabled, but swtpm was not found."
            exit 1
        fi
    fi
}

function viewer_param_check() {
    if [ "${OS_KERNEL}" == "Darwin" ]; then
        return
    fi

    if [ "${viewer}" != "none" ] && [ "${viewer}" != "spicy" ] && [ "${viewer}" != "remote-viewer" ]; then
        echo "ERROR! Requested viewer '${viewer}' is not recognised."
        exit 1
    fi
    if [ "${viewer}" == "spicy" ] && ! command -v spicy &>/dev/null; then
        echo "ERROR! Requested 'spicy' as viewer, but 'spicy' is not installed."
        exit 1
    elif [ "${viewer}" == "remote-viewer" ] && ! command -v remote-viewer &>/dev/null; then
        echo "ERROR! Requested 'remote-viewer' as viewer, but 'remote-viewer' is not installed."
        exit 1
    fi
}

function fileshare_param_check() {
    if [ "${PUBLIC}" == "none" ]; then
        PUBLIC=""
    else
        # PUBLICSHARE is the only directory exposed to guest VMs for file
        # sharing via 9P, spice-webdavd and Samba. This path is not configurable.
        if [ -z "${PUBLIC}" ]; then
            if command -v xdg-user-dir &>/dev/null; then
                PUBLIC=$(xdg-user-dir PUBLICSHARE)
            elif [ -d "${HOME}/Public" ]; then
                PUBLIC="${HOME}/Public"
            fi
        fi

        if [ ! -d "${PUBLIC}" ]; then
            echo " - WARNING! Public directory: '${PUBLIC}' doesn't exist!"
            PUBLIC=""
        else
            # QEMU mount tags are limited to 31 bytes; truncate long user names.
            PUBLIC_TAG="Public-${USER,,}"
            PUBLIC_TAG="${PUBLIC_TAG:0:31}"
            PUBLIC_PERMS=$(${STAT} -c "%A" "${PUBLIC}")
        fi
    fi

    # Prefer virtiofs over 9p when the standalone virtiofsd is available and the
    # guest is Linux. virtiofs uses shared memory rather than a transport protocol,
    # giving much lower latency and higher throughput than 9p.
    # NOTE: only the standalone virtiofsd (Rust) is supported — the legacy
    # QEMU-bundled C daemon (/usr/lib/qemu/virtiofsd) uses incompatible CLI
    # syntax and requires root, so it is intentionally ignored here.
    if [ -n "${PUBLIC}" ] && [ "${guest_os}" == "linux" ]; then
        VIRTIOFSD=$(command -v virtiofsd 2>/dev/null)
        if [ ! -x "${VIRTIOFSD}" ]; then
            VIRTIOFSD=""
        fi
    fi
}

# ---- status.sh ----
# VM state queries (public contract, schema v1).

function status_vm() {
    # Canonical state from the state machine (instance identity -> QMP ->
    # process markers -> filesystem), plus the ops legal from that state.
    local STATE PID_JSON="null" OPS_JSON="" OP=""
    if [ -n "${VM_PID}" ]; then
        PID_JSON="${VM_PID}"
        # Live instance: authoritative QMP state, then process markers.
        local QMP_STATE=""
        QMP_STATE=$(qmp_vm_status 2>/dev/null)
        if [ -n "${QMP_STATE}" ]; then
            case "${QMP_STATE}" in
            paused | inmigrate | suspended) STATE="paused" ;;
            *) STATE="running" ;;
            esac
        elif [ "$(ps -o stat= -p "${VM_PID}" 2>/dev/null | cut -c1)" == "T" ] || [ -f "${VMDIR}/${VMNAME}.paused" ]; then
            STATE="paused"
        else
            STATE="running"
        fi
    else
        # No live pid known: state machine resolves stopped/stale/unknown.
        STATE=$(vm_state)
    fi
    for OP in $(vm_allowed_ops "${STATE}"); do OPS_JSON+="${OPS_JSON:+,}\"${OP}\""; done
    if [ "${OUTPUT_FORMAT}" == "json" ]; then
        qe_ok "vm.status" "$(printf '{"name":"%s","state":"%s","pid":%s,"allowedOps":[%s]}' \
            "$(json_escape "${VMNAME}")" "${STATE}" "${PID_JSON}" "${OPS_JSON}")"
        exit 0
    fi
    if [ -n "${VM_PID}" ]; then
        echo "${STATE} ${VM_PID}"
        exit 0
    fi
    echo "${STATE}"
    exit 0
}

function list_vms() {
    local CONFS=() CONF="" DIR="" OS="" STATE="" PID="" FIRST=1
    shopt -s nullglob
    CONFS=(*.conf)
    shopt -u nullglob
    if [ ${#CONFS[@]} -eq 0 ]; then
        if [ "${OUTPUT_FORMAT}" == "json" ]; then
            qe_ok "vm.list" '{"vms":[]}'
        else
            echo "No VM configurations found in $(pwd)."
        fi
        return 0
    fi
    if [ "${OUTPUT_FORMAT}" == "json" ]; then
        local VMS_JSON="["
        for CONF in "${CONFS[@]}"; do
            DIR="$(basename "${CONF}" .conf)"
            OS=$(sed -n 's/^guest_os="*\([^"]*\)"*/\1/p' "${CONF}" | head -1)
            STATE="stopped"
            PID="null"
            if [ -f "${DIR}/${DIR}.pid" ]; then
                PID=$(cat "${DIR}/${DIR}.pid")
                if vm_pid_alive "${DIR}/${DIR}.pid"; then
                    if [ -f "${DIR}/${DIR}.paused" ]; then
                        STATE="paused"
                    else
                        STATE="running"
                    fi
                else
                    STATE="stale"
                    PID="null"
                fi
            fi
            [ ${FIRST} -eq 0 ] && VMS_JSON+=","
            FIRST=0
            VMS_JSON+=$(printf '{"name":"%s","guest_os":"%s","state":"%s","pid":%s}' \
                "$(json_escape "${DIR}")" "$(json_escape "${OS:-linux}")" "${STATE}" "${PID}")
        done
        qe_ok "vm.list" "{\"vms\":${VMS_JSON}]}"
        return 0
    fi
    printf '%-28s %-16s %-9s %s\n' "VM" "GUEST OS" "STATE" "PID"
    for CONF in "${CONFS[@]}"; do
        DIR="$(basename "${CONF}" .conf)"
        OS=$(sed -n 's/^guest_os="*\([^"]*\)"*/\1/p' "${CONF}" | head -1)
        STATE="stopped"
        PID=""
        if [ -f "${DIR}/${DIR}.pid" ]; then
            PID=$(cat "${DIR}/${DIR}.pid")
            if vm_pid_alive "${DIR}/${DIR}.pid"; then
                if [ -f "${DIR}/${DIR}.paused" ]; then
                    STATE="paused"
                else
                    STATE="running"
                fi
            else
                STATE="stale"
                PID=""
            fi
        fi
        printf '%-28s %-16s %-9s %s\n' "${DIR}" "${OS:-linux}" "${STATE}" "${PID}"
    done
}

# Read a .conf WITHOUT executing it: conf files are bash and are executed
# via `source` on normal launch, so imported/foreign confs should be
# inspected first. Uses the declarative conf_get parser.
function inspect_conf() {
    local F="${1:-${VM}}" KEY VALUE
    if [ ! -f "${F}" ]; then
        echo "ERROR! ${F} not found."
        return 1
    fi
    for KEY in guest_os arch boot tpm secureboot disk_img disk_format disk_size iso img fixed_iso floppy ram cpu_cores network bridge macaddr port_forwards display monitor serial sound_card public_dir seed kernel initrd boot_args gpu_passthrough vnc_listen; do
        VALUE=$(conf_get "${F}" "${KEY}" 2>/dev/null) || continue
        printf '%s="%s"\n' "${KEY}" "${VALUE}"
    done
}

function capabilities() {
    # Public contract for external consumers (quickgui, scripts, CI).
    # Keep this in sync with schemas/v1/capabilities.schema.json.
    qe_ok "capabilities" "$(cat <<JSON
{"applicationVersion":"${VERSION}","capabilities":{"structuredCatalog":true,"structuredStatus":true,"listVms":true,"snapshots":true,"structuredSnapshots":true,"monitorCommands":true,"sendKeys":true,"screenshot":true,"clone":true,"rename":true,"exportImport":true,"diskOps":true,"downloadProgress":false,"safeDeletePlan":true,"verifiableDeletePlan":true,"qmpChannel":true,"instanceIdentity":true,"sharedExclusiveLocks":true,"portRegistry":true,"verifiedArchives":true,"structuredEvents":true,"asyncOperations":true,"tomlConfig":true,"hostInventory":true}}
JSON
)"
}

# --host-info: versioned host inventory for QuickGUI (dependencies,
# acceleration and machine facts in one machine-readable call).
host_info() {
    local KVM="false"
    [ -w /dev/kvm ] && KVM="true"
    local IOMMU="false" GPUS="" FIRST=1 LINE
    if [ -d /sys/kernel/iommu_groups ] && find /sys/kernel/iommu_groups -maxdepth 2 -name devices 2>/dev/null | grep -q .; then IOMMU="true"; fi
    if command -v lspci >/dev/null 2>&1; then
        while IFS= read -r LINE; do
            [ ${FIRST} -eq 0 ] && GPUS+=","
            FIRST=0
            GPUS+="\"$(json_escape "${LINE}")\""
        done < <(lspci -Dnn 2>/dev/null | grep -Ei "vga|3d|display" | cut -d' ' -f1,4- | head -8)
    fi
    local QE_VER
    QE_VER=$(${QEMU:-qemu-system-x86_64} --version 2>/dev/null | head -1)
    local OVMF="false" AAVMF="false"
    ls /usr/share/OVMF*/OVMF_CODE.fd /usr/share/edk2/ovmf/OVMF_CODE.fd 2>/dev/null | grep -q . && OVMF="true"
    ls /usr/share/AAVMF* /usr/share/edk2/aarch64/QEMU_EFI.fd 2>/dev/null | grep -q . && AAVMF="true"
    local FREE_HUGE
    FREE=$(df -k "${XDG_DATA_HOME:-$HOME/.local/share}" 2>/dev/null | awk 'NR==2{print $4}')
    HUGE=$(awk '/HugePages_Total/{print $2}' /proc/meminfo 2>/dev/null || echo 0)
    qe_ok "host.info" "$(printf '{\"kernel\":\"%s\",\"arch\":\"%s\",\"cpuCores\":%s,\"memTotalKb\":%s,\"kvm\":%s,\"iommu\":%s,\"qemu\":\"%s\",\"qemuAarch64\":%s,\"qemuImg\":%s,\"socat\":%s,\"jq\":%s,\"swtpm\":%s,\"ovmf\":%s,\"aavmf\":%s,\"passt\":%s,\"virtiofsd\":%s,\"spicy\":%s,\"remoteViewer\":%s,\"freeKb\":%s,\"hugePages\":%s,\"gpus\":[%s]}'         "$(json_escape "$(uname -sr)")" "$(uname -m)" "$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo 0)"         "$(awk '/MemTotal/{print $2}' /proc/meminfo 2>/dev/null || echo 0)" "${KVM}" "${IOMMU}"         "$(json_escape "${QE_VER}")"         "$(command -v qemu-system-aarch64 >/dev/null && echo true || echo false)"         "$(command -v qemu-img >/dev/null && echo true || echo false)"         "$(command -v socat >/dev/null && echo true || echo false)"         "$(command -v jq >/dev/null && echo true || echo false)"         "$(command -v swtpm >/dev/null && echo true || echo false)"         "${OVMF}" "${AAVMF}"         "$(command -v passt >/dev/null && echo true || echo false)"         "$(command -v virtiofsd >/dev/null && echo true || echo false)"         "$(command -v spicy >/dev/null && echo true || echo false)"         "$(command -v remote-viewer >/dev/null && echo true || echo false)"         "${FREE:-0}" "${HUGE:-0}" "${GPUS}")"
}


# --migrate-conf: write a TOML migration of the VM config next to the
# original (the .conf is never modified or executed). With --dry-run
# print the TOML to stdout and a diff against an existing .toml.
migrate_conf() {
    if [ -n "${DRY_RUN}" ]; then
        local OUT="${VM%.conf}.toml"
        if [ -f "${OUT}" ]; then
            echo "Diff (existing ${OUT} vs regenerated):"
            diff -u "${OUT}" <(toml_from_conf "${VM}") || true
        else
            toml_from_conf "${VM}"
        fi
        return 0
    fi
    conf_migrate "${VM}" "${VM%.conf}.toml"
}

# --op-status <id>: print the state of an async operation recorded by
# export/import/clone-style long operations.
operation_status() {
    if [ -z "${OP_ID}" ]; then
        echo "ERROR! '--op-status' needs an operationId." >&2
        return 1
    fi
    op_status "${OP_ID}"
}

# --op-list: all recorded operations for the VM as one JSON object.
operation_list() { op_list; }

# --op-cancel <id>: mark an operation cancelled and kill its worker if
# a live workerPid was recorded.
operation_cancel() {
    if [ -z "${OP_ID}" ]; then
        echo "ERROR! '--op-cancel' needs an operationId." >&2
        return 1
    fi
    op_cancel "${OP_ID}"
}

# --events [cursor]: JSONL events with seq > cursor for resumable tails.
events_read() { qe_events_read "${EVENTS_CURSOR:-0}"; }

qe_doctor_wrap() { qe_doctor "${DOCTOR_FIX:-}"; }
