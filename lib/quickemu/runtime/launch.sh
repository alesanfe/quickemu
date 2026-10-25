# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Sourced partial: shares the conf-schema global namespace with bin/quickemu.
# QEMU process spawn: writes the .sh launcher, starts the guest and
# verifies the process stayed up, cleaning runtime state on failure.

function qemu_spawn() {
    echo "${QEMU}" "${SHELL_ARGS}" "2>/dev/null" >>"${VMDIR}/${VMNAME}.sh"
    sed -i -e 's/ -/ \\\n    -/g' "${VMDIR}/${VMNAME}.sh"
    "${QEMU}" "$@" &>"${VMDIR}/${VMNAME}.log" &
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
}

# Initialise the generated launch script for this boot.
# Skipped in plan mode (--dry-run): the .sh is an execution artifact.
function runtime_launcher_init() {
    [ -z "${DRY_RUN}" ] && echo "#!/usr/bin/env bash" >"${VMDIR}/${VMNAME}.sh"
}

# True when the swtpm control socket will be usable for this boot —
# either already created by the running daemon or planned (--dry-run).
function swtpm_socket_ready() {
    [ -n "${DRY_RUN}" ] || [ -S "${VMDIR}/${VMNAME}.swtpm-sock" ]
}

# QEMU command-line construction from VM config.

