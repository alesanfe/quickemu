# Fish completion for quickemu.
set -l subs status pause resume reset shutdown kill restart delete-vm clone rename move backup backup-list backup-verify backup-restore backup-prune trash trash-list trash-restore trash-empty op-list op-status op-cancel snapshot vfio-scan doctor host-info capabilities migrate-conf inspect-conf
complete -c quickemu -f -a "$subs" -d 'Subcommand'
complete -c quickemu -l vm -r -d 'VM config (.conf/.toml)'
complete -c quickemu -l format -xa 'human json' -d 'Output format'
complete -c quickemu -l snapshot -xa 'apply create delete info list tree verify' -d 'Snapshot action'
complete -c quickemu -l external -d 'External qcow2 snapshot'
complete -c quickemu -l doctor -d 'Diagnose stale state'
complete -c quickemu -l fix -d 'Repair safe findings'
complete -c quickemu -l persist -d 'Persist hotplug to conf'
complete -c quickemu -l dry-run -d 'Preview without executing'
