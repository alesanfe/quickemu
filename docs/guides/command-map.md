# Command → Envelope Contract Map

The single source of truth for the `command` field of every v1
envelope. QuickGUI's `_commandMatches` normalises on this table —
any change here is a contract break requiring a schemaVersion bump.

Normalization (QuickGUI): strip a leading `vm.`/`host.`/`ops.`/`qmp.`
prefix, then `.`→`-`. Aliases exist for the lifecycle/legacy names
below — emitting the documented name always matches.

| CLI invocation | envelope `command` | Notes |
|---|---|---|
| --status | vm.status | `data.allowedOps` advertises per-state op names |
| --start / bare --vm (boot) | vm.started | `data.alreadyRunning` when live |
| --shutdown | vm.shutdown | ACPI request, async |
| --powerdown | vm.powerdown | same ACPI path, GUI batch stop |
| --stop / --kill | vm.stopped | `data.forced`/`alreadyStopped` |
| --reset | vm.reset | — |
| --pause | vm.paused | — |
| --resume | vm.resumed | — |
| --suspend | vm.suspend | — |
| --resume (suspended state) | vm.resume | — |
| --restart | vm.restart | composite stop+start, one envelope |
| --dry-run / --echo | vm.plan | `data.command` argv |
| --delete-plan | vm.delete-plan | `data.delete`/`planId` |
| --delete-vm | vm.delete | `data.trashed` when --trash |
| --trash (bare) | vm.trash | move-to-trash, `data.trashed` |
| --trash-list / -restore / -empty | vm.trash.list / .restore / .empty | — |
| --clone | vm.clone | operationId |
| --linked-clone | vm.linked-clone | — |
| --rename | vm.rename | — |
| --move | vm.move | operationId |
| --export | vm.export | operationId |
| --import | vm.import | operationId |
| --backup (incl. --backup composite) | vm.backup | operationId |
| --backup-incr | vm.backup | dirty-bitmap incrementals |
| --backup-incr-list / --backup-list | vm.backup.list | `data.backups` |
| --backup-restore | vm.backup-restore | operationId |
| --backup-verify | vm.backup-verify | — |
| --backup-prune | vm.backup-prune | — |
| --disk-resize | vm.disk-resize | operationId |
| --disk-resize-commit / --resize-commit | vm.disk-resize-commit | qemu-img commit into backing |
| --disk-compact | vm.disk-compact | operationId |
| --create-disk | vm.create-disk | — |
| --snapshot create/apply/delete | vm.snapshot.create/.apply/.delete | also --memory (incl. register/unregister internal ops) |
| --snapshot list / --snapshot-list-json | vm.snapshot.list | `data.snapshots` |
| --snapshot tree / info / verify / policy / consolidate | vm.snapshot.tree/.info/.verify/.policy/.consolidate | — |
| --hotplug-usb / --hotplug-disk | vm.hotplug | `data.devices` |
| --hotunplug | vm.hotunplug | — |
| --monitor-cmd | vm.monitor-cmd | `data.reply` |
| --send-keys | vm.send-keys | — |
| --type | vm.type | — |
| --screenshot | vm.screenshot | `data.path` |
| --spice-console | vm.spice-console | — |
| --vnc | vm.vnc | `data.port`/`uri` |
| --serial-console | vm.serial-console | — |
| --monitor-console | vm.monitor-console | — |
| --publicdir | vm.publicdir | `data.path` |
| --status-quo (bare) | vm.status-quo | `data.written` conf keys persisted |
| --autostart | vm.autostart | — |
| --shortcut / --delete-shortcut | vm.shortcut / vm.shortcut-delete | — |
| --vfio-scan | vm.vfio.scan | — |
| --host-info | host-info | — |
| --capabilities | capabilities | — |
| --inventory | vm.inventory | — |
| --stats | vm.stats | — |
| --list-vms | vm.list | `data.vms` |
| --op-list / --op-status / --op-cancel | ops.op-list / ops.op-status / ops.op-cancel | `data.operations`/`operationId` |
| --op-watch | JSONL stream, not an envelope | by design |
| --events | vm.events | `data.events` |
| --migrate-conf | vm.migrate-conf | — |
| --inspect-conf | vm.config | `data.entries` |
| --doctor / --check | vm.doctor | `data.findings` + `data.report` |
| --plugins / --plugin | vm.plugins / vm.plugin | — |
| ssh-config | vm.ssh_config | — |
| vm.ip (guest ip) | vm.ip | `data.addresses` |
| guest ping/info/exec/push/pull | guest.ping/.info/.exec/.push/.pull | — |
| guest shutdown/reboot/halt/fsfreeze | guest.shutdown/.reboot/.halt/.fsfreeze | — |
| guest usage errors | guest.usage | parse misuse |
| ssh usage errors | ssh.usage | parse misuse |
| early exits (conf missing, param check) | vm.<action> | `config_not_found`/`param_check` codes |
| error envelope | same `command` + `success:false` + `errors[]` | typed code |

## Typed error codes (stable — QuickGUI maps on these)

`LOCK_BUSY` `LOCK_TIMEOUT` `PORT_BUSY` `PORT_TAKEN` `PORT_RANGE_EXHAUSTED`
`PLAN_EXPIRED` `PLAN_MISMATCH` `OPERATION_NOT_FOUND` `OP_FINISHED`
`OP_PERSIST` `QMP_*` (QMP_UNAVAILABLE GREETING CAPABILITIES WRITE TIMEOUT
ERROR BAD_COMMAND LIMIT EOF) `TAR_SLIP` `ARCHIVE_LINK` `ARCHIVE_BOMB`
`ARCHIVE_BAD` `MANIFEST_MISSING` `MANIFEST_DUP` `CHECKSUM_MISMATCH`
`SNAPSHOT_CONFLICT` `VM_LOCKED` `VM_ALREADY_RUNNING` `VM_MUST_BE_STOPPED`
`VM_NOT_FOUND` `UNSAFE_ARCHIVE` `INVALID_ARGUMENT` `PROCESS_DEAD`
`UID_MISMATCH` `STARTTIME_MISMATCH` `COMM_MISMATCH` `EXE_MISMATCH`
`TOML_PARSE` `BAD_ARGS` `NOT_FOUND` `WATCH_UNAVAILABLE` `IMPORT_ABORTED`
`CONFIG_NOT_FOUND` `PARAM_CHECK` `NO_BACKING` `COMMIT_FAILED`