function configure_usb() {
    local DEVICE=""
    local USB_BUS=""
    local USB_DEV=""
    local USB_NAME=""
    local VENDOR_ID=""
    local PRODUCT_ID=""
    local USB_NOT_READY=0

    # Have any USB devices been requested for pass-through?
    if ((${#usb_devices[@]})); then
        echo " - USB:      Host pass-through requested:"
        for DEVICE in "${usb_devices[@]}"; do
            VENDOR_ID=$(echo "${DEVICE}" | cut -d':' -f1)
            PRODUCT_ID=$(echo "${DEVICE}" | cut -d':' -f2)
            USB_BUS=$(lsusb -d "${VENDOR_ID}:${PRODUCT_ID}" | cut -d' ' -f2)
            USB_DEV=$(lsusb -d "${VENDOR_ID}:${PRODUCT_ID}" | cut -d' ' -f4 | cut -d':' -f1)
            USB_NAME=$(lsusb -d "${VENDOR_ID}:${PRODUCT_ID}" | cut -d' ' -f7-)
            if [ -z "${USB_NAME}" ]; then
                echo "             ! USB device ${VENDOR_ID}:${PRODUCT_ID} not found. Check your configuration"
                continue
            elif [ -w "/dev/bus/usb/${USB_BUS}/${USB_DEV}" ]; then
                echo "             o ${USB_NAME} on bus ${USB_BUS} device ${USB_DEV} is accessible."
            else
                echo "             x ${USB_NAME} on bus ${USB_BUS} device ${USB_DEV} needs permission changes:"
                echo "               sudo chown -v ${USER}:root /dev/bus/usb/${USB_BUS}/${USB_DEV}"
                USB_NOT_READY=1
            fi
            USB_PASSTHROUGH="${USB_PASSTHROUGH} -device usb-host,bus=hostpass.0,vendorid=0x${VENDOR_ID},productid=0x${PRODUCT_ID}"
        done

        if [ "${USB_NOT_READY}" -eq 1 ]; then
            echo "               ERROR! USB permission changes are required 👆"
            exit 1
        fi
    fi
}

function configure_cpu_flag() {
    local HOST_CPU_FLAG="${1}"
    # Convert the flag to lowercase for QEMU
    local QEMU_CPU_FLAG=${HOST_CPU_FLAG,,}
    if check_cpu_flag "${HOST_CPU_FLAG}"; then
        # Replace _ with - to make it compatible with QEMU
        QEMU_CPU_FLAG="${HOST_CPU_FLAG//_/-}"
        QEMU_CPU_FLAG="${QEMU_CPU_FLAG//4-/4\.}"
        # macOS uses different flag names
        if [ "${OS_KERNEL}" == "Darwin" ]; then
            case "${HOST_CPU_FLAG}" in
            avx) QEMU_CPU_FLAG="AVX1.0" ;;
            esac
        fi
        echo ",+${QEMU_CPU_FLAG}"
    else
        # Fully disable any QEMU flags that are not supported by the host CPU
        if [ "${HOST_CPU_VENDOR}" == "AuthenticAMD" ]; then
            case ${HOST_CPU_FLAG} in
            pcid) echo ",-${QEMU_CPU_FLAG}" ;;
            esac
        fi
    fi
}

function reset_cpu_flags() {
    CPU_FLAG_MAP=()
}

function add_cpu_flag() {
    local flag="${1#,}" # Strip leading comma if present

    # Skip empty flags
    [[ -z "${flag}" ]] && return 0

    # Validate flag format: must be [+-]?name or name=value
    if [[ ! "${flag}" =~ ^[+-]?[a-zA-Z][a-zA-Z0-9._-]*(=.+)?$ ]]; then
        echo "WARNING: Invalid CPU flag format: '${flag}' - skipping"
        return 1
    fi

    # Extract base flag name (without +/- prefix and =value suffix)
    local prefix=""
    local base="${flag}"
    if [[ "${flag}" == [+-]* ]]; then
        prefix="${flag:0:1}"
        base="${flag:1}"
    fi
    base="${base%%=*}"

    # Check for exact duplicate
    if [[ -n "${CPU_FLAG_MAP[${flag}]:-}" ]]; then
        return 0 # Silently skip duplicates
    fi

    # Check for conflicts (opposite prefix)
    local opposite=""
    if [[ "${prefix}" == "+" ]]; then
        opposite="-${base}"
    elif [[ "${prefix}" == "-" ]]; then
        opposite="+${base}"
    fi

    if [[ -n "${opposite}" ]] && [[ -n "${CPU_FLAG_MAP[${opposite}]:-}" ]]; then
        echo "WARNING: Conflicting CPU flag '${flag}' ignored (${opposite} already set)"
        return 1
    fi

    # Check for value conflicts (e.g., flag=on vs flag=off)
    if [[ "${flag}" == *=* ]]; then
        for existing in "${!CPU_FLAG_MAP[@]}"; do
            if [[ "${existing%%=*}" == "${base}" ]] && [[ "${existing}" == *=* ]]; then
                echo "WARNING: Conflicting CPU flag '${flag}' ignored (${existing} already set)"
                return 1
            fi
        done
    fi

    # Add to tracking map and append to CPU string
    CPU_FLAG_MAP["${flag}"]=1
    CPU+=",${flag}"
    return 0
}

function configure_cpu() {
    HOST_CPU_CORES=$(get_nproc)
    HOST_CPU_MODEL=$(get_cpu_info '^Model name:')
    HOST_CPU_SOCKETS=$(get_cpu_info 'Socket')
    if [ "${OS_KERNEL}" == "Darwin" ]; then
        HOST_CPU_VENDOR=$(get_cpu_info 'Vendor')
    else
        HOST_CPU_VENDOR=$(get_cpu_info '^Vendor ID')
    fi

    if [ "${HOST_CPU_SOCKETS}" = "-" ]; then
        HOST_CPU_SOCKETS=1
    fi

    CPU_MODEL="host"
    QEMU_ACCEL="tcg"
    # Configure appropriately for the host platform
    if [ "${OS_KERNEL}" == "Darwin" ]; then
        MANUFACTURER=$(ioreg -l | grep -e Manufacturer | grep -v iMan | cut -d'"' -f4 | sort -u)
        CPU_KVM_UNHALT=""
        QEMU_ACCEL="hvf"
        # QEMU for macOS from Homebrew does not support SMM
        SMM="off"
    else
        if [ -r /sys/class/dmi/id/sys_vendor ]; then
            MANUFACTURER=$(head -n 1 /sys/class/dmi/id/sys_vendor)
        fi
        CPU_KVM_UNHALT=",kvm_pv_unhalt"
        GUEST_TWEAKS+=" -global kvm-pit.lost_tick_policy=discard"
        QEMU_ACCEL="kvm"
    fi

    if [ "${ARCH_VM}" == "aarch64" ]; then
        # ARM64 guest support
        # https://qemu-project.gitlab.io/qemu/system/arm/virt.html
        # highmem=on allows aarch64 guests on the "virt" machine type to access guest RAM
        # above the 4GB boundary. This is required for VMs configured with >3GB RAM and is
        # generally more compatible on modern aarch64 systems.
        MACHINE_TYPE="virt,highmem=on"
        case ${ARCH_HOST} in
        arm64 | aarch64)
            # Native ARM64 host running ARM64 guest - use hardware acceleration
            CPU_MODEL="max"
            ;;
        *)
            # Cross-architecture emulation (e.g., x86_64 host running ARM64 guest)
            CPU_MODEL="max"
            QEMU_ACCEL="tcg"
            ;;
        esac
    elif [ "${ARCH_VM}" != "${ARCH_HOST}" ]; then
        # If the architecture of the VM is different from the host, disable acceleration
        # and use TCG (Tiny Code Generator) software emulation. TCG emulates the target
        # architecture in software, allowing cross-architecture virtualisation (e.g.,
        # running x86_64 VMs on ARM hosts).
        #
        # Users can manually force TCG mode by adding this to their VM .conf file:
        #   cpu_model="qemu64"
        #   extra_args="-accel tcg"
        # or by exporting QEMU_ACCEL in the shell environment:
        #   export QEMU_ACCEL="tcg"
        #
        # TCG is useful for:
        # - Cross-architecture virtualisation (x86 on ARM, ARM on x86)
        # - Testing VMs on hosts without hardware virtualisation support
        # - Emulating CPU features in software (e.g., running macOS x86_64 VMs on ARM Macs)
        CPU_MODEL="qemu64"
        CPU_KVM_UNHALT=""
        QEMU_ACCEL="tcg"
    fi

    # Detect if running inside a VM based on manufacturer detection
    # Note: Checking CPU flags (vmx/svm) indicates hardware virtualisation SUPPORT,
    # not whether we're inside a VM. Nested virtualisation may expose these flags.
    case ${MANUFACTURER,,} in
    qemu | virtualbox)
        CPU_MODEL="qemu64"
        QEMU_ACCEL="tcg"
        HYPERVISOR="${MANUFACTURER,,}"
        ;;
    *) HYPERVISOR="" ;;
    esac

    if [ -z "${HYPERVISOR}" ]; then
        # A CPU with Intel VT-x / AMD SVM support is required
        if [ "${HOST_CPU_VENDOR}" == "GenuineIntel" ]; then
            if ! check_cpu_flag vmx; then
                echo "ERROR! Intel VT-x support is required."
                exit 1
            fi
        elif [ "${HOST_CPU_VENDOR}" == "AuthenticAMD" ] || [ "${HOST_CPU_VENDOR}" == "HygonGenuine" ]; then
            # HygonGenuine is Chinese AMD-compatible CPUs (Hygon Dhyana)
            if ! check_cpu_flag svm; then
                echo "ERROR! AMD SVM support is required."
                exit 1
            fi
        elif [ "${ARCH_HOST}" == "aarch64" ] || [ "${ARCH_HOST}" == "arm64" ]; then
            # ARM hosts running native ARM guests with hardware acceleration (HVF on macOS, KVM on Linux)
            # ARM processors don't have x86-specific virtualisation flags (VT-x/SVM) to check.
            # We use architecture detection here instead of vendor detection (used for x86)
            # because ARM CPUs don't have standardised vendor strings like x86 (GenuineIntel/AuthenticAMD).
            # Cross-architecture guests (x86 on ARM) use TCG and skip this validation block entirely.
            # No validation needed here - ARM virtualisation support is handled by the hypervisor
            true
        else
            # Unknown CPU vendor - could be future/custom CPUs
            echo "WARNING! Unknown CPU vendor '${HOST_CPU_VENDOR}' - cannot verify virtualisation support."
            echo "         If virtualisation fails, check your CPU supports hardware virtualisation and it's enabled in firmware."
        fi
    fi

    CPU="-cpu ${CPU_MODEL}"
    reset_cpu_flags

    # Make any OS specific adjustments
    if [ "${guest_os}" == "freedos" ] || [ "${guest_os}" == "windows" ] || [ "${guest_os}" == "windows-server" ]; then
        # SMM is not available on QEMU for macOS via Homebrew
        if [ "${OS_KERNEL}" == "Linux" ]; then
            SMM="on"
        fi
    fi

    # SMM is also required for Linux guests when Secure Boot is enabled
    if [ "${secureboot}" == "on" ]; then
        if [ "${guest_os}" == "linux" ]; then
            # SMM is not available on QEMU for macOS via Homebrew
            if [ "${OS_KERNEL}" == "Linux" ]; then
                SMM="on"
            fi
        fi
    fi

    case ${guest_os} in
    batocera | freedos | haiku | solaris) MACHINE_TYPE="pc" ;;
    kolibrios | reactos)
        CPU="-cpu qemu32"
        reset_cpu_flags
        MACHINE_TYPE="pc"
        ;;
    macos)
        # If the host has an Intel CPU, passes the host CPU model features, model, stepping, exactly to the guest.
        # Disable huge pages (,-pdpe1gb) on macOS to prevent crashes
        # - https://stackoverflow.com/questions/60231203/qemu-qcow2-mmu-gva-to-gpa-crash-in-mac-os-x
        # vmware-cpuid-freq=on enables VMware TSC frequency reporting for macOS timing
        if [ "${HOST_CPU_VENDOR}" == "GenuineIntel" ] && [ -z "${HYPERVISOR}" ]; then
            CPU_MODEL="host"
            CPU="-cpu ${CPU_MODEL},-pdpe1gb,+hypervisor,vmware-cpuid-freq=on"
            reset_cpu_flags
        else
            CPU_MODEL="Haswell-v2"
            CPU="-cpu ${CPU_MODEL},vendor=GenuineIntel,-pdpe1gb,+avx,+sse,+sse2,+ssse3,vmware-cpuid-freq=on"
            reset_cpu_flags
        fi
        # A CPU with fma is required for Metal support
        # A CPU with invtsc is required for macOS to boot
        # Skip CPU feature checks when using TCG emulation (cross-architecture)
        # as TCG will emulate the required x86 features in software. This enables
        # running macOS x86_64 VMs on ARM Macs through software emulation.
        if [ "${QEMU_ACCEL}" != "tcg" ]; then
            case ${macos_release} in
            ventura | sonoma | sequoia | tahoe)
                # A CPU with AVX2 support is required for >= macOS Ventura
                if check_cpu_flag sse4_2 && check_cpu_flag avx2; then
                    if [ "${HOST_CPU_VENDOR}" != "GenuineIntel" ] && [ -z "${HYPERVISOR}" ]; then
                        add_cpu_flag "+avx2"
                        add_cpu_flag "+sse4.2"
                    fi
                else
                    echo "ERROR! macOS ${macos_release} requires a CPU with SSE 4.2 and AVX2 support."
                    echo "       Try macOS Monterey or Big Sur."
                    exit 1
                fi
                ;;
            catalina | big-sur | monterey)
                # A CPU with SSE4.2 support is required for >= macOS Catalina
                if check_cpu_flag sse4_2; then
                    if [ "${HOST_CPU_VENDOR}" != "GenuineIntel" ] && [ -z "${HYPERVISOR}" ]; then
                        add_cpu_flag "+sse4.2"
                    fi
                else
                    echo "ERROR! macOS ${macos_release} requires a CPU with SSE 4.2 support."
                    exit 1
                fi
                ;;
            *)
                # A CPU with SSE4.1 support is required for >= macOS Sierra
                if check_cpu_flag sse4_1; then
                    if [ "${HOST_CPU_VENDOR}" != "GenuineIntel" ] && [ -z "${HYPERVISOR}" ]; then
                        add_cpu_flag "+sse4.1"
                    fi
                else
                    echo "ERROR! macOS ${macos_release} requires a CPU with SSE 4.1 support."
                    exit 1
                fi
                ;;
            esac

            if [ "${HOST_CPU_VENDOR}" != "GenuineIntel" ] && [ -z "${HYPERVISOR}" ]; then
                for FLAG in abm adx aes amd-ssbd apic arat bmi1 bmi2 clflush cmov cx8 cx16 de \
                    eist erms f16c fma fp87 fsgsbase fxsr invpcid invtsc lahf_lm lm \
                    mca mce mmx movbe mpx msr mtrr nx pae pat pcid pge pse popcnt pse36 \
                    rdrand rdtscp sep smep syscall tsc tsc_adjust vaes vbmi2 vmx vpclmulqdq \
                    x2apic xgetbv1 xsave xsaveopt; do
                    local cpu_flag
                    cpu_flag=$(configure_cpu_flag "${FLAG}")
                    [[ -n "${cpu_flag}" ]] && add_cpu_flag "${cpu_flag#,}"
                done
                # AMD CPUs with constant_tsc need explicit TSC flags for macOS stability
                # constant_tsc is AMD's equivalent of Intel's invtsc
                if [ "${HOST_CPU_VENDOR}" == "AuthenticAMD" ] && check_cpu_flag invtsc; then
                    add_cpu_flag "+tsc"
                    add_cpu_flag "+tsc-deadline"
                fi
            fi
        fi

        # Disable S3 support in the VM to prevent macOS suspending during install
        # Disable ACPI PCI hotplug to prevent issues with macOS (required for QEMU 6.1+)
        GUEST_TWEAKS+=" -global ICH9-LPC.disable_s3=1 -global ICH9-LPC.acpi-pci-hotplug-with-bridge-support=off -device isa-applesmc,osk=$(echo "bheuneqjbexolgurfrjbeqfthneqrqcyrnfrqbagfgrny(p)NccyrPbzchgreVap" | tr 'A-Za-z' 'N-ZA-Mn-za-m')"

        # Disable High Precision Timer
        if [ "${QEMU_VER_SHORT}" -ge 70 ]; then
            MACHINE_TYPE+=",hpet=off"
        else
            GUEST_TWEAKS+=" -no-hpet"
        fi
        ;;
    windows | windows-server)
        # x86/x86_64 Windows guests require additional CPU feature flags.
        # ARM64 guests use the generic CPU configuration established earlier.
        if [ "${ARCH_VM}" == "x86_64" ]; then
            CPU="-cpu ${CPU_MODEL},+hypervisor,+invtsc,l3-cache=on"
            reset_cpu_flags
        fi
        # KVM-specific flags: migratable and Hyper-V enlightenments
        if [ "${ARCH_VM}" == "x86_64" ] && [ "${QEMU_ACCEL}" == "kvm" ]; then
            if [ "${QEMU_VER_SHORT}" -gt 60 ]; then
                add_cpu_flag "migratable=no"
                add_cpu_flag "hv_passthrough"
            else
                add_cpu_flag "migratable=no"
                add_cpu_flag "hv_frequencies"
                [[ -n "${CPU_KVM_UNHALT}" ]] && add_cpu_flag "${CPU_KVM_UNHALT#,}"
                add_cpu_flag "hv_reenlightenment"
                add_cpu_flag "hv_relaxed"
                add_cpu_flag "hv_spinlocks=8191"
                add_cpu_flag "hv_stimer"
                add_cpu_flag "hv_synic"
                add_cpu_flag "hv_time"
                add_cpu_flag "hv_vapic"
                add_cpu_flag "hv_vendor_id=1234567890ab"
                add_cpu_flag "hv_vpindex"
            fi
        fi
        # Disable S3 support in the VM to ensure Windows can boot with SecureBoot enabled
        #  - https://wiki.archlinux.org/title/QEMU/Troubleshooting#Virtual_machine_not_booting_when_using_a_Secure_Boot_enabled_OVMF
        GUEST_TWEAKS+=" -global ICH9-LPC.disable_s3=1"

        # Disable High Precision Timer
        # ARM64 "virt" machines do not implement this property
        # as HPET is only available on PC (x86/x86_64) machine types.
        if [ "${ARCH_VM}" == "x86_64" ]; then
            if [ "${QEMU_VER_SHORT}" -ge 70 ]; then
                MACHINE_TYPE+=",hpet=off"
            else
                GUEST_TWEAKS+=" -no-hpet"
            fi
        fi
        ;;
    esac

    if [ "${HOST_CPU_VENDOR}" == "AuthenticAMD" ] && [ "${guest_os}" != "macos" ] && [ "${ARCH_VM}" == "x86_64" ]; then
        add_cpu_flag "topoext"
    fi

    if [ -z "${cpu_cores}" ]; then
        if [ "${HOST_CPU_CORES}" -ge 32 ]; then
            GUEST_CPU_CORES="16"
        elif [ "${HOST_CPU_CORES}" -ge 16 ]; then
            GUEST_CPU_CORES="8"
        elif [ "${HOST_CPU_CORES}" -ge 8 ]; then
            GUEST_CPU_CORES="4"
        elif [ "${HOST_CPU_CORES}" -ge 4 ]; then
            GUEST_CPU_CORES="2"
        else
            GUEST_CPU_CORES="1"
        fi
    else
        GUEST_CPU_CORES="${cpu_cores}"
    fi

    # Windows 11 requires a minimum of 2 CPU cores to install
    # https://github.com/quickemu-project/quickemu/issues/1423
    if [ "${guest_os}" == "windows" ] && [[ "${VMNAME}" == *"windows-11"* || "${VMNAME}" == *"win11"* ]]; then
        if [ "${GUEST_CPU_CORES}" -lt 2 ]; then
            echo " - CPU:      Adjusting CPU cores from ${GUEST_CPU_CORES} to 2 (Windows 11 minimum requirement)"
            GUEST_CPU_CORES="2"
        fi
    fi

    # macOS guests cannot boot with most core counts not powers of 2.
    # Find the nearest but lowest power of 2 using a predefined table
    if [ "${guest_os}" == "macos" ]; then
        local POWERS=(1 2 4 8 16 32 64 128 256 512 1024)
        for ((i = ${#POWERS[@]} - 1; i >= 0; i--)); do
            if [ "${POWERS[i]}" -le "${GUEST_CPU_CORES}" ]; then
                GUEST_CPU_CORES="${POWERS[i]}"
                break
            fi
        done
    fi

    if [ "${OS_KERNEL}" == "Darwin" ]; then
        # Get the number of physical cores
        physicalcpu=$(sysctl -n hw.physicalcpu)
        # Get the number of logical processors
        logicalcpu=$(sysctl -n hw.logicalcpu)
        # Check if Hyper-Threading is enabled
        if [ "${logicalcpu}" -gt "${physicalcpu}" ]; then
            HOST_CPU_SMT="on"
        else
            HOST_CPU_SMT="off"
        fi
    elif [ -e /sys/devices/system/cpu/smt/control ]; then
        HOST_CPU_SMT=$(cat /sys/devices/system/cpu/smt/control)
    fi

    # Account for Hyperthreading/SMT.
    if [ "${GUEST_CPU_CORES}" -ge 2 ]; then
        case ${HOST_CPU_SMT} in
        on)
            GUEST_CPU_THREADS=2
            GUEST_CPU_LOGICAL_CORES=$((GUEST_CPU_CORES / GUEST_CPU_THREADS))
            ;;
        *)
            GUEST_CPU_THREADS=1
            GUEST_CPU_LOGICAL_CORES=${GUEST_CPU_CORES}
            ;;
        esac
    else
        GUEST_CPU_THREADS=1
        GUEST_CPU_LOGICAL_CORES=${GUEST_CPU_CORES}
    fi

    SMP="-smp cores=${GUEST_CPU_LOGICAL_CORES},threads=${GUEST_CPU_THREADS},sockets=${HOST_CPU_SOCKETS}"
    echo " - CPU:      ${HOST_CPU_MODEL}"
    echo " - CPU VM:   ${CPU_MODEL%%,*}, ${HOST_CPU_SOCKETS} Socket(s), ${GUEST_CPU_LOGICAL_CORES} Core(s), ${GUEST_CPU_THREADS} Thread(s)"

    if [ "${guest_os}" == "macos" ] || [ "${guest_os}" == "windows" ] || [ "${guest_os}" == "windows-server" ]; then
        # Display MSRs alert if the guest is macOS or windows
        ignore_msrs_alert
    fi
}

