# Command → Envelope Contract Map

The single source of truth for the `command` field of every v1
envelope. QuickGUI's `_commandMatches` normalises on this table —
any change here is a contract break requiring a schemaVersion bump.

| CLI action | envelope `command` | Async op id in `meta` |
|---|---|---|
| --status | vm.status | — |
| --start (boot) | vm.started | — |
| --shutdown | vm.shutdown | — |
| --kill | vm.stopped | — |
| --reset | vm.reset | — |
| --pause | vm.paused | — |
| --resume | vm.resumed | — |
| --delete-plan | vm.plan | — |
| --delete-vm | vm.trashed (with --trash) / vm.deleted | — |
| --clone | vm.clone | operationId |
| --rename | vm.rename | — |
| --move | vm.move | operationId |
| --export | vm.export | operationId |
| --import | vm.import | operationId |
| --backup | vm.backup | operationId |
| --backup-restore | vm.backup-restore | operationId |
| --trash / --trash-list / --trash-restore / --trash-empty | vm.trash* | — |
| --disk-resize | vm.disk-resize | operationId |
| --disk-compact | vm.disk-compact | operationId |
| --snapshot apply/create/delete | vm.snapshot.* | — |
| --snapshot list/tree/verify | vm.snapshot-info / tree / verify | — |
| --hotplug-usb / --hotplug-disk / --hotunplug | vm.hotplug / vm.hotunplug | — |
| --vfio-scan | host.vfio | — |
| --host-info | host.info | — |
| --capabilities | host.capabilities | — |
| --op-status / --op-list / --op-cancel | vm.op-* | operationId |
| --events | vm.events | — |
| --migrate-conf | vm.migrate | — |
| --inspect-conf | vm.config | — |
| --doctor | vm.doctor | — |
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
