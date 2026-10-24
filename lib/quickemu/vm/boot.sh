# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Sourced partial: shares the conf-schema global namespace with bin/quickemu.
# VM launch orchestration: config -> argv -> spawn.

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
    # Set the hostname of the VM
    NET="user,hostname=${VMNAME}"

    echo "#!/usr/bin/env bash" >"${VMDIR}/${VMNAME}.sh"

    configure_cpu
    configure_ram
    check_macos_tsc_stability
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

    # $bridge backwards compatibility for Quickemu <= 4.0
    if [ -n "${bridge}" ]; then
        network="${bridge}"
    fi

    if [ "${network}" == "none" ]; then
        # Disable all networking
        echo " - Network:  Disabled"
        args+=(-nic none)
    elif [ "${network}" == "restrict" ]; then
        echo " - Network:  Restricted (${NET_DEVICE})"
        # shellcheck disable=SC2054,SC2206
        args+=(-device ${NET_DEVICE},netdev=nic -netdev ${NET},restrict=y,id=nic)
    elif [ "${network}" == "passt" ]; then
        # passt provides unprivileged user-mode networking; QEMU 10.1+
        echo " - Network:  passt (${NET_DEVICE})"
        if [ "${QEMU_VER_SHORT}" -lt 101 ]; then
            echo "ERROR! 'passt' networking requires QEMU 10.1 or newer."
            exit 1
        fi
        # shellcheck disable=SC2054,SC2206
        args+=(-device ${NET_DEVICE},netdev=nic -netdev passt,id=nic)
    elif [ -n "${network}" ]; then
        # Enable bridge mode networking
        echo " - Network:  Bridged (${network})"

        # If a persistent MAC address is provided, use it.
        local MAC=""
        if [ -n "${macaddr}" ]; then
            MAC=",mac=${macaddr}"
        fi

        local BRIDGE_HELPER=""
        BRIDGE_HELPER=$(type -p qemu-bridge-helper)
        # shellcheck disable=SC2054,SC2206
        args+=(-nic bridge,br=${network},helper=${BRIDGE_HELPER},model=${NET_DEVICE}${MAC})
    else
        echo " - Network:  User (${NET_DEVICE})"
        # shellcheck disable=SC2054,SC2206
        args+=(-device ${NET_DEVICE},netdev=nic -netdev ${NET},id=nic)
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
        args+=(-drive if=ide,index=0,media=disk,file="${disk_img}")

    elif [ "${guest_os}" == "templeos" ]; then
        # TempleOS supports ATA/IDE disks only
        # shellcheck disable=SC2054,SC2206
        args+=(-drive if=ide,index=0,media=disk,file="${disk_img}")

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

    if [ "${tpm}" == "on" ] && [ -S "${VMDIR}/${VMNAME}.swtpm-sock" ]; then
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
            echo "monitor-telnet,${monitor_telnet_port},${monitor_telnet_host}" >>"${VMDIR}/${VMNAME}.ports"
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
            echo "serial-telnet,${serial_telnet_port},${serial_telnet_host}" >>"${VMDIR}/${VMNAME}.ports"
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

    if [ -z "${VM_PID}" ]; then
        if [ -n "${DRY_RUN}" ]; then
            emit_execution_plan "${VMNAME}" "${QEMU} ${SHELL_ARGS}"
            exit 0
        fi
        echo "${QEMU}" "${SHELL_ARGS}" "2>/dev/null" >>"${VMDIR}/${VMNAME}.sh"
        sed -i -e 's/ -/ \\\n    -/g' "${VMDIR}/${VMNAME}.sh"
        ${QEMU} "${args[@]}" &>"${VMDIR}/${VMNAME}.log" &
        VM_PID=$!
        sleep 0.25
        if kill -0 "${VM_PID}" 2>/dev/null; then
            echo " - Process:  Started ${VM} as ${VMNAME} (${VM_PID})"
            configure_cpu_pinning
        else
            echo " - Process:  ERROR! Failed to start ${VM} as ${VMNAME}"
            rm -f "${VMDIR}/${VMNAME}.pid"
            rm -f "${VMDIR}/${VMNAME}.spice"
            rm -f "${VMDIR}/${VMNAME}.sock"
            stop_virtiofsd
            echo && cat "${VMDIR}/${VMNAME}.log"
            exit 1
        fi
    fi
}