function configure_ram() {
    local OS_PRETTY_NAME=""
    RAM_VM="2G"
    if [ -z "${ram}" ]; then
        # Host RAM in GiB (meminfo on Linux, hw.memsize on macOS).
        local RAM_HOST=$(( $(host_total_mem_mb) / 1024 ))

        if [ "${RAM_HOST}" -ge 128 ]; then
            RAM_VM="32G"
        elif [ "${RAM_HOST}" -ge 64 ]; then
            RAM_VM="16G"
        elif [ "${RAM_HOST}" -ge 16 ]; then
            RAM_VM="8G"
        elif [ "${RAM_HOST}" -ge 8 ]; then
            RAM_VM="4G"
        fi
    else
        RAM_VM="${ram}"
    fi
    echo " - RAM VM:   ${RAM_VM} RAM"

    case "${guest_os}" in
    windows | windows-server)
        OS_PRETTY_NAME="Windows"
        min_ram="4"
        ;;
    macos)
        OS_PRETTY_NAME="macOS"
        min_ram="8"
        ;;
    esac

    if [ -n "${min_ram}" ] && [ "${RAM_VM//G/}" -lt "${min_ram}" ]; then
        if [ -z "${ram}" ]; then
            echo "             ERROR! The guest virtual machine has been allocated insufficient RAM to run ${OS_PRETTY_NAME}."
            echo "             You can override the guest RAM allocation by adding 'ram=${min_ram}G' to ${VM}"
            exit 1
        else
            echo "             WARNING! You have allocated less than the recommended amount of RAM to run ${OS_PRETTY_NAME}."
        fi
    fi
}

