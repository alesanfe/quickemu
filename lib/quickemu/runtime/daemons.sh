# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Sourced partial: shares the conf-schema global namespace with bin/quickemu.
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
