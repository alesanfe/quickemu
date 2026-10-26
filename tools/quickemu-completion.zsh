#compdef quickemu
# Zsh completion for quickemu (subcommands + flags + file types).
_arguments \
    '--vm[VM config]:file:_files -g "*.conf *.toml"' \
    '--status[show state]' '--pause' '--resume' '--reset' '--shutdown' '--kill' '--restart' \
    '--delete-vm[delete VM]' '--delete-plan[preview delete]' '--plan-id[plan hash]:plan:' \
    '--clone[clone VM]:name:' '--rename[rename VM]:name:' '--move[move VM]:dir:_files -/' \
    '--backup' '--backup-list' '--backup-verify[verify]:file:_files' '--backup-restore[restore]:file:_files' '--backup-prune[keep N]:n:' \
    '--trash' '--trash-list' '--trash-restore[restore]:name:' '--trash-empty' \
    '--op-list' '--op-status[op id]:id:' '--op-cancel[op id]:id:' \
    '--snapshot[action]:action:(apply create delete info list tree verify)' '--external' \
    '--hotplug-usb[vid:pid]:dev:' '--hotplug-disk[file]:file:_files' '--hotunplug[id]:id:' '--persist' \
    '--vfio-scan' '--host-info' '--migrate-conf' '--inspect-conf' '--capabilities' \
    '--doctor' '--fix' '--events[cursor]:seq:' '--format[fmt]:fmt:(human json)' '--dry-run'
