# Bash completion for quickemu — source or install to /etc/bash_completion.d/
_quickemu() {
    local cur prev opts
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"
    opts="--access --autostart --braille --clone --cpu-pinning --ctrl --delete \
--delete-disk --delete-vm --disk-compact --disk-resize --display --doctor \
--dry-run --export --extra --full --fullscreen --height --ignore-msrs-always \
--ignore-tsc-warning --import --keyboard --kill --list-vms --monitor \
--monitor-cmd --monitor-telnet-host --monitor-telnet-port --mouse --offline \
--pause --port --public-dir --rename --resume --screenshot --send-keys \
--serial --serial-telnet-host --serial-telnet-port --shortcut --snapshot \
--socket-path --sound-card --sound-duplex --spice-port --spice-shared-dir \
--ssh-access --ssh-port --status --status-quo --terminate --title --tpm \
--tpmstate --translate-gid --translate-uid --type --uri --usb-controller \
--usb-passthrough --version --viewer --vm --width"

    case "${prev}" in
        --vm)             COMPREPLY=($(compgen -f -X '!*.conf' -- "${cur}")); return 0;;
        --display)        COMPREPLY=($(compgen -W "gtk sdl spice spice-app cocoa vnc none" -- "${cur}")); return 0;;
        --snapshot)       COMPREPLY=($(compgen -W "apply create delete info list" -- "${cur}")); return 0;;
        --monitor)        COMPREPLY=($(compgen -W "socket telnet none" -- "${cur}")); return 0;;
        --serial)         COMPREPLY=($(compgen -W "socket telnet none" -- "${cur}")); return 0;;
        --usb-controller) COMPREPLY=($(compgen -W "ehci xhci none" -- "${cur}")); return 0;;
        --keyboard)       COMPREPLY=($(compgen -W "usb virtio ps2" -- "${cur}")); return 0;;
        --mouse)          COMPREPLY=($(compgen -W "tablet virtio ps2 usb" -- "${cur}")); return 0;;
        --export|--import) COMPREPLY=($(compgen -f -X '!*.tar.gz' -- "${cur}")); return 0;;
        --access)         COMPREPLY=($(compgen -W "local remote" -- "${cur}")); return 0;;
        *) ;;
    esac
    if [[ "${cur}" == -* ]]; then
        COMPREPLY=($(compgen -W "${opts}" -- "${cur}"))
    else
        COMPREPLY=($(compgen -f -X '!*.conf' -- "${cur}"))
    fi
    return 0
}
complete -F _quickemu quickemu
