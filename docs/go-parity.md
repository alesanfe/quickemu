# Bash ↔ Go parity (ADR-0001 phase E–G)

## Status by subsystem — verified via `tools/compare-backends.sh`

| Subsystem | Bash file | Go tool | Status |
|---|---|---|---|
| ops journal | `lib/quickemu/runtime/ops.sh` | `quickemu-ops` | ✅ **100%** — new/set/done/fail/status/watch/list/cancel/prune/**recover/rollback** |
| ports registry | port flock helpers | `quickemu-ports` | ✅ **100%** — alloc/free/check |
| proc fingerprint | `vm_proc_*` | `quickemu-proc` | ✅ **100%** — comm/start-time/uuid |
| qmp client | `lib/quickemu/runtime/monitor.sh` | `quickemu-qmp` | ✅ **100%** — query-status, safe commands |
| archive import | `lib/quickemu/vm/disk.sh` (import part) | `quickemu-archive` | ✅ **100%** — tar-slip validation + extract |
| toml config | `lib/quickemu/config/toml.sh` | `quickemu-toml` | ✅ **100%** — parse/validate |
| api daemon | `quickemu-api.sh` | `quickemu-api` | ✅ **100%** — state/ops/start/stop/pause/resume/kill/reset |
| **catalog** | `lib/quickget/catalog/*.sh` | `quickget-catalog` | 🟡 **~73%** — list/releases/editions/arch/trust ✅; url resolution ✅ for 113 static templates + 91 hash fetchers; **3 scrape-OSes delegate to Bash** |

## Go catalog detail

- `list`, `releases`, `editions`, `arch`, `trust` — fully resolved from `catalog/data/catalog.json` (165 entries)
- `url` — template substitution (`%RELEASE%/%EDITION%/%ARCH%`) + **live checksum fetch** via `net/http` — replaces `web_pipe` for the 113 template-based OSes
- `scrape: true` entries delegate to `bash -c 'source lib/...; get_<os>'` — documented delegation, not a gap

## Bash-only (launcher core — migration in progress)

`lib/quickemu/runtime/launch.sh` (891 LOC) + `lib/quickemu/vm/vm.sh` (1290 LOC) — QEMU arg building, display, network, storage, snapshot/spice. These are Bash-only today; `quickemu-api` already exposes the `start/stop/pause/resume` contract that the GUI uses, so the Go path exists at the API boundary even while the launcher internals remain Bash.

## Verification

```bash
tools/build-go.sh        # builds all 8 binaries
tools/run-go-tests.sh    # go test -race -cover per package
tools/compare-backends.sh  # zero divergences on extracted subsystems
```
