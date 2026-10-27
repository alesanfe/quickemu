# Go backend parity inventory (QE-07)

Status legend: ✅ done · 🟡 partial · ⬜ missing · 🚫 blocked

| Bash capability | Go equivalent | Status | Notes |
|---|---|---|---|
| `--version` | `cmd.Version` | ✅ | emits contract version |
| `--list` | `cmd.List` | ✅ | os_support parity |
| `--check` | `cmd.Check` | ✅ | web_check + range fallback |
| `--download` | `cmd.Download` | 🟡 | core path, no zsync yet |
| `--create-config` | `cmd.Config` | 🟡 | basic fields |
| `vm_lock` mkdir-lock | `internal/lock` | ✅ | stale detection |
| `op_*` journal | `internal/ops` | ✅ | JSON state files |
| `qe_emit` JSON | `internal/json` | ✅ | v1 envelope + operationId |
| PID fingerprint | `internal/proc` | ✅ | comm=qemu check |
| `web_get`/`web_pipe`/`web_redirect` | `internal/download` | ✅ | incl. S3 range fallback |
| zsync resume | `internal/download` | ⬜ | planned |
| `port_forwards` validation | `internal/net` | 🟡 | parse only |
| `port allocation` | `internal/net` | 🟡 | ephemeral range |
| `import_vm` tar-slip | `internal/import` | ⬜ | planned |
| `export_vm` | `internal/export` | ⬜ | planned |
| `delete_vm` confinement | `internal/delete` | ✅ | realpath check |
| `vm_monitor_socket_ready` | `internal/qmp` | ✅ | socket validation |
| `qmp_command` allowlist | `internal/qmp` | ✅ | same denylist |
| `--minimum-trust` | `internal/trust` | ⬜ | pending |
| `arch_<os>` | `catalog/arch` | ✅ | default amd64 |
| `editions_<os>` | `catalog/editions` | 🟡 | subset |
| `releases_<os>` | `catalog/releases` | 🟡 | subset |
| `os_info` | `catalog/info` | 🟡 | subset |
| `windows` (unattended) | `catalog/windows` | ⬜ | not started |
| `macos` (recovery) | `catalog/macos` | ⬜ | not started |
| `wsl` | `catalog/wsl` | ⬜ | not started |
| `plan`/`execute` planning | `internal/plan` | 🟡 | delete_plan done |
| `rename`/`clone` | `internal/vm` | 🟡 | basic |
| `compact` | `internal/vm` | ⬜ | not started |
| `snapshot` | `internal/vm` | ⬜ | not started |
| `spice` | `internal/display` | ⬜ | not started |
| `--format json` | global flag | ✅ | schemaVersioned |
| `--dry-run` | global flag | 🟡 | some ops |
| `--non-interactive` | global flag | 🟡 | some ops |

## Go-only advantages (not in Bash)

- Typed `OperationId`/`PlanId`/`Port`/`TrustLevel` domains
- Race detector in `go test -race`
- Coverage per package
- Concurrent catalog check

## Pending for parity

- Full editions/releases coverage (currently subset)
- zsync + resume
- import/export
- compact/snapshot/spice
- windows/macos/wsl catalog entries
- Golden tests Bash ↔ Go (QE-07.2)