function configure_bios() {
    # Always Boot macOS using EFI
    if [ "${guest_os}" == "macos" ]; then
        boot="efi"
        if [ -e "${VMDIR}/OVMF_CODE.fd" ] && [ -e "${VMDIR}/OVMF_VARS-1024x768.fd" ]; then
            EFI_CODE="${VMDIR}/OVMF_CODE.fd"
            EFI_VARS="${VMDIR}/OVMF_VARS-1024x768.fd"
        elif [ -e "${VMDIR}/OVMF_CODE.fd" ] && [ -e "${VMDIR}/OVMF_VARS-1920x1080.fd" ]; then
            EFI_CODE="${VMDIR}/OVMF_CODE.fd"
            EFI_VARS="${VMDIR}/OVMF_VARS-1920x1080.fd"
        else
            MAC_MISSING="Firmware"
        fi

        # Check for OpenCore bootloader
        if [ -e "${VMDIR}/OpenCore.qcow2" ]; then
            MAC_BOOTLOADER="${VMDIR}/OpenCore.qcow2"
        elif [ -e "${VMDIR}/ESP.qcow2" ]; then
            # Backwards compatibility for Clover
            MAC_BOOTLOADER="${VMDIR}/ESP.qcow2"
        else
            MAC_MISSING="Bootloader"
        fi

        if [ -n "${MAC_MISSING}" ]; then
            echo "ERROR! macOS ${MAC_MISSING} was not found."
            echo "       Use 'quickget' to download the required files."
            exit 1
        fi

        # Truncated or corrupt firmware downloads produce unusable pflash
        # devices (e.g. "system firmware block device pflash0 has invalid
        # size"). OVMF_CODE.fd is ~2M and the OVMF_VARS file at least 128K.
        if [ "$(wc -c <"${EFI_CODE}")" -lt 1048576 ] || [ "$(wc -c <"${EFI_VARS}")" -lt 131072 ]; then
            echo "ERROR! macOS firmware in ${VMDIR} appears to be corrupt or incomplete."
            echo "       Delete OVMF_CODE.fd and OVMF_VARS*.fd, then re-run quickget."
            exit 1
        fi
        BOOT_STATUS="EFI (macOS), OVMF ($(basename "${EFI_CODE}")), SecureBoot (${secureboot})."
    elif [[ "${boot}" == *"efi"* ]]; then
        EFI_VARS="${VMDIR}/OVMF_VARS.fd"

        # Preserve backward compatibility
        if [ -z "${DRY_RUN}" ]; then
            if [ -e "${VMDIR}/${VMNAME}-vars.fd" ]; then
                mv "${VMDIR}/${VMNAME}-vars.fd" "${EFI_VARS}"
            elif [ -e "${VMDIR}/OVMF_VARS_4M.fd" ]; then
                mv "${VMDIR}/OVMF_VARS_4M.fd" "${EFI_VARS}"
            fi
        fi

        # OVMF_CODE_4M.fd is for booting guests in non-Secure Boot mode.
        # While this image technically supports Secure Boot, it does so
        # without requiring SMM support from QEMU

        # OVMF_CODE.secboot.fd is like OVMF_CODE_4M.fd, but will abort if QEMU
        # does not support SMM.

        local SHARE_PATH
        SHARE_PATH=$(get_qemu_share_path)

        # https://bugzilla.redhat.com/show_bug.cgi?id=1929357#c5
        # TODO: Check if macOS should use 'edk2-i386-vars.fd'
        # Search for firmware if EFI_CODE is not set or the specified file doesn't exist
        if [ -z "${EFI_CODE}" ] || [ ! -e "${EFI_CODE}" ]; then
            if [ "${ARCH_VM}" == "aarch64" ]; then
                # Prefer Fedora's monolithic EDK2 firmware for Windows ARM64 guests.
                # This firmware is passed with -bios and does not use persistent
                # pflash variable storage.
                if [ "${guest_os}" == "windows" ] &&
                    [ "${secureboot}" != "on" ] &&
                    [ -e "${SHARE_PATH}/edk2/aarch64/QEMU_EFI.fd" ]; then
                    EFI_CODE="${SHARE_PATH}/edk2/aarch64/QEMU_EFI.fd"
                    EFI_MODE="bios"
                else
                    # AAVMF firmware paths for ARM64 guests using persistent pflash.
                    # shellcheck disable=SC2054,SC2140
                    ovmfs=("/usr/share/AAVMF/AAVMF_CODE.fd","/usr/share/AAVMF/AAVMF_VARS.fd"
                        "${SHARE_PATH}/edk2/aarch64/QEMU_CODE.fd","${SHARE_PATH}/edk2/aarch64/QEMU_VARS.fd"
                        "${SHARE_PATH}/edk2/aarch64/QEMU_EFI-pflash.raw","${SHARE_PATH}/edk2/aarch64/vars-template-pflash.raw"
                        "${SHARE_PATH}/qemu/edk2-aarch64-code.fd","${SHARE_PATH}/qemu/edk2-arm-vars.fd"
                        "${SHARE_PATH}/AAVMF/AAVMF_CODE.fd","${SHARE_PATH}/AAVMF/AAVMF_VARS.fd"
                    )
                fi
            else
                # x86_64 OVMF firmware paths
                case ${secureboot} in
                on) # shellcheck disable=SC2054,SC2140
                    ovmfs=("${SHARE_PATH}/OVMF/OVMF_CODE_4M.secboot.fd","${SHARE_PATH}/OVMF/OVMF_VARS_4M.ms.fd"
                        "${SHARE_PATH}/edk2/ovmf/OVMF_CODE.secboot.fd","${SHARE_PATH}/edk2/ovmf/OVMF_VARS.secboot.fd"
                        "${SHARE_PATH}/OVMF/x64/OVMF_CODE.secboot.fd","${SHARE_PATH}/OVMF/x64/OVMF_VARS.fd"
                        "${SHARE_PATH}/edk2-ovmf/OVMF_CODE.secboot.fd","${SHARE_PATH}/edk2-ovmf/OVMF_VARS.fd"
                        "${SHARE_PATH}/qemu/ovmf-x86_64-smm-ms-code.bin","${SHARE_PATH}/qemu/ovmf-x86_64-smm-ms-vars.bin"
                        "${SHARE_PATH}/qemu/edk2-x86_64-secure-code.fd","${SHARE_PATH}/qemu/edk2-i386-vars.fd"
                        "${SHARE_PATH}/edk2-ovmf/x64/OVMF_CODE.secboot.fd","${SHARE_PATH}/edk2-ovmf/x64/OVMF_VARS.fd"
                        "${SHARE_PATH}/edk2/x64/OVMF_CODE.secboot.4m.fd","${SHARE_PATH}/edk2/x64/OVMF_VARS.4m.fd"
                        "${SHARE_PATH}/edk2/ovmf/OVMF_CODE_4M.secboot.qcow2","${SHARE_PATH}/edk2/ovmf/OVMF_VARS_4M.secboot.qcow2"
                    ) ;;
                *) # shellcheck disable=SC2054,SC2140
                    ovmfs=("${SHARE_PATH}/OVMF/OVMF_CODE_4M.fd","${SHARE_PATH}/OVMF/OVMF_VARS_4M.fd"
                        "${SHARE_PATH}/edk2/ovmf/OVMF_CODE.fd","${SHARE_PATH}/edk2/ovmf/OVMF_VARS.fd"
                        "${SHARE_PATH}/OVMF/OVMF_CODE.fd","${SHARE_PATH}/OVMF/OVMF_VARS.fd"
                        "${SHARE_PATH}/OVMF/x64/OVMF_CODE.fd","${SHARE_PATH}/OVMF/x64/OVMF_VARS.fd"
                        "${SHARE_PATH}/edk2-ovmf/OVMF_CODE.fd","${SHARE_PATH}/edk2-ovmf/OVMF_VARS.fd"
                        "${SHARE_PATH}/qemu/ovmf-x86_64-4m-code.bin","${SHARE_PATH}/qemu/ovmf-x86_64-4m-vars.bin"
                        "${SHARE_PATH}/qemu/edk2-x86_64-code.fd","${SHARE_PATH}/qemu/edk2-i386-vars.fd"
                        "${SHARE_PATH}/edk2-ovmf/x64/OVMF_CODE.fd","${SHARE_PATH}/edk2-ovmf/x64/OVMF_VARS.fd"
                        "${SHARE_PATH}/edk2/x64/OVMF_CODE.4m.fd","${SHARE_PATH}/edk2/x64/OVMF_VARS.4m.fd"
                        "${SHARE_PATH}/edk2/ovmf/OVMF_CODE_4M.qcow2","${SHARE_PATH}/edk2/ovmf/OVMF_VARS_4M.qcow2"
                    ) ;;
                esac
            fi

            if [ "${EFI_MODE}" == "pflash" ]; then
                # Select the first complete firmware pair, preserving the order of preference above.
                _IFS=$IFS
                IFS=","
                for f in "${ovmfs[@]}"; do
                    # shellcheck disable=SC2086
                    set -- ${f}
                    if [ -e "${1}" ] && [ -e "${2}" ]; then
                        EFI_CODE="${1}"
                        EFI_EXTRA_VARS="${2}"
                        break
                    fi
                done
                IFS=$_IFS
            fi
        fi
        if [ -z "${EFI_CODE}" ] || [ ! -e "${EFI_CODE}" ]; then
            if [ "${secureboot}" == "on" ]; then
                echo "ERROR! SecureBoot was requested but no SecureBoot capable firmware was found."
            else
                echo "ERROR! EFI boot requested but no EFI firmware found."
            fi
            echo "       Please install OVMF firmware."
            exit 1
        fi
        if [ -n "${EFI_EXTRA_VARS}" ]; then
            if [ ! -e "${EFI_EXTRA_VARS}" ]; then
                echo " - EFI:      ERROR! EFI_EXTRA_VARS file ${EFI_EXTRA_VARS} does not exist."
                exit 1
            fi
            # Write destination vars file with correct extension
            # based on source format of the EFI_EXTRA_VARS file
            QCOW2VARS=$(is_firmware_qcow2 "${EFI_EXTRA_VARS}")
            if [ "${QCOW2VARS}" = "true" ]; then
                EFI_VARS="${VMDIR}/OVMF_VARS.qcow2"
            else
                EFI_VARS="${VMDIR}/OVMF_VARS.fd"
            fi
            [ -z "${DRY_RUN}" ] && efi_vars "${EFI_EXTRA_VARS}" "${EFI_VARS}"
        fi

        # Make sure EFI_VARS references an actual, writeable, file
        if [ "${EFI_MODE}" == "pflash" ] && [ -z "${DRY_RUN}" ] &&
            { [ ! -f "${EFI_VARS}" ] || [ ! -w "${EFI_VARS}" ]; }; then
            echo " - EFI:      ERROR! ${EFI_VARS} is not a regular file or not writeable."
            echo "             Deleting ${EFI_VARS}. Please re-run quickemu."
            qe_safe_rm "${EFI_VARS}"
            exit 1
        fi

        # If EFI_CODE references a symlink, resolve it to the real file.
        if [ -L "${EFI_CODE}" ]; then
            echo " - EFI:      WARNING! ${EFI_CODE} is a symlink."
            echo -n "             Resolving to... "
            EFI_CODE=$(realpath "${EFI_CODE}")
            echo "${EFI_CODE}"
        fi
        if [ "${EFI_MODE}" == "bios" ]; then
            BOOT_STATUS="EFI (${guest_os^}), firmware (${EFI_CODE}), mode (-bios), SecureBoot (off)."
        else
            BOOT_STATUS="EFI (${guest_os^}), OVMF (${EFI_CODE}), EFI Vars (${EFI_VARS}), SecureBoot (${secureboot})."
        fi
    else
        BOOT_STATUS="Legacy BIOS (${guest_os^})"
        boot="legacy"
        secureboot="off"
        # Legacy BIOS boot requires the i440fx/PIIX3 chipset (pc), not Q35
        MACHINE_TYPE="pc"
    fi

    echo " - BOOT:     ${BOOT_STATUS}"
}

