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
    # Subcommand aliases: `quickemu status --vm x` == `quickemu --status
    # --vm x`. A leading bare word matching a known action is rewritten to
    # its flag form; everything else falls through untouched.
    case "${1:-}" in
    status | vfio-scan | disk-compact | move | pause | resume | reset | shutdown | kill | restart | delete-vm | delete-plan | list-vms | capabilities | host-info | migrate-conf | inspect-conf | backup | backup-list | trash | trash-list | clone | rename | snapshot-apply | snapshot-create | snapshot-delete | snapshot-info | snapshot-list | monitor-cmd | send-keys | type | screenshot | viewer | fullscreen | status-quo | vnc | spice-console | publicdir | ignore-msrs | ignore-msrs-always | create-disk | disk-resize | disk-resize-commit | backup-verify | backup-restore | trash-restore | op-status | operation-status | operation-list | operation-cancel | op-watch | operation-watch)
        set -- "--${1}" "${@:2}"
        ;;
    esac
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
        -disk-compact | --disk-compact)
            ACTIONS+=(disk_compact)
            shift
            ;;
        -move | --move)
            MOVE_TO="${2}"
            ACTIONS+=(move_vm)
            shift 2
            ;;
        -fix | --fix)
            DOCTOR_FIX="--fix"
            shift
            ;;
        -vfio-scan | --vfio-scan)
            ACTIONS+=(vfio_scan)
            shift
            ;;
        -host-info | --host-info)
            ACTIONS+=(host_info)
            shift
            ;;
        -migrate-conf | --migrate-conf)
            ACTIONS+=(migrate_conf)
            shift
            ;;
        -backup | --backup)
            ACTIONS+=(backup_vm)
            shift
            ;;
        -backup-list | --backup-list)
            ACTIONS+=(backup_list)
            shift
            ;;
        -backup-verify | --backup-verify)
            BACKUP_FILE="${2}"
            ACTIONS+=(backup_verify)
            shift 2
            ;;
        -backup-restore | --backup-restore)
            BACKUP_FILE="${2}"
            ACTIONS+=(backup_restore)
            shift 2
            ;;
        -persist | --persist)
            HOTPLUG_PERSIST=1
            shift
            ;;
        -hotplug-usb | --hotplug-usb)
            ACTIONS+=(vm_hotplug_usb "${2}")
            shift 2
            ;;
        -hotplug-disk | --hotplug-disk)
            ACTIONS+=(vm_hotplug_disk "${2}")
            shift 2
            ;;
        -hotunplug | --hotunplug)
            ACTIONS+=(vm_hotunplug "${2}")
            shift 2
            ;;
        -backup-prune | --backup-prune)
            BACKUP_KEEP="${2:-5}"
            ACTIONS+=(backup_prune)
            shift 2
            ;;
        -trash-empty | --trash-empty)
            ACTIONS+=(trash_empty)
            shift
            ;;
        -trash | --trash)
            TRASH=1
            shift
            ;;
        -trash-list | --trash-list)
            ACTIONS+=(trash_list)
            shift
            ;;
        -trash-restore | --trash-restore)
            RESTORE_VM="${2}"
            ACTIONS+=(trash_restore)
            shift 2
            ;;
        -events | --events)
            EVENTS_CURSOR="${2:-0}"
            case "${EVENTS_CURSOR}" in ''|*[!0-9]*) EVENTS_CURSOR=0 ;; esac
            ACTIONS+=(events_read)
            shift
            # consume the optional cursor arg when present
            if [ $# -gt 0 ] && [[ "${1}" =~ ^[0-9]+$ ]]; then shift; fi
            ;;
        -external | --external)
            EXTERNAL=1
            shift
            ;;
        -snapshot-tree | --snapshot-tree)
            ACTIONS+=(snapshot_tree)
            shift
            ;;
        -snapshot-verify | --snapshot-verify)
            ACTIONS+=(snapshot_verify)
            shift
            ;;
        -op-list | --op-list | -operation-list | --operation-list)
            ACTIONS+=(operation_list)
            shift
            ;;
        -op-cancel | --op-cancel | -operation-cancel | --operation-cancel)
            OP_ID="${2}"
            ACTIONS+=(operation_cancel)
            shift 2
            ;;
        -plugin | --plugin)
            plugin_dispatch "${2}" "${@:3}"
            ;;
        -plugins | --plugins)
            ACTIONS+=(plugin_list)
            shift
            ;;
        -stats | --stats)
            ACTIONS+=(vm_stats)
            shift
            ;;
        -inventory | --inventory)
            ACTIONS+=(inventory_show)
            shift
            ;;
        -profile | --profile)
            PROFILE="${2}"
            shift 2
            ;;
        -linked-clone | --linked-clone)
            ACTIONS+=(linked_clone_vm)
            LINKED_DST="${2}"
            shift 2
            ;;
        -op-watch | --op-watch | -operation-watch | --operation-watch)
            ACTIONS+=(operation_watch)
            shift
            ;;
        -op-status | --op-status | -operation-status | --operation-status)
            OP_ID="${2}"
            ACTIONS+=(operation_status)
            shift 2
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
            if [ -z "${3}" ] && [ "${SNAPSHOT_ACTION}" != "info" ] && [ "${SNAPSHOT_ACTION}" != "list" ] && [ "${SNAPSHOT_ACTION}" != "tree" ] && [ "${SNAPSHOT_ACTION}" != "verify" ]; then
                echo "ERROR! '--snapshot ${SNAPSHOT_ACTION}' needs a tag."
                exit 1
            fi
            SNAPSHOT_TAG="${3}"
            if [ "${SNAPSHOT_ACTION}" == "info" ] || [ "${SNAPSHOT_ACTION}" == "list" ] || [ "${SNAPSHOT_ACTION}" == "tree" ] || [ "${SNAPSHOT_ACTION}" == "verify" ]; then
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
        -plan-id | --plan-id)
            DELETE_PLAN_ID="${2}"
            shift 2
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
