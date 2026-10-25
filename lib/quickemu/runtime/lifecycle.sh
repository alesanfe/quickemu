# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Sourced partial: shares the conf-schema global namespace with bin/quickemu.
# Runtime lifecycle: helper daemons, pidfiles and port bookkeeping.

# ---- daemons.sh ----
# Companion daemon lifecycle (virtiofsd, SPICE viewer).

function start_virtiofsd() {
    # Start virtiofsd as a background daemon and record its PID so it can be
    # cleaned up when the VM exits. The socket path is placed alongside other
    # VM runtime files in VMDIR.
    if [ -z "${VIRTIOFSD}" ]; then
        return
    fi

    # Skip virtiofs when booting from an ISO (installation). By this point in
    # vm_boot(), quickemu has already cleared $iso when the disk is in use, so
    # a non-empty $iso reliably means we are booting the installer. The
    # shared-memory NUMA backend required for virtiofs can cause installer hangs.
    if [ -n "${iso}" ]; then
        VIRTIOFSD=""
        return
    fi

    VIRTIOFSD_SOCKET="${VMDIR}/${VMNAME}.virtiofsd-sock"
    if [ -n "${DRY_RUN}" ]; then
        # Planning only: report the socket the real boot would create.
        echo " - virtiofsd: ${VIRTIOFSD_SOCKET} (planned)"
        return
    fi
    # Remove any stale socket left by an unclean shutdown. quickemu already
    # checks the PID file and refuses to start if the VM is running, so a
    # live virtiofsd cannot be behind this socket at this point.
    rm -f "${VIRTIOFSD_SOCKET}"
    local virtiofsd_args=(
        --socket-path="${VIRTIOFSD_SOCKET}"
        --shared-dir="${PUBLIC}"
        --announce-submounts
        --translate-uid="squash-guest:0:$(id -u):65536" # any guest UID in 0..65535 becomes current host UID
        --translate-gid="squash-guest:0:$(id -g):65536" # any guest GID in 0..65535 becomes current host GID
    )

    # Capture virtiofsd stderr separately so we can inspect it without
    # false-matching stale entries from previous runs in the shared VM log.
    local virtiofsd_stderr
    virtiofsd_stderr=$(mktemp)
    echo "${VIRTIOFSD} ${virtiofsd_args[*]} &" >>"${VMDIR}/${VMNAME}.sh"
    ${VIRTIOFSD} "${virtiofsd_args[@]}" >>"${VMDIR}/${VMNAME}.log" 2>"${virtiofsd_stderr}" &

    # virtiofsd forks: the shell child exits once the daemon child is running.
    # Poll for the socket rather than using a fixed sleep to avoid a race.
    local i
    for i in $(seq 1 20); do
        [ -S "${VIRTIOFSD_SOCKET}" ] && break
        sleep 0.1
    done

    if [ ! -S "${VIRTIOFSD_SOCKET}" ]; then
        if grep -q "Operation not permitted" "${virtiofsd_stderr}" 2>/dev/null; then
            echo " - WARNING! virtiofsd failed to start (insufficient permissions); falling back to 9p."
            echo "            Install the standalone virtiofsd package to enable virtiofs support."
        else
            echo " - WARNING! virtiofsd failed to start; falling back to 9p."
        fi
        cat "${virtiofsd_stderr}" >>"${VMDIR}/${VMNAME}.log"
        rm -f "${virtiofsd_stderr}"
        VIRTIOFSD=""
        VIRTIOFSD_SOCKET=""
        VIRTIOFSD_PID=""
        return
    fi
    cat "${virtiofsd_stderr}" >>"${VMDIR}/${VMNAME}.log"
    rm -f "${virtiofsd_stderr}"

    # Resolve the daemon child's PID via the socket to avoid PID reuse issues.
    # Prefer fuser (unambiguous socket owner); fall back to pgrep with a comm
    # check to confirm the process is actually virtiofsd.
    if command -v fuser >/dev/null 2>&1; then
        VIRTIOFSD_PID=$(fuser "${VIRTIOFSD_SOCKET}" 2>/dev/null | tr -s ' ' '\n' | grep -m1 '[0-9]')
    else
        local candidate socket_pat
        # Escape regex metacharacters in the socket path before passing to pgrep -f.
        # shellcheck disable=SC2016 # single-quoted sed script on purpose
        socket_pat=$(printf '%s' "${VIRTIOFSD_SOCKET}" | sed 's/[[\.*^$()+?{}|]/\\&/g')
        candidate=$(pgrep -f "virtiofsd.*${socket_pat}" 2>/dev/null | head -1)
        if [ "$(ps -p "${candidate}" -o comm= 2>/dev/null)" = "virtiofsd" ]; then
            VIRTIOFSD_PID="${candidate}"
        fi
    fi
    echo "${VIRTIOFSD_PID}" >"${VMDIR}/${VMNAME}.virtiofsd-pid"
    echo " - virtiofsd: ${VIRTIOFSD_SOCKET} (${VIRTIOFSD_PID})"
}

