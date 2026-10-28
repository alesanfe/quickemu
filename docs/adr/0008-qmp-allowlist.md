# ADR 0008: QMP command allowlist with explicit raw escape hatch

## Status: accepted (moved here from ADR-0001 phase docs; structural
matching supersedes the original substring match — see below)

## Context
`quickemu-qmp` is the single process boundary between user-facing
tools and QEMU monitors. Unrestricted QMP (`system_reset`, `quit`,
`blockdev-*`, `chardev-*`) lets any caller destroy a guest or mutate
devices; unrestricted `human-monitor-command` does the same via HMP.

## Decision
`execute` is fail-closed to a command-name allowlist (`query-*`,
`screendump`, `system_powerdown`, `system_wakeup`, `stop`, `cont`,
`block-dirty-bitmap`, `drive-backup`, `human-monitor-command` — the
Bash parity set). The allowlist matches on the **parsed command name**,
not substring inclusion in the raw payload: a name that merely
contains an allowed substring is denied. `QUICKEMU_QMP_RAW=1` is the
explicit escape hatch for raw commands.

## Alternatives
- Full allowlist of exact spellings — adopted for name matching;
  the `query-*` family remains a prefix because QEMU adds query
  commands per release.
- Per-socket policy files — rejected: overkill vs one env hatch.

## Consequences
- (+) Injection via crafted command strings is structurally excluded.
- (+) Power users keep full access via an explicit, grep-able env var.
- (−) New QEMU commands need allowlist updates (same as Bash did).

## Review when
HMP passthrough (`human-monitor-command`) needs argument-level
filtering rather than blanket inclusion.
