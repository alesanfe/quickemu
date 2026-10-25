# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Sourced partial: shares the conf-schema global namespace with bin/quickemu.
# Command-line argument parsing -> ACTIONS + option globals.

cli_parse_args() {
# Take command line arguments
if [ $# -lt 1 ]; then
    usage
    exit 1
else
    while [ $# -gt 0 ]; do
        case "${1}" in
        -access | --access)
            SHORTCUT_OPTIONS+="--access ${2} "
            ACCESS="${2}"
            shift 2
            ;;
        -braille | --braille)
            SHORTCUT_OPTIONS+="--braille "
            BRAILLE="on"
            shift
            ;;
        -cpu-pinning | --cpu-pinning)
            SHORTCUT_OPTIONS+="--cpu-pinning ${2} "
            CPU_PINNING=${2}
            shift 2
            ;;
        -delete | --delete | -delete-disk | --delete-disk)
            ACTIONS+=(delete_disk)
            shift
            ;;
        -delete-plan | --delete-plan)
            ACTIONS+=(delete_plan)
            shift
            ;;
        -delete-vm | --delete-vm)
            ACTIONS+=(delete_vm)
            shift
            ;;
        -display | --display)
            SHORTCUT_OPTIONS+="--display ${2} "
            display="${2}"
            # display_param_check is deferred to the launch path: at parse
            # time QEMU has not been resolved yet, so the GTK availability
            # probe would always fail and silently downgrade to SDL.
            shift 2
            ;;
        -fullscreen | --fullscreen | -full-screen | --full-screen)
            SHORTCUT_OPTIONS+="--fullscreen "
            FULLSCREEN="--full-screen"
            shift
            ;;
        -ignore-msrs-always | --ignore-msrs-always)
            ignore_msrs_always
            exit
            ;;
        -ignore-tsc-warning | --ignore-tsc-warning)
            IGNORE_TSC_WARNING="1"
            shift
            ;;
        -kill | --kill)
            ACTIONS+=(kill_vm)
            shift
            ;;
        -shutdown | --shutdown)
            ACTIONS+=(shutdown_vm)
            shift
            ;;
        -reset | --reset)
            ACTIONS+=(reset_vm)
            shift
            ;;
        -format | --format)
            OUTPUT_FORMAT="${2}"
            if [ "${OUTPUT_FORMAT}" != "human" ] && [ "${OUTPUT_FORMAT}" != "json" ]; then
                echo "ERROR! '--format' must be 'human' or 'json'."
                exit 1
            fi
            shift 2
            ;;
        -capabilities | --capabilities)
            ACTIONS+=(capabilities)
            shift
            ;;
        -status | --status)
            ACTIONS+=(status_vm)
            shift
            ;;
        -pause | --pause)
            ACTIONS+=(pause_vm)
            shift
            ;;
        -resume | --resume)
            ACTIONS+=(resume_vm)
            shift
            ;;
        -screenshot | --screenshot)
            ACTIONS+=(screenshot_vm)
            if [ -n "${2}" ] && [[ "${2}" != -* ]]; then
                SCREENSHOT_OUT="${2}"
                shift 2
            else
                shift
            fi
            ;;
        -clone | --clone)
            ACTIONS+=(clone_vm)
            if [ -n "${2}" ] && [[ "${2}" != -* ]]; then
                CLONE_NAME="${2}"
                shift 2
            else
                shift
            fi
            ;;
        -full | --full)
            CLONE_FULL="true"
            shift
            ;;
        -disk-resize | --disk-resize)
            ACTIONS+=(disk_resize)
            DISK_SIZE_ARG="${2}"
            shift 2
            ;;
        -disk-compact | --disk-compact)
            ACTIONS+=(disk_compact)
            shift
            ;;
        -export | --export)
            ACTIONS+=(export_vm)
            EXPORT_FILE="${2}"
            shift 2
            ;;
        -import | --import)
            ACTIONS+=(import_vm)
            IMPORT_FILE="${2}"
            shift 2
            ;;
        -rename | --rename)
            ACTIONS+=(rename_vm)
            RENAME_TO="${2}"
            shift 2
            ;;
        -send-keys | --send-keys)
            ACTIONS+=(send_keys)
            SEND_KEYS_ARG="${2}"
            shift 2
            ;;
        -type | --type)
            ACTIONS+=(type_text)
            TYPE_TEXT="${2}"
            shift 2
            ;;
        -list-vms | --list-vms)
            ACTIONS+=(list_vms)
            shift
            ;;
        -doctor | --doctor | -check | --check)
            ACTIONS+=(doctor_vm)
            shift
            ;;
        -autostart | --autostart)
            ACTIONS+=(autostart_create)
            shift
            ;;
        -usb-passthrough | --usb-passthrough)
            SHORTCUT_OPTIONS+="--usb-passthrough ${2} "
            usb_devices+=("${2}")
            shift 2
            ;;
        -dry-run | --dry-run | -echo | --echo)
            DRY_RUN="true"
            shift
            ;;
        -offline | --offline)
            SHORTCUT_OPTIONS+="--offline "
            network="none"
            shift
            ;;
        -snapshot | --snapshot)
            if [ -z "${2}" ]; then
                echo "ERROR! '--snapshot' needs an action to perform."
                exit 1
            fi
            SNAPSHOT_ACTION="${2}"
            if [ -z "${3}" ] && [ "${SNAPSHOT_ACTION}" != "info" ] && [ "${SNAPSHOT_ACTION}" != "list" ]; then
                echo "ERROR! '--snapshot ${SNAPSHOT_ACTION}' needs a tag."
                exit 1
            fi
            SNAPSHOT_TAG="${3}"
            if [ "${SNAPSHOT_ACTION}" == "info" ] || [ "${SNAPSHOT_ACTION}" == "list" ]; then
                shift 2
            else
                shift 3
            fi
            ;;
        -status-quo | --status-quo)
            SHORTCUT_OPTIONS+="--status-quo "
            STATUS_QUO="-snapshot"
            shift
            ;;
        -shortcut | --shortcut)
            ACTIONS+=(shortcut_create)
            shift
            ;;
        -inspect-conf | --inspect-conf)
            ACTIONS+=(inspect_conf)
            shift
            ;;
        -delete-shortcut | --delete-shortcut)
            ACTIONS+=(delete_shortcut)
            shift
            ;;
        -vm | --vm)
            VM="${2}"
            shift 2
            ;;
        -viewer | --viewer)
            SHORTCUT_OPTIONS+="--viewer ${2} "
            viewer="${2}"
            shift 2
            ;;
        -width | --width)
            SHORTCUT_OPTIONS+="--width ${2} "
            width="${2}"
            shift 2
            ;;
        -height | --height)
            SHORTCUT_OPTIONS+="--height ${2} "
            height="${2}"
            shift 2
            ;;
        -ssh-access | --ssh-access)
            SHORTCUT_OPTIONS+="--ssh-access ${2} "
            SSH_ACCESS="${2}"
            shift 2
            ;;
        -ssh-port | --ssh-port)
            SHORTCUT_OPTIONS+="--ssh-port ${2} "
            ssh_port="${2}"
            shift 2
            ;;
        -spice-port | --spice-port)
            SHORTCUT_OPTIONS+="--spice-port ${2} "
            spice_port="${2}"
            shift 2
            ;;
        -public-dir | --public-dir)
            SHORTCUT_OPTIONS+="--public-dir ${2} "
            PUBLIC="${2}"
            shift 2
            ;;
        -monitor | --monitor)
            SHORTCUT_OPTIONS+="--monitor ${2} "
            monitor="${2}"
            shift 2
            ;;
        -monitor-cmd | --monitor-cmd)
            SHORTCUT_OPTIONS+="--monitor-cmd ${2} "
            MONITOR_CMD="${2}"
            shift 2
            ;;
        -monitor-telnet-host | --monitor-telnet-host)
            SHORTCUT_OPTIONS+="--monitor-telnet-host ${2} "
            monitor_telnet_host="${2}"
            shift 2
            ;;
        -monitor-telnet-port | --monitor-telnet-port)
            SHORTCUT_OPTIONS+="--monitor-telnet-port ${2} "
            monitor_telnet_port="${2}"
            shift 2
            ;;
        -serial | --serial)
            SHORTCUT_OPTIONS+="--serial ${2} "
            serial="${2}"
            shift 2
            ;;
        -serial-telnet-host | --serial-telnet-host)
            SHORTCUT_OPTIONS+="--serial-telnet-host ${2} "
            serial_telnet_host="${2}"
            shift 2
            ;;
        -serial-telnet-port | --serial-telnet-port)
            SHORTCUT_OPTIONS+="--serial-telnet-port ${2} "
            serial_telnet_port="${2}"
            shift 2
            ;;
        -keyboard | --keyboard)
            SHORTCUT_OPTIONS+="--keyboard ${2} "
            keyboard="${2}"
            shift 2
            ;;
        -keyboard_layout | --keyboard_layout)
            SHORTCUT_OPTIONS+="--keyboard_layout ${2} "
            keyboard_layout="${2}"
            shift 2
            ;;
        -mouse | --mouse)
            SHORTCUT_OPTIONS+="--mouse ${2} "
            mouse="${2}"
            shift 2
            ;;
        -usb-controller | --usb-controller)
            SHORTCUT_OPTIONS+="--usb-controller ${2} "
            usb_controller="${2}"
            shift 2
            ;;
        -extra_args | --extra_args)
            SHORTCUT_OPTIONS+="--extra_args ${2} "
            extra_args+="${2}"
            shift 2
            ;;
        -sound-card | --sound-card)
            SHORTCUT_OPTIONS+="--sound-card ${2} "
            sound_card="${2}"
            shift 2
            ;;
        -sound-duplex | --sound-duplex)
            SHORTCUT_OPTIONS+="--sound-duplex ${2} "
            sound_duplex="${2}"
            shift 2
            ;;
        -version | --version)
            echo "${VERSION}"
            exit
            ;;
        -h | --h | -help | --help)
            usage
            exit 0
            ;;
        *)
            echo "ERROR! \"${1}\" is not a supported parameter."
            usage
            exit 1
            ;;
        esac
    done
fi
}