function stop_virtiofsd() {
    local pid_file="${VMDIR}/${VMNAME}.virtiofsd-pid"
    local socket="${VMDIR}/${VMNAME}.virtiofsd-sock"
    local pid=""

    if [ -n "${VIRTIOFSD_PID}" ]; then
        pid="${VIRTIOFSD_PID}"
    elif [ -r "${pid_file}" ]; then
        pid=$(cat "${pid_file}")
    fi

    if [ -n "${pid}" ] && kill -0 "${pid}" 2>/dev/null; then
        # Guard against PID reuse: only signal the process if it is still
        # a virtiofsd instance.
        if [ "$(ps -p "${pid}" -o comm= 2>/dev/null)" = "virtiofsd" ]; then
            # Ask virtiofsd to shut down gracefully first; it will close the
            # vhost-user socket and flush any pending I/O before exiting.
            kill -TERM "${pid}" 2>/dev/null
            local i
            for i in 1 2 3 4 5; do
                kill -0 "${pid}" 2>/dev/null || break
                sleep 0.2
            done
            # Force-kill only if it is still alive after the grace period.
            if kill -0 "${pid}" 2>/dev/null; then
                kill -KILL "${pid}" 2>/dev/null
            fi
        fi
    fi

    rm -f "${pid_file}" "${socket}"
    VIRTIOFSD_PID=""
    VIRTIOFSD_SOCKET=""
}

