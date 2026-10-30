# Stable Error Codes (JSON contract v1)

All structured-error output uses one of these codes in `error.code`. Codes
are stable within the contract version — do not reuse or rename them without
a schema bump.

| Code | Meaning | Retryable |
|---|---|---|
| `LOCK_BUSY` | another operation holds the VM lock | yes |
| `LOCK_TIMEOUT` | waited for the lock past the deadline | yes |
| `PLAN_EXPIRED` | the execution plan's deadline passed | no — replan |
| `PLAN_MISMATCH` | plan arguments differ from current state | no — replan |
| `OPERATION_NOT_FOUND` | no journal entry for the requested operation | yes — replan |
| `OPERATION_INTERRUPTED` | op worker died mid-operation | inspect `op-* status` |
| `OPERATION_ROLLBACK` | op failed and was rolled back | no — retry the op |
| `OP_CORRUPT` | journal entry unreadable | no — manual cleanup |
| `OP_FINISHED` | op already reached a terminal state | no |
| `OP_PERSIST` | journal write failed | retry |
| `PORT_BUSY` | requested port already reserved/in use | yes — another port |
| `PORT_BOUND` | socket bind failed on reservation | yes |
| `PORT_TAKEN` | port claimed by another VM | yes — another port |
| `PORT_RANGE_EXHAUSTED` | no free port in the configured range | no — widen range |
| `PORT_PERSIST` | port registry write failed | retry |
| `PLAN_*` family | see delete/export transactional ops | replan |
| `VM_MUST_BE_STOPPED` | action requires a stopped VM | no — stop first |
| `NOT_RUNNING` | action requires a running VM | no — start first |
| `NOT_FOUND` | VM/conf/resource not found | no |
| `NO_CONF` | `--vm` conf missing | no |
| `NO_DISK` | referenced disk image missing | no |
| `NO_QEMU_IMG` | `qemu-img` not on PATH | no — install it |
| `PERMISSION_DENIED` | filesystem/os denied the action | fix perms then retry |
| `CHECKSUM_MISMATCH` | downloaded image failed integrity check | retry or re-resolve |
| `VERIFY_FAILED` | generic verification failure | retry |
| `IMPORT_CONFLICT` | import target already exists | rename or remove first |
| `IMPORT_FAILED` | import pipeline failed | retry |
| `UNSAFE_ARCHIVE` | archive failed member policy | no — hostile archive |
| `TAR_SLIP` | archive member escapes target dir | no — hostile archive |
| `ARCHIVE_BAD` | malformed archive | no |
| `ARCHIVE_BOMB` | archive exceeds bomb limits | no |
| `ARCHIVE_LINK` | archive contains link members | no |
| `MANIFEST_MISSING` | export manifest absent | no |
| `MANIFEST_DUP` | duplicate manifest entry | no |
| `EXTRACT_FAIL` | archive extraction failed | retry |
| `EXPORT_FAILED` | export pipeline failed | retry |
| `DELETE_FAILED` | delete/trash op failed | retry |
| `RESTORE_FAILED` | trash restore failed | retry |
| `TRASH_FAILED` | trash op failed | retry |
| `CLONE_FAILED` | clone op failed | retry |
| `RENAME_FAILED` | rename op failed | retry |
| `MOVE_FAILED` | move op failed | retry |
| `BACKUP_FAILED` | backup op failed | retry |
| `BACKUP_KEEP` | retained-backup policy rejected | no |
| `SNAPSHOT_CONFLICT` | snapshot name/state conflict | resolve first |
| `CONSOLIDATE_FAILED` | snapshot consolidate failed | retry |
| `AUTOSTART_FAILED` | autostart install failed | retry |
| `SHORTCUT_FAILED` | desktop shortcut creation failed | retry |
| `TOML_PARSE` | .toml unparseable | fix syntax |
| `UNKNOWN_PRESET` | preset name not in registry | check `--list-presets` |
| `UNKNOWN_PROFILE` | profile name not in registry | check profile list |
| `BAD_ARGS` / `INVALID_ARGS` | malformed CLI arguments | fix and retry |
| `AUTH_THROTTLED` | API rate limit hit | yes — back off |
| `UNAUTHORIZED` | API token missing/invalid | provide token |
| `WRITE_DISABLED` | API write endpoint not enabled | enable writes |
| `METHOD` | HTTP method not allowed | use documented verb |
| `QMP_TIMEOUT` | monitor command timed out | retry |
| `MINIMUM_TRUST` / `QC_TRUST` | source below `--minimum-trust` | pick trusted source |

Human-format `ERROR!`/`WARNING!` lines are diagnostics, not contract —
grep them for detail, branch on `errors[].code` for logic.
