# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Sourced partial: shares the conf-schema global namespace with bin/quickemu.
# VM display aspect: maps the display= conf to QEMU video/display args.

function configure_display() {
    # Determine which audio driver to use: PipeWire, PulseAudio, or ALSA
    # Socket detection is more reliable than process detection on headless servers
    local AUDIO_DRIVER="alsa"
    local pw_socket="${PIPEWIRE_REMOTE:-${XDG_RUNTIME_DIR}/pipewire-0}"
    local pa_socket="${PULSE_SERVER:-${XDG_RUNTIME_DIR}/pulse/native}"
    # Check the audio backends this QEMU build actually supports; some builds
    # (e.g. Guix) lack PipeWire/PulseAudio even when the sockets exist.
    local qemu_audio_drivers=""
    qemu_audio_drivers=$("${QEMU}" -audio-help 2>/dev/null)

    if [ "${QEMU_VER_SHORT}" -ge 81 ] && [ -S "${pw_socket}" ] && grep -qw 'driver=pipewire' <<<"${qemu_audio_drivers}"; then
        # QEMU's pipewire audio backend was added in version 8.1
        AUDIO_DRIVER="pipewire"
    elif [ -S "${pa_socket}" ] && grep -qw 'driver=pa' <<<"${qemu_audio_drivers}"; then
        AUDIO_DRIVER="pa"
    fi

    # Setup the appropriate audio device based on the display output
    # https://www.kraxel.org/blog/2020/01/qemu-sound-audiodev/
    case ${display} in
    cocoa) AUDIO_DEV="coreaudio,id=audio0" ;;
    none | spice | spice-app) AUDIO_DEV="spice,id=audio0" ;;
    *) AUDIO_DEV="${AUDIO_DRIVER},id=audio0" ;;
    esac

    # Determine a sane resolution for Linux guests.
    local X_RES="1280"
    local Y_RES="800"
    if [ -n "${width}" ] && [ -n "${height}" ]; then
        local X_RES="${width}"
        local Y_RES="${height}"
    fi

    # https://www.kraxel.org/blog/2019/09/display-devices-in-qemu/
    case ${guest_os} in
    *bsd | templeos) DISPLAY_DEVICE="VGA" ;;
    linux_old | solaris) DISPLAY_DEVICE="vmware-svga" ;;
    linux)
        # ARM64 does not have VGA hardware - use virtio-gpu-pci instead of virtio-vga
        if [ "${ARCH_VM}" == "aarch64" ]; then
            DISPLAY_DEVICE="virtio-gpu-pci"
        else
            case ${display} in
            none | spice | spice-app) DISPLAY_DEVICE="virtio-gpu" ;;
            *) DISPLAY_DEVICE="virtio-vga" ;;
            esac
        fi
        ;;
    macos)
        # macOS has native VMware display driver support; aligns with OSX-KVM
        DISPLAY_DEVICE="vmware-svga"
        ;;
    windows | windows-server)
        # As ARM64 "virt" machines do not provide legacy VGA/QXL hardware,
        # use virtio-gpu-pci as this match the Linux ARM64 configuration.
        if [ "${ARCH_VM}" == "aarch64" ]; then
            DISPLAY_DEVICE="virtio-gpu-pci"
        else
            case ${display} in
            none | spice) DISPLAY_DEVICE="qxl-vga" ;;
            cocoa | gtk | sdl | spice-app) DISPLAY_DEVICE="virtio-vga" ;;
            esac
        fi
        ;;
    *) DISPLAY_DEVICE="qxl-vga" ;;
    esac

    # Map Quickemu $display to QEMU -display
    case ${display} in
    cocoa)
        # macOS: prefer OpenGL ES (via ANGLE) for stability and performance
        # ANGLE provides OpenGL ES on macOS through Metal, which is more stable
        # than the deprecated native OpenGL implementation
        # Reference: https://gist.github.com/akihikodaki/87df4149e7ca87f18dc56807ec5a1bc5
        if [ "${gl}" == "on" ] && check_cocoa_gl_es_support; then
            DISPLAY_RENDER="${display},gl=es"
            gl="es"
        else
            DISPLAY_RENDER="${display}"
            [ "${gl}" == "on" ] && gl="off"
        fi
        ;;
    gtk)
        if [ -n "${FULLSCREEN}" ]; then
            DISPLAY_RENDER="${display},grab-on-hover=on,zoom-to-fit=off,gl=${gl},full-screen=on"
        else
            DISPLAY_RENDER="${display},grab-on-hover=on,zoom-to-fit=off,gl=${gl}"
        fi
        ;;
    none)
        DISPLAY_RENDER="none"
        gl="off"
        ;; # No display backend means no GL context
    spice)
        # For local SPICE with GL, use egl-headless to provide the GL context
        # for VirGL. Don't use gl=on in SPICE itself as it blocks the main loop.
        if [ -z "${ACCESS}" ] || [ "${ACCESS}" == "local" ]; then
            if [ "${gl}" == "on" ]; then
                DISPLAY_RENDER="egl-headless,rendernode=/dev/dri/renderD128"
            else
                DISPLAY_RENDER="none"
            fi
        else
            # Remote access cannot use GL
            DISPLAY_RENDER="none"
            gl="off"
        fi
        ;;
    sdl) DISPLAY_RENDER="${display},gl=${gl}" ;;
    spice-app) DISPLAY_RENDER="${display},gl=${gl}" ;;
    vnc)
        DISPLAY_RENDER="none"
        gl="off"
        ;;
    *) DISPLAY_RENDER="${display}" ;;
    esac

    # https://www.kraxel.org/blog/2021/05/virtio-gpu-qemu-graphics-update/
    # For GL-enabled displays, check if dedicated GL device variants are available.
    # Note: virtio-gpu-pci becomes virtio-gpu-gl-pci (not virtio-gpu-pci-gl)
    if [ "${gl}" != "off" ] && [[ "${DISPLAY_DEVICE}" =~ ^virtio-(vga|gpu|gpu-pci)$ ]]; then
        local GL_DEVICE=""
        case "${DISPLAY_DEVICE}" in
        virtio-gpu-pci) GL_DEVICE="virtio-gpu-gl-pci" ;;
        virtio-gpu) GL_DEVICE="virtio-gpu-gl" ;;
        virtio-vga) GL_DEVICE="virtio-vga-gl" ;;
        esac

        if "${QEMU}" -device help 2>&1 | grep -q "\"${GL_DEVICE}\""; then
            DISPLAY_DEVICE="${GL_DEVICE}"
        fi
        echo -n " - Display:  ${display^^}, ${DISPLAY_DEVICE}, GL (${gl}), VirGL (on)"
    else
        echo -n " - Display:  ${display^^}, ${DISPLAY_DEVICE}, GL (${gl}), VirGL (off)"
    fi

    # Disable default VGA for SPICE modes to prevent duplicate scanouts
    # SPICE creates its own display output; the default VGA would create a second one
    case ${display} in
    none | spice | spice-app) VGA="-vga none" ;;
    *) VGA="" ;;
    esac

    # Build the video configuration
    VIDEO="${VGA:+${VGA} }-device ${DISPLAY_DEVICE}"

    # ARM64 needs ramfb for UEFI boot display only when the display device
    # has no firmware driver. Modern AAVMF/EDK2 drives virtio-gpu directly,
    # and the extra ramfb console just sits frozen in front of the real one
    # (e.g. a black 800x600 surface under cocoa).
    if [ "${ARCH_VM}" == "aarch64" ] && [[ ! "${DISPLAY_DEVICE}" =~ ^virtio-(gpu|gpu-pci|gpu-gl|gpu-gl-pci) ]]; then
        VIDEO="-device ramfb ${VIDEO}"
    fi

    # Set display resolution for devices that support xres/yres parameters
    # Use (,|$) anchor to match device names with or without comma-separated parameters
    # Pattern ordered most-specific to least-specific for clarity (vga-gl before vga, etc.)
    if [[ "${DISPLAY_DEVICE}" =~ ^(virtio-(vga|vga-gl|gpu|gpu-gl|gpu-pci|gpu-gl-pci)|qxl|qxl-vga|bochs-display)(,|$) ]]; then
        VIDEO="${VIDEO},xres=${X_RES},yres=${Y_RES}"
        echo " @ (${X_RES} x ${Y_RES})"
    else
        echo " "
    fi

    # Allocate VRAM to VGA devices
    # Note: virtio devices (virtio-vga, virtio-gpu-pci, and their -gl variants) use
    # dynamic memory management and don't require explicit VRAM allocation via parameters.
    # They use QEMU's default max_hostmem setting (256 MiB) which is sufficient for most use cases.
    case ${DISPLAY_DEVICE} in
    bochs-display) VIDEO="${VIDEO},vgamem=67108864" ;;
    qxl | qxl-vga) VIDEO="${VIDEO},ram_size=65536,vram_size=65536,vgamem_mb=64" ;;
    ati-vga | cirrus-vga | VGA | vmware-svga) VIDEO="${VIDEO},vgamem_mb=256" ;;
    esac

    # Configure multiscreen if max_outputs was provided in the .conf file
    if [ -n "${max_outputs}" ]; then
        VIDEO="${VIDEO},max_outputs=${max_outputs}"
    fi

    # Add fullscreen options
    VIDEO="${VIDEO} ${FULLSCREEN}"
}