function start_viewer {
    # Exit early if viewer is disabled or display is not SPICE
    if [ "${viewer}" == "none" ] || [ "${display}" != "spice" ]; then
        return
    fi

    # Build viewer arguments based on connection mode (Unix socket or TCP)
    local viewer_args=()
    local viewer_uri=""
    local errno=0

    # Determine connection mode from .spice file content
    # - Unix socket mode: file contains a path (with /)
    # - TCP mode: file contains just a port number
    local spice_info=""
    if [ -r "${VMDIR}/${VMNAME}.spice" ]; then
        spice_info=$(cat "${VMDIR}/${VMNAME}.spice")
    fi

    if [[ "${spice_info}" == */* ]]; then
        # Unix socket mode (path contains /)
        local SPICE_SOCKET="${spice_info}"
        if [ "${viewer}" == "spicy" ]; then
            viewer_args+=("--uri=spice+unix://${SPICE_SOCKET}")
        else
            viewer_uri="spice+unix://${SPICE_SOCKET}"
        fi
    elif [ -n "${spice_info}" ]; then
        # TCP mode (port number)
        if [ "${viewer}" == "spicy" ]; then
            viewer_args+=("--port" "${spice_info}")
        else
            viewer_uri="spice://localhost:${spice_info}"
        fi
    else
        # Fallback: no .spice file, use ACCESS variable to determine mode
        if [ -z "${ACCESS}" ] || [ "${ACCESS}" == "local" ]; then
            # Unix socket mode
            local SPICE_SOCKET="${VMDIR}/${VMNAME}.sock"
            if [ "${viewer}" == "spicy" ]; then
                viewer_args+=("--uri=spice+unix://${SPICE_SOCKET}")
            else
                viewer_uri="spice+unix://${SPICE_SOCKET}"
            fi
        else
            # TCP mode for remote access
            if [ "${viewer}" == "spicy" ]; then
                viewer_args+=("--port" "${spice_port}")
            else
                viewer_uri="spice://localhost:${spice_port}"
            fi
        fi
    fi

    # Add common arguments
    viewer_args+=("--title" "${VMNAME}")

    # Add shared directory if configured (not for macOS guests)
    if [ "${guest_os}" != "macos" ] && [ -n "${PUBLIC}" ]; then
        viewer_args+=("--spice-shared-dir" "${PUBLIC}")
    fi

    # Add fullscreen if requested
    if [ -n "${FULLSCREEN}" ]; then
        viewer_args+=("${FULLSCREEN}")
    fi

    # Add URI for remote-viewer (spicy uses --uri= in the args already)
    if [ "${viewer}" == "remote-viewer" ] && [ -n "${viewer_uri}" ]; then
        viewer_args+=("${viewer_uri}")
    fi

    # Add extra arguments
    if [ -n "${viewer_extra_args}" ]; then
        # shellcheck disable=SC2206
        viewer_args+=(${viewer_extra_args})
    fi

    # Launch the viewer
    echo " - Viewer:   ${viewer} ${viewer_args[*]} >/dev/null 2>&1 &"
    "${viewer}" "${viewer_args[@]}" >/dev/null 2>&1 &
    errno=$?

    if [ ${errno} -ne 0 ]; then
        echo "WARNING! Could not start viewer (${viewer}) Err: ${errno}"
    fi
}

# ---- pid.sh ----
# PID file helpers for VM liveness.

# Best-effort process name: ps -o comm= where supported (Linux, macOS,
# BSD), /proc/<pid>/exename elsewhere (Cygwin/MSYS2).
vm_proc_comm() { # pid -> process name or empty
    local pid="${1}" c=""
    c=$(ps -p "${pid}" -o comm= 2>/dev/null | tr -d ' ')
    if [ -z "${c}" ] && [ -r "/proc/${pid}/exename" ]; then
        c=$(basename "$(cat "/proc/${pid}/exename" 2>/dev/null)" 2>/dev/null)
    fi
    printf '%s' "${c%.exe}"
}

# ---- instance identity ----
# Strong per-boot identity: a pidfile alone cannot distinguish a recycled
# PID or a foreign process. qemu_spawn publishes
# ${VMDIR}/${VMNAME}.runtime.json with the instance UUID, PID, process
# start-time, owner uid and a stable fingerprint; vm_instance_verify then
# answers "is this exactly the instance quickemu registered?".

vm_instance_uuid() {
    if [ -r /proc/sys/kernel/random/uuid ]; then
        cat /proc/sys/kernel/random/uuid
    else
        printf '%s-%s-%s\n' "$(date +%s%N 2>/dev/null || date +%s)" "$$" "${RANDOM:-0}"
    fi
}

# jiffies-since-boot start time (Linux/MSYS2 /proc). Empty when unavailable.
vm_proc_start_time() { # pid -> start-time or empty
    awk '{print $22}' "/proc/${1}/stat" 2>/dev/null
}

# Write VMDIR/VMNAME.runtime.json atomically for a live QEMU pid.
vm_instance_publish() { # pid -> 0 ok / 1 failed
    local pid="${1}" tmp id st fp
    id=$(vm_instance_uuid)
    st=$(vm_proc_start_time "${pid}")
    fp=$(printf '%s' "${QEMU:-qemu}|${VM}|${VMNAME}" | sha256sum 2>/dev/null | awk '{print "sha256:"$1}')
    tmp="${VMDIR}/.${VMNAME}.runtime.json.tmp"
    printf '{"schemaVersion":%s,"instanceId":"%s","vmId":"%s","pid":%s,"processStartTime":%s,"uid":%s,"commandFingerprint":"%s","configPath":"%s","createdAt":"%s"}\n' \
        "${QE_SCHEMA_VERSION:-1}" "$(json_escape "${id}")" "$(json_escape "${VMNAME}")" \
        "${pid}" "${st:-null}" "$(id -u)" "${fp}" \
        "$(json_escape "${VM}")" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" >"${tmp}" || return 1
    chmod 600 "${tmp}" 2>/dev/null || true
    mv -f "${tmp}" "${VMDIR}/${VMNAME}.runtime.json"
}

# Reads <pidfile> (and its sibling .runtime.json when present) and prints a
# typed status word; exit 0 only when the process IS this instance.
#   alive            verified instance (runtime + pid + uid + starttime + comm)
#   alive-legacy     live process, no runtime record (pre-runtime VMs)
#   dead             no live process
#   pid-reused       live PID but start-time/proof differs from the runtime
#   foreign-process  live PID belonging to an unrelated process
#   uid-mismatch     live PID owned by a different uid
#   runtime-corrupt  runtime JSON present but invalid/inconsistent
vm_instance_verify() { # pidfile -> status word
    local pf="${1}" rt pid st comm expect rt_pid rt_st rt_uid
    [ -f "${pf}" ] || { echo dead; return 1; }
    pid=$(tr -d '[:space:]' <"${pf}")
    case "${pid}" in '' | *[!0-9]*) echo dead; return 1 ;; esac
    rt="${pf%.pid}.runtime.json"
    expect=$(basename "${pf}" .pid)
    if [ ! -f "${rt}" ]; then
        kill -0 "${pid}" 2>/dev/null || { echo dead; return 1; }
        comm=$(vm_proc_comm "${pid}")
        case "${comm}" in
        "${expect}" | "${expect:0:15}" | qemu*) echo alive-legacy; return 0 ;;
        esac
        echo foreign-process; return 1
    fi
    rt_pid=$(sed -n 's/.*"pid":\([0-9][0-9]*\).*/\1/p' "${rt}")
    rt_st=$(sed -n 's/.*"processStartTime":\([0-9][0-9]*\).*/\1/p' "${rt}")
    rt_uid=$(sed -n 's/.*"uid":\([0-9][0-9]*\).*/\1/p' "${rt}")
    if [ -z "${rt_pid}" ] || [ "${rt_pid}" != "${pid}" ]; then
        echo runtime-corrupt; return 1
    fi
    kill -0 "${pid}" 2>/dev/null || { echo dead; return 1; }
    if [ -n "${rt_uid}" ] && [ "${rt_uid}" != "$(id -u)" ]; then
        echo uid-mismatch; return 1
    fi
    st=$(vm_proc_start_time "${pid}")
    if [ -n "${rt_st}" ] && [ -n "${st}" ] && [ "${st}" != "${rt_st}" ]; then
        echo pid-reused; return 1
    fi
    comm=$(vm_proc_comm "${pid}")
    case "${comm}" in
    "${expect}" | "${expect:0:15}" | qemu*) echo alive; return 0 ;;
    esac
    echo foreign-process; return 1
}

vm_pid_alive() { # pidfile -> 0 if process alive AND belongs to the VM
    local st
    st=$(vm_instance_verify "${1}")
    [ "${st}" = alive ] || [ "${st}" = alive-legacy ]
}

# True when the VM's pidfile exists and its process is alive.
vm_running() { vm_pid_alive "${1}/${2:-$(basename "${1}" .conf)}.pid"; }

function stop_swtpm() {
    local pid_file="${VMDIR}/${VMNAME}.swtpm-pid"
    local socket="${VMDIR}/${VMNAME}.swtpm-sock"
    local pid=""

    if [ -r "${pid_file}" ]; then
        pid=$(tr -d '[:space:]' <"${pid_file}")
    fi

    if [ -n "${pid}" ] && kill -0 "${pid}" 2>/dev/null; then
        # Guard against PID reuse: only signal the process if it is still
        # a swtpm instance.
        if [ "$(ps -p "${pid}" -o comm= 2>/dev/null)" = "swtpm" ]; then
            kill -TERM "${pid}" 2>/dev/null
            local i
            for i in 1 2 3 4 5; do
                kill -0 "${pid}" 2>/dev/null || break
                sleep 0.2
            done
            if kill -0 "${pid}" 2>/dev/null; then
                kill -KILL "${pid}" 2>/dev/null
            fi
        fi
    fi

    rm -f "${pid_file}" "${socket}"
}

# Remove every runtime artefact a boot can leave behind and stop the
# companion daemons (virtiofsd, swtpm). Idempotent; safe on a VM that is
# already stopped, was killed, or exited on its own.
function vm_runtime_cleanup() {
    stop_virtiofsd
    stop_swtpm
    rm -f "${VMDIR}/${VMNAME}.pid" \
        "${VMDIR}/${VMNAME}.spice" \
        "${VMDIR}/${VMNAME}.sock" \
        "${VMDIR}/${VMNAME}-monitor.socket" \
        "${VMDIR}/${VMNAME}-qmp.socket" \
        "${VMDIR}/${VMNAME}-serial.socket" \
        "${VMDIR}/${VMNAME}-agent.sock" \
        "${VMDIR}/${VMNAME}.ports" \
        "${VMDIR}/${VMNAME}.paused" \
        "${VMDIR}/${VMNAME}.runtime.json"
    # Drop the operation lock only when the VM is fully gone.
    rm -rf "${VMDIR}/${VMNAME}.lock" "${VMDIR}/${VMNAME}.lock.shared"
    _port_registry_release
    qe_event "vm.stopped"
    return 0
}

# ---- state.sh ----
# Canonical VM state: one word answering "what is this VM doing?".
# Sources in priority order: instance identity (runtime.json) -> QMP
# query-status -> process/markers -> filesystem.
vm_state() { # -> starting|running|paused|stopping|stopped|stale|unknown
    local st qst
    st=$(vm_instance_verify "${VMDIR}/${VMNAME}.pid" 2>/dev/null)
    case "${st}" in
    alive | alive-legacy)
        qst=$(qmp_vm_status 2>/dev/null)
        case "${qst}" in
        paused | inmigrate | suspended) echo paused; return ;;
        shutdown) echo stopping; return ;;
        "") : ;; # channel absent -> fall back to markers
        *) echo running; return ;;
        esac
        if [ -f "${VMDIR}/${VMNAME}.paused" ]; then echo paused
        elif [ "$(ps -o stat= -p "$(tr -d '[:space:]' <"${VMDIR}/${VMNAME}.pid" 2>/dev/null)" 2>/dev/null | cut -c1)" == "T" ]; then echo paused
        else echo running; fi
        return ;;
    dead) [ -f "${VMDIR}/${VMNAME}.pid" ] && { echo stale; return; }; echo stopped; return ;;
    runtime-corrupt | runtime-missing) echo unknown; return ;;
    *) echo unknown; return ;; # pid-reused/foreign/uid-mismatch: not ours
    esac
}

