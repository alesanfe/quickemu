# Bash completion for quickemu — source or install to /etc/bash_completion.d/
_quickemu() {
    local cur prev opts
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"
    opts="--access --autostart --backup --backup-list --backup-prune \
--backup-restore --backup-verify --braille --capabilities --check --clone \
--cpu-pinning --delete-disk --delete-plan --delete-shortcut --delete-vm \
--disk-compact --disk-resize --display --doctor --dry-run --echo --events \
--export --external --extra-args --fix --format --full --fullscreen \
--height --host-info --hotplug-disk --hotplug-usb --hotunplug \
--ignore-msrs --ignore-msrs-always --ignore-tsc-warning --import \
--inspect-conf --inventory --keyboard --keyboard-layout --kill \
--linked-clone --list-vms --migrate-conf --monitor --monitor-cmd \
--monitor-console --monitor-telnet-host --monitor-telnet-port --mouse \
--move --offline --op-cancel --op-list --op-status --op-watch --pause \
--persist --plan-id --plugin --plugins --profile --public-dir --publicdir \
--rename --reset --restart --resume --screenshot --send-keys --serial \
--serial-console --serial-telnet-host --serial-telnet-port --shortcut \
--shutdown --snapshot --snapshot-tree --snapshot-verify --sound-card \
--sound-duplex --spice-console --spice-port --ssh-access --ssh-port \
--start --stats --status --status-quo --stop --trash --trash-empty \
--trash-list --trash-restore --type --usb-controller --usb-passthrough \
--version --vfio-scan --viewer --vm --vnc --width"

    case "${prev}" in
        --vm)             COMPREPLY=($(compgen -f -X '!*.@(conf|toml)' -- "${cur}")); return 0;;
        --display)        COMPREPLY=($(compgen -W "gtk sdl spice spice-app cocoa vnc none" -- "${cur}")); return 0;;
        --snapshot)       COMPREPLY=($(compgen -W "apply create delete info list tree verify" -- "${cur}")); return 0;;
        --monitor|--serial) COMPREPLY=($(compgen -W "socket telnet none" -- "${cur}")); return 0;;
        --usb-controller) COMPREPLY=($(compgen -W "ehci xhci none" -- "${cur}")); return 0;;
        --keyboard)       COMPREPLY=($(compgen -W "usb virtio ps2" -- "${cur}")); return 0;;
        --keyboard-layout) COMPREPLY=($(compgen -W "en-us en-gb es de fr it pt" -- "${cur}")); return 0;;
        --mouse)          COMPREPLY=($(compgen -W "tablet virtio ps2 usb" -- "${cur}")); return 0;;
        --format)         COMPREPLY=($(compgen -W "human json text" -- "${cur}")); return 0;;
        --export|--import|--backup-verify|--backup-restore)
                          COMPREPLY=($(compgen -f -X '!*.tar.gz' -- "${cur}")); return 0;;
        --access|--ssh-access) COMPREPLY=($(compgen -W "local remote" -- "${cur}")); return 0;;
        --sound-card)     COMPREPLY=($(compgen -W "intel-hda ac97 es1370 ich9-intel-hda sb16 usb-audio virtio-sound-pci none" -- "${cur}")); return 0;;
        --sound-duplex)   COMPREPLY=($(compgen -W "hda-micro hda-duplex hda-output" -- "${cur}")); return 0;;
        --viewer)         COMPREPLY=($(compgen -W "spicy remote-viewer none" -- "${cur}")); return 0;;
        --profile)        COMPREPLY=($(compgen -W "macos windows linux linux_old kolibrios freedos windows-server reactos haiku" -- "${cur}")); return 0;;
        --trash-restore|--op-status|--op-watch|--op-cancel|--plan-id)
                          COMPREPLY=($(compgen -f -- "${cur}")); return 0;;
        --move|--linked-clone|--public-dir)
                          COMPREPLY=($(compgen -d -- "${cur}")); return 0;;
        --clone|--rename|--disk-resize|--hotplug-usb|--hotunplug|--monitor-cmd|--send-keys|--type|--plugin|--events)
                          return 0;;
        *) ;;
    esac
    if [[ "${cur}" == -* ]]; then
        COMPREPLY=($(compgen -W "${opts}" -- "${cur}"))
    else
        COMPREPLY=($(compgen -f -X '!*.@(conf|toml)' -- "${cur}"))
    fi
    return 0
}
complete -F _quickemu quickemu