function configure_os_quirks() {

    if [ "${guest_os}" == "batocera" ] || [ "${guest_os}" == "haiku" ] || [ "${guest_os}" == "kolibrios" ]; then
        NET_DEVICE="rtl8139"
    fi

    if [ "${guest_os}" == "freebsd" ] || [ "${guest_os}" == "ghostbsd" ]; then
        mouse="usb"
    fi

    case ${guest_os} in
    windows-server) NET_DEVICE="e1000" ;;
    *bsd | linux* | windows) NET_DEVICE="virtio-net-pci" ;;
    freedos)
        sound_card="${sound_card:-sb16}"
        NET_DEVICE="pcnet"
        ;;
    *solaris)
        usb_controller="xhci"
        sound_card="${sound_card:-ac97}"
        ;;
    reactos)
        NET_DEVICE="e1000"
        keyboard="ps2"
        ;;
    templeos)
        NET_DEVICE="e1000"
        keyboard="ps2"
        mouse="ps2"
        sound_card="${sound_card:-none}"
        usb_controller="none"
        ;;
    macos)
        # Tune QEMU optimisations based on the macOS release, or fallback to lowest
        # common supported options if none is specified.
        #   * VirtIO Block Media doesn't work in High Sierra (at all) or the Mojave (Recovery Image)
        #   * VirtIO Network is supported since Big Sur
        #   * VirtIO Memory Balloning is supported since Big Sur (https://pmhahn.github.io/virtio-balloon/)
        #   * VirtIO RNG is supported since Big Sur, but exposed to all guests by default.
        case ${macos_release} in
        monterey | ventura | sonoma | sequoia | tahoe)
            # macOS 12+ (Monterey onwards) supports virtio-sound-pci natively
            BALLOON="-device virtio-balloon"
            MAC_DISK_DEV="virtio-blk-pci"
            NET_DEVICE="virtio-net-pci"
            USB_HOST_PASSTHROUGH_CONTROLLER="nec-usb-xhci"
            GUEST_TWEAKS+=" -global nec-usb-xhci.msi=off"
            sound_card="${sound_card:-virtio-sound-pci}"
            usb_controller="xhci"
            ;;
        big-sur)
            # Big Sur uses VirtIO but usb-audio/VoodooHDA don't work reliably
            # Fall back to ich9-intel-hda which may work with VoodooHDA
            BALLOON="-device virtio-balloon"
            MAC_DISK_DEV="virtio-blk-pci"
            NET_DEVICE="virtio-net-pci"
            USB_HOST_PASSTHROUGH_CONTROLLER="nec-usb-xhci"
            GUEST_TWEAKS+=" -global nec-usb-xhci.msi=off"
            sound_card="${sound_card:-ich9-intel-hda}"
            usb_controller="xhci"
            ;;
        *)
            # Backwards compatibility if no macos_release is specified.
            # Also safe catch all for High Sierra and Mojave
            BALLOON=""
            if [ "${macos_release}" == "catalina" ]; then
                MAC_DISK_DEV="virtio-blk-pci"
            else
                MAC_DISK_DEV="ide-hd,bus=ahci.2"
            fi
            NET_DEVICE="vmxnet3"
            USB_HOST_PASSTHROUGH_CONTROLLER="usb-ehci"
            ;;
        esac
        ;;
    *) NET_DEVICE="rtl8139" ;;
    esac
}