# Actions allowed from each state (transition guard used by status JSON
# and future dispatch). Prints a space-separated list.
vm_allowed_ops() { # state -> ops
    case "${1}" in
    running) echo "pause resume shutdown reset kill status monitor-cmd screenshot send-keys type snapshot-clone export" ;;
    paused) echo "resume shutdown kill status monitor-cmd screenshot" ;;
    stopped | stale) echo "start delete clone rename export import snapshot apply create delete info list inspect-conf delete-plan" ;;
    unknown) echo "status inspect-conf" ;;
    *) echo "status" ;;
    esac
}

# ---- lock.sh ----
# Per-VM operation lock (mkdir is atomic on POSIX filesystems).
# Prevents concurrent boot/destructive operations on the same VM.

# Lock modes: exclusive (default, mutating ops) and shared (read-only
# ops like status/inspect/delete-plan). Layout per VM:
#   <vm>.lock/            exclusive holder (contains pid file)
#   <vm>.lock.shared/     reader pids, one file per reader
# A writer must win the mkdir AND wait for readers to drain; readers
# fail fast while a writer holds the lock.

# Remove reader slots whose owning process is gone.
_vm_lock_prune_readers() { # shared dir
    local f pid
    [ -d "${1}" ] || return 0
    for f in "${1}"/*.pid; do
        [ -f "${f}" ] || continue
        pid=$(tr -d '[:space:]' <"${f}" 2>/dev/null)
        if [ -z "${pid}" ] || ! kill -0 "${pid}" 2>/dev/null; then
            rm -f -- "${f}"
        fi
    done
}

_vm_lock_has_live_readers() { # shared dir -> 0 if any live reader
    local f pid
    [ -d "${1}" ] || return 1
    for f in "${1}"/*.pid; do
        [ -f "${f}" ] || continue
        pid=$(tr -d '[:space:]' <"${f}" 2>/dev/null)
        [ -n "${pid}" ] && kill -0 "${pid}" 2>/dev/null && return 0
    done
    return 1
}

vm_lock_acquire() { # [shared|exclusive] -> 0 acquired / 1 already locked
    local mode="${1:-exclusive}"
    local lockdir="${VMDIR}/${VMNAME}.lock" sharedir="${VMDIR}/${VMNAME}.lock.shared"
    local tries=0 max="${VM_LOCK_TRIES:-1}" owner=""
    if [ "${mode}" = shared ]; then
        # A read lock on a VM that does not exist is trivially acquired:
        # there is nothing to read and nothing to create.
        [ -d "${VMDIR}" ] || return 0
        while :; do
            # Reclaim a dead writer before refusing readers.
            if [ -d "${lockdir}" ] && [ -f "${lockdir}/pid" ]; then
                owner=$(tr -d '[:space:]' <"${lockdir}/pid")
                [ -n "${owner}" ] && ! kill -0 "${owner}" 2>/dev/null && rm -rf "${lockdir}"
            fi
            [ -d "${lockdir}" ] || break
            tries=$((tries + 1)); [ "${tries}" -ge "${max}" ] && {
                echo "ERROR! ${VMNAME} is locked by another operation." >&2
                return 1
            }
            sleep 0.2
        done
        mkdir -p "${sharedir}" || return 1
        echo "${BASHPID:-$$}" >"${sharedir}/${BASHPID:-$$}.pid"
        # Writer may have raced in after our check: back out if so.
        if [ -d "${lockdir}" ]; then
            rm -f "${sharedir}/${BASHPID:-$$}.pid"
            echo "ERROR! ${VMNAME} is locked by another operation." >&2
            return 1
        fi
        return 0
    fi
    mkdir -p "${VMDIR}" || return 1
    while ! mkdir "${lockdir}" 2>/dev/null; do
        # Reclaim the lock if the owning process is gone (crash-safe).
        if [ -f "${lockdir}/pid" ]; then
            owner=$(tr -d '[:space:]' <"${lockdir}/pid")
            if [ -n "${owner}" ] && ! kill -0 "${owner}" 2>/dev/null; then
                rm -rf "${lockdir}"
                continue
            fi
        fi
        tries=$((tries + 1))
        if [ "${tries}" -ge "${max}" ]; then
            if [ "${OUTPUT_FORMAT:-}" == "json" ]; then
                qe_fail "vm.lock" VM_LOCKED "${VMNAME} is locked by another operation."
            else
                echo "ERROR! ${VMNAME} is locked by another operation." >&2
            fi
            return 1
        fi
        sleep 0.2
    done
    echo "$$" >"${lockdir}/pid"
    # We hold the exclusive slot: live readers must drain before we return.
    # Dead reader slots are pruned; a bounded wait avoids deadlock when a
    # reader is stuck (VM_LOCK_TRIES applies again).
    tries=0
    while _vm_lock_has_live_readers "${sharedir}"; do
        _vm_lock_prune_readers "${sharedir}"
        _vm_lock_has_live_readers "${sharedir}" || break
        tries=$((tries + 1))
        if [ "${tries}" -ge "${max}" ]; then
            rm -rf "${lockdir}"
            echo "ERROR! ${VMNAME} has readers; exclusive operation refused." >&2
            return 1
        fi
        sleep 0.2
    done
    return 0
}

vm_lock_release() { # [shared|exclusive] -> releases our own hold only
    local mode="${1:-exclusive}"
    if [ "${mode}" = shared ]; then
        rm -f "${VMDIR}/${VMNAME}.lock.shared/${BASHPID:-$$}.pid"
        rmdir "${VMDIR}/${VMNAME}.lock.shared" 2>/dev/null || true
    else
        rm -rf "${VMDIR}/${VMNAME}.lock"
    fi
}

# ---- port registry ----
# Cross-VM port reservation registry: closes most of the find-then-bind
# TOCTOU between two quickemu instances on the same host/user.
# Layout: one file per VM inside a per-user dir; each line is
#   port <TAB> vmname <TAB> pid <TAB> epoch
# The dir itself is created atomically; writers take a global mkdir lock.

QE_PORT_REGISTRY="${QE_PORT_REGISTRY:-$(qe_runtime_dir)/ports}"

_port_registry_lock() { # global lock for the whole select+register op
    local tries=0
    while ! mkdir "${QE_PORT_REGISTRY}.lock" 2>/dev/null; do
        # Reclaim if the owner died while holding it.
        if [ -f "${QE_PORT_REGISTRY}.lock/pid" ]; then
            local owner
            owner=$(cat "${QE_PORT_REGISTRY}.lock/pid" 2>/dev/null)
            [ -n "${owner}" ] && ! kill -0 "${owner}" 2>/dev/null && { rm -rf "${QE_PORT_REGISTRY}.lock"; continue; }
        fi
        tries=$((tries + 1)); [ "${tries}" -ge 25 ] && return 1
        sleep 0.2
    done
    echo "$$" >"${QE_PORT_REGISTRY}.lock/pid"
}

_port_registry_unlock() { rm -rf "${QE_PORT_REGISTRY}.lock"; }

# Is <port> already reserved by a live quickemu instance? Prunes stale rows.
_port_registry_taken() { # port -> 0 if reserved by a live vm
    [ -d "${QE_PORT_REGISTRY}" ] || return 1
    local row p vm pid
    for f in "${QE_PORT_REGISTRY}"/*; do
        [ -f "${f}" ] || continue
        while IFS=$'\t' read -r p vm pid _; do
            [ "${p}" = "${1}" ] || continue
            if kill -0 "${pid}" 2>/dev/null; then return 0; fi
            rm -f -- "${f}" 2>/dev/null || true   # stale reservation
            break
        done <"${f}"
    done
    return 1
}

_port_registry_reserve() { # port -> writes reservation for this VM
    mkdir -p "${QE_PORT_REGISTRY}" 2>/dev/null || return 1
    printf '%s\t%s\t%s\t%s\n' "${1}" "${VMNAME:-unknown}" "$$" "$(date +%s)" \
        >"${QE_PORT_REGISTRY}/${VMNAME:-unknown}.reserved"
}

# Drop this VM's reservations (called from vm_runtime_cleanup and on
# failed boots so a dead VM never holds a port hostage).
_port_registry_release() {
    rm -f -- "${QE_PORT_REGISTRY}/${VMNAME:-unknown}.reserved" 2>/dev/null || true
}

# ---- ports.sh ----
# Host TCP port allocation for guests and .ports runtime state.

function get_port() {
    local PORT_START=$1
    local PORT_RANGE=$((PORT_START + $2))
    local PORT
    # Select and register the port atomically: two quickemu instances must
    # not pick the same free port before QEMU has bound it.
    _port_registry_lock || {
        echo "ERROR! Could not acquire the port allocation lock." >&2
        return 1
    }
    for ((PORT = PORT_START; PORT <= PORT_RANGE; PORT++)); do
        _port_registry_taken "${PORT}" && continue
        # A connect to 127.0.0.1 always completes immediately (accepted or
        # refused), so no timeout wrapper is needed. A failed connect means
        # the port is free. This also works on macOS where GNU 'timeout' is
        # not installed.
        if ! (exec 3<>"/dev/tcp/127.0.0.1/${PORT}") 2>/dev/null; then
            _port_registry_reserve "${PORT}" 2>/dev/null || true
            _port_registry_unlock
            echo "${PORT}"
            return 0
        fi
    done
    _port_registry_unlock
    return 1
}

function parse_ports_from_file {
    local FILE="${VMDIR}/${VMNAME}.ports"
    local host_name=""
    local port_name=""
    local port_number=""

    # VMs created before .ports bookkeeping have no file; nothing to parse.
    [ -f "${FILE}" ] || return 0

    # Loop over each line in the file
    while IFS= read -r CONF || [ -n "${CONF}" ]; do
        # parse ports
        port_name=$(echo "${CONF}" | cut -d',' -f 1)
        port_number=$(echo "${CONF}" | cut -d',' -f 2)
        host_name=$(echo "${CONF}" | awk -F',' '{print $3,"."}')

        if [ "${port_name}" == "ssh" ]; then
            ssh_port="${port_number}"
        elif [ "${port_name}" == "spice" ]; then
            spice_port="${port_number}"
        elif [ "${port_name}" == "monitor-telnet" ]; then
            monitor_telnet_port="${port_number}"
            monitor_telnet_host="${host_name}"
        elif [ "${port_name}" == "serial-telnet" ]; then
            serial_telnet_port="${port_number}"
            serial_telnet_host="${host_name}"
        fi
    done <"${FILE}"
}

# Record an exposed host endpoint for this boot in the runtime .ports file.
# Skipped in plan mode (--dry-run): the file is execution state.
function ports_record() {
    [ -z "${DRY_RUN}" ] && echo "${1}" >>"${VMDIR}/${VMNAME}.ports"
    return 0
}

# Record where the SPICE viewer should connect (socket path or TCP port).
# Skipped in plan mode (--dry-run).
function spice_record() {
    [ -z "${DRY_RUN}" ] && echo "${1}" >"${VMDIR}/${VMNAME}.spice"
    return 0
}
