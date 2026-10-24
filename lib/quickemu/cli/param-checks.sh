# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Sourced partial: shares the conf-schema global namespace with bin/quickemu.
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
        if [ "${display}" != "gtk" ] && [ "${display}" != "none" ] && [ "${display}" != "sdl" ] && [ "${display}" != "spice" ] && [ "${display}" != "spice-app" ]; then
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

function ports_param_check() {
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