function configure_storage() {
    local create_options=""
    echo " - Disk:     ${disk_img} (${disk_size})"
    if [ ! -f "${disk_img}" ]; then
        if [ -n "${DRY_RUN}" ]; then
            # Planning only: describe the disk that would be created, create nothing.
            echo "             Would create ${disk_img} (${disk_format}, ${disk_size})"
        else
            # If there is no disk image, create a new image.
            mkdir -p "${VMDIR}" 2>/dev/null
            case ${preallocation} in
            off | metadata | falloc | full) true ;;
            *)
                echo "ERROR! ${preallocation} is an unsupported disk preallocation option."
                exit 1
                ;;
            esac

            case ${disk_format} in
            qcow2) create_options="lazy_refcounts=on,preallocation=${preallocation},nocow=on" ;;
            raw) create_options="preallocation=${preallocation}" ;;
            *) true ;;
            esac

            # https://blog.programster.org/qcow2-performance
            if ! ${QEMU_IMG} create -q -f "${disk_format}" -o "${create_options=}" "${disk_img}" "${disk_size}"; then
                echo "ERROR! Failed to create ${disk_img} using ${disk_format} format."
                exit 1
            fi

            if [ -z "${iso}" ] && [ -z "${img}" ]; then
                echo "ERROR! You haven't specified a .iso or .img image to boot from."
                exit 1
            fi
            echo "             Just created, booting from ${iso}${img}"
        fi
        DISK_USED="no"
    elif [ -e "${disk_img}" ]; then
        # If the VM is not running, check for disk related issues.
        if [ -z "${VM_PID}" ]; then
            # Check there isn't already a process attached to the disk image.
            if ! ${QEMU_IMG} info "${disk_img}" >/dev/null; then
                echo "             Failed to get \"write\" lock. Is another process using the disk?"
                exit 1
            fi
        else
            if ! ${QEMU_IMG} check -q "${disk_img}"; then
                echo "             Disk integrity check failed. Please run qemu-img check --help."
                echo
                "${QEMU_IMG}" check "${disk_img}"
                exit 1
            fi
        fi

        # Only check disk image size if preallocation is off
        if [ "${preallocation}" == "off" ]; then
            # A newly created qcow2 overlay can be physically small while
            # exposing all data from its backing file. Treat such images as
            # used instead of relying solely on the overlay file size.
            if "${QEMU_IMG}" info --output=json "${disk_img}" 2>/dev/null |
                grep -q '"backing-filename"[[:space:]]*:'; then
                DISK_USED="yes"
            else
                DISK_CURR_SIZE=$(${STAT} -c%s "${disk_img}")
                if [ "${DISK_CURR_SIZE}" -le "${DISK_MIN_SIZE}" ]; then
                    echo "             Looks unused, booting from ${iso}${img}"
                    if [ -z "${iso}" ] && [ -z "${img}" ]; then
                        echo "ERROR! You haven't specified a .iso or .img image to boot from."
                        exit 1
                    fi
                else
                    DISK_USED="yes"
                fi
            fi
        else
            DISK_USED="yes"
        fi
    fi

    if [ "${DISK_USED}" == "yes" ] && [ "${guest_os}" != "kolibrios" ]; then
        # If there is a disk image that appears to be used do not boot from installation media.
        iso=""
        img=""
    fi

    # Has the status quo been requested?
    if [ "${STATUS_QUO}" == "-snapshot" ]; then
        if [ -z "${img}" ] && [ -z "${iso}" ]; then
            echo "             Existing disk state will be preserved, no writes will be committed."
        fi
    fi

    if [ -n "${iso}" ] && [ -e "${iso}" ]; then
        echo " - Boot ISO: ${iso}"
    elif [ -n "${img}" ] && [ -e "${img}" ]; then
        echo " - Recovery: ${img}"
    fi

    if [ -n "${fixed_iso}" ] && [ -e "${fixed_iso}" ]; then
        echo " - CD-ROM:   ${fixed_iso}"
    fi
}

