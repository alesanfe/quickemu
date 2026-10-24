# Public CLI contract (v1)

quickemu exposes a machine-readable interface for external consumers
(GUIs, scripts, CI). Stability rules:

- `--capabilities` prints the capability descriptor (JSON, schema v1).
  Consumers MUST branch on `capabilities`, not on `applicationVersion`.
- `--format json` switches `--status` and `--list-vms` output to the
  versioned JSON shapes declared in `schemas/`.
- `schemaVersion` is bumped only on breaking changes; additive fields
  keep the same version.
- Human output (`--format human`, the default) is **not** a stable API.

## Exit codes

| Code | Meaning |
|------|---------|
| 0 | Success (including "VM stopped" answers) |
| 1 | Generic failure: bad arguments, missing conf, missing tool, action failed |
| 2 | Reserved |

Actions that mutate (kill, snapshot, clone, disk-resize, export, import,
rename, send-keys, type, pause, resume, screenshot, doctor) return 0 on
success and 1 on failure, printing diagnostics on stderr/stdout in human
format.
