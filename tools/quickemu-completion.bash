# Bash completion for quickemu subcommands and flags.
# Source: source tools/quickemu-completion.bash
_quickemu() {
    local cur prev cmds
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"
    cmds="status start pause resume reset shutdown kill restart delete-vm delete-plan list-vms capabilities host-info migrate-conf inspect-conf backup backup-list backup-verify backup-restore backup-prune trash trash-list trash-restore trash-empty clone rename move op-status op-list op-cancel snapshot vfio-scan events"
    case "${prev}" in
    --vm) COMPREPLY=($(compgen -f -X '!*.@(conf|toml)' -- "${cur}")); return ;;
    --trash-restore|--backup-verify|--backup-restore|--op-status|--op-cancel|--move) COMPREPLY=($(compgen -f -- "${cur}")); return ;;
    --snapshot) COMPREPLY=($(compgen -W 'apply create delete info list tree verify' -- "${cur}")); return ;;
    esac
    case "${cur}" in
    -*) COMPREPLY=($(compgen -W '--vm --status --pause --resume --reset --shutdown --kill --restart --delete-vm --delete-plan --plan-id --clone --rename --move --backup --backup-list --backup-verify --backup-restore --backup-prune --trash --trash-list --trash-restore --trash-empty --op-list --op-status --op-cancel --snapshot --external --snapshot-tree --snapshot-verify --hotplug-usb --hotplug-disk --hotunplug --persist --vfio-scan --host-info --migrate-conf --inspect-conf --capabilities --events --format --dry-run' -- "${cur}")) ;;
    *) COMPREPLY=($(compgen -W "${cmds}" -- "${cur}") $(compgen -f -X '!*.@(conf|toml)' -- "${cur}")) ;;
    esac
}
complete -F _quickemu quickemu