function configure_audio() {
    # Build the sound hardware configuration
    sound_card="${sound_card:-intel-hda}"
    case ${sound_card} in
    ich9-intel-hda | intel-hda) SOUND="-device ${sound_card} -device ${sound_duplex},audiodev=audio0" ;;
    usb-audio) SOUND="-device ${sound_card},audiodev=audio0" ;;
    virtio-sound-pci) SOUND="-device ${sound_card},audiodev=audio0" ;;
    ac97 | es1370 | sb16) SOUND="-device ${sound_card},audiodev=audio0" ;;
    none) SOUND="" ;;
    esac
    echo " - Sound:    ${sound_card} (${sound_duplex})"
}

function configure_ports() {
    if [ -z "${DRY_RUN}" ]; then
        echo -n "" >"${VMDIR}/${VMNAME}.ports"
        qe_safe_rm "${VMDIR}/${VMNAME}.spice"
        qe_safe_rm "${VMDIR}/${VMNAME}.sock"
    fi

    if [ -z "${ssh_port}" ]; then
        # Find a free port to expose ssh to the guest
        ssh_port=$(get_port 22220 9)
    fi

    if [ -z "${SSH_ACCESS}" ] || [ "${SSH_ACCESS}" == "local" ]; then
        SSH_ADDR="127.0.0.1"
        ssh_host="localhost"
    elif [ "${SSH_ACCESS}" == "remote" ]; then
        SSH_ADDR=""
        ssh_host="localhost"
    else
        SSH_ADDR="${SSH_ACCESS}"
        ssh_host="${SSH_ACCESS}"
    fi

    if [ -n "${ssh_port}" ]; then
        ports_record "ssh,${ssh_port}"
        NET="${NET},hostfwd=tcp:${SSH_ADDR}:${ssh_port}-:22"
        echo " - ssh:      On host:  ssh user@${ssh_host} -p ${ssh_port}"
    else
        echo " - ssh:      All ssh ports have been exhausted."
    fi

    # Have any port forwards been requested?
    if ((${#port_forwards[@]})); then
        echo " - PORTS:    Port forwards requested:"
        for FORWARD in "${port_forwards[@]}"; do
            HOST_PORT=$(echo "${FORWARD}" | cut -d':' -f1)
            GUEST_PORT=$(echo "${FORWARD}" | cut -d':' -f2)
            echo "              - ${HOST_PORT} => ${GUEST_PORT}"
            NET="${NET},hostfwd=tcp::${HOST_PORT}-:${GUEST_PORT}"
            NET="${NET},hostfwd=udp::${HOST_PORT}-:${GUEST_PORT}"
        done
    fi

    if [ "${display}" == "none" ] || [ "${display}" == "spice" ] || [ "${display}" == "spice-app" ]; then
        SPICE="disable-ticketing=on"

        if [ "${display}" == "spice-app" ]; then
            # spice-app uses QEMU's built-in viewer with GL support
            SPICE+=",gl=${gl}"
            echo " - SPICE:    Enabled"
        elif [ "${display}" == "spice" ]; then
            # For spice display, use Unix socket for local or TCP for remote
            if [ -z "${ACCESS}" ] || [ "${ACCESS}" == "local" ]; then
                # Unix socket mode for local access
                # GL context is provided by egl-headless display, not SPICE
                SPICE+=",unix=on,addr=${VMDIR}/${VMNAME}.sock"
                ports_record "unix,${VMDIR}/${VMNAME}.sock"
                spice_record "${VMDIR}/${VMNAME}.sock"
                echo -n " - SPICE:    On host:  spicy --uri=\"spice+unix://${VMDIR}/${VMNAME}.sock\" --title \"${VMNAME}\""
                if [ "${guest_os}" != "macos" ] && [ -n "${PUBLIC}" ]; then
                    echo -n " --spice-shared-dir ${PUBLIC}"
                fi
                echo "${FULLSCREEN}"
            else
                # TCP mode for remote access (no GL support)
                if [ -z "${spice_port}" ]; then
                    spice_port=$(get_port 5930 9)
                fi

                if [ "${ACCESS}" == "remote" ]; then
                    SPICE_ADDR=""
                else
                    SPICE_ADDR="${ACCESS}"
                fi

                if [ -z "${spice_port}" ]; then
                    echo " - SPICE:    All SPICE ports have been exhausted."
                    echo "             ERROR! Requested SPICE display, but no SPICE ports are free."
                    exit 1
                fi

                SPICE+=",port=${spice_port},addr=${SPICE_ADDR}"
                ports_record "spice,${spice_port}"
                spice_record "${spice_port}"
                echo -n " - SPICE:    On host:  spicy --title \"${VMNAME}\" --port ${spice_port}"
                if [ "${guest_os}" != "macos" ] && [ -n "${PUBLIC}" ]; then
                    echo -n " --spice-shared-dir ${PUBLIC}"
                fi
                echo "${FULLSCREEN}"
            fi
        elif [ "${display}" == "none" ]; then
            # display=none with SPICE for headless VMs - use TCP for remote access
            if [ -z "${spice_port}" ]; then
                spice_port=$(get_port 5930 9)
            fi

            if [ -z "${ACCESS}" ]; then
                SPICE_ADDR="127.0.0.1"
            elif [ "${ACCESS}" == "remote" ]; then
                SPICE_ADDR=""
            elif [ "${ACCESS}" == "local" ]; then
                SPICE_ADDR="127.0.0.1"
            else
                SPICE_ADDR="${ACCESS}"
            fi

            if [ -z "${spice_port}" ]; then
                echo " - SPICE:    All SPICE ports have been exhausted."
                echo "             ERROR! Requested SPICE display, but no SPICE ports are free."
                exit 1
            fi

            SPICE+=",port=${spice_port},addr=${SPICE_ADDR}"
            ports_record "spice,${spice_port}"
            spice_record "${spice_port}"
            echo -n " - SPICE:    On host:  spicy --title \"${VMNAME}\"${SPICE_ADDR:+ --host ${SPICE_ADDR}} --port ${spice_port}"
            if [ "${guest_os}" != "macos" ] && [ -n "${PUBLIC}" ]; then
                echo -n " --spice-shared-dir ${PUBLIC}"
            fi
            echo "${FULLSCREEN}"
        fi
    fi
}

function configure_file_sharing() {
    if [ -n "${PUBLIC}" ]; then
        # WebDAV
        case ${guest_os} in
        macos)
            if [ "${display}" == "none" ] || [ "${display}" == "spice" ] || [ "${display}" == "spice-app" ]; then
                # Reference: https://gitlab.gnome.org/GNOME/phodav/-/issues/5
                echo " - WebDAV:   On guest: build spice-webdavd (https://gitlab.gnome.org/GNOME/phodav/-/merge_requests/24)"
                echo " - WebDAV:   On guest: Finder -> Connect to Server -> http://localhost:9843/"
            fi
            ;;
        *) echo " - WebDAV:   On guest: dav://localhost:9843/" ;;
        esac

        # virtiofs or 9p depending on host capability
        if [ "${guest_os}" != "windows" ] || [ "${guest_os}" == "windows-server" ]; then
            if [ "${guest_os}" == "linux" ] && [ -n "${VIRTIOFSD}" ]; then
                echo " - virtiofs: On guest: sudo mount -t virtiofs ${PUBLIC_TAG} ~/$(basename "${PUBLIC}")"
            elif [ "${guest_os}" == "linux" ]; then
                echo " - 9P:       On guest: sudo mount -t 9p -o trans=virtio,version=9p2000.L,msize=512000 ${PUBLIC_TAG} ~/$(basename "${PUBLIC}")"
            elif [ "${guest_os}" == "macos" ]; then
                # PUBLICSHARE needs to be world writeable for seamless integration with
                # macOS. Test if it is world writeable, and prompt what to do if not.
                echo " - 9P:       On guest: sudo mount_9p ${PUBLIC_TAG}"
                if [ "${PUBLIC_PERMS}" != "drwxrwxrwx" ]; then
                    echo " - 9P:       On host:  chmod 777 ${PUBLIC}"
                    echo "             Required for macOS integration 👆"
                fi
            fi
        fi

        # SMB
        # We need to search in NixOS compatible paths as well as the standard location
        # since /usr/sbin/smbd may not be in the PATH.
        if [ -x "$(command -v smbd)" ] || [ -x "/usr/sbin/smbd" ]; then
            NET+=",smb=${PUBLIC}"
            echo " - smbd:     On guest: smb://10.0.2.4/qemu"
        fi
    fi
}

function configure_tpm() {
    # Start TPM
    if [ "${tpm}" == "on" ]; then
        local tpm_args=()
        # shellcheck disable=SC2054
        tpm_args+=(socket
            --ctrl type=unixio,path="${VMDIR}/${VMNAME}.swtpm-sock"
            --terminate
            --tpmstate dir="${VMDIR}"
            --tpm2)
        if [ -z "${DRY_RUN}" ]; then
            echo "${SWTPM} ${tpm_args[*]} &" >>"${VMDIR}/${VMNAME}.sh"
            ${SWTPM} "${tpm_args[@]}" >>"${VMDIR}/${VMNAME}.log" &
            echo " - TPM:      ${VMDIR}/${VMNAME}.swtpm-sock (${!})"
            sleep 0.25
        else
            echo " - TPM:      ${VMDIR}/${VMNAME}.swtpm-sock (planned)"
        fi
    fi
}

function configure_cpu_pinning() {
    if [ -z "${CORE_MAPPING}" ]; then
        return
    fi

    GUEST_CPUS=""
    idx=0
    for tid_dir in /proc/"${VM_PID}"/task/*; do
        tid=$(basename "$tid_dir")
        name=$(cat "$tid_dir/comm")

        if [[ "$name" == CPU* ]]; then
            # Map per core, if threads are specified pin them to the same core
            core_idx=$((idx / GUEST_CPU_THREADS))
            host_cpu=${CORE_MAPPING[$core_idx]}

            if ((idx % GUEST_CPU_THREADS == 0)); then
                [[ -n "$GUEST_CPUS" ]] && GUEST_CPUS+=","
                GUEST_CPUS+="$core_idx"
            fi

            taskset -cp "$host_cpu" "$tid" &>/dev/null
            idx=$((idx + 1))
        fi
    done

    echo " - CPU Pinning: Bind guest cores to host cores (${GUEST_CPUS} -> ${CPU_PINNING})"
}
