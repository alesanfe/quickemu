# ADR 0014: keep gopsutil + procfs — they do not duplicate

## Status: accepted (measured review)

## Context
Two process/host libraries entered the tree and looked redundant:
`github.com/shirou/gopsutil/v4` (~8 indirect deps) and
`github.com/prometheus/procfs`.

## Measured usage
| Dep | Call sites | Purpose |
|---|---|---|
| gopsutil/v4/mem | `qelib.HostFreeMemMB` | `mem.VirtualMemory().Available` — works on macOS/BSD, not only Linux `/proc/meminfo` |
| gopsutil/v4/process | `quickemu-proc` | `CreateTime()` — portable process start-time fallback for non-Linux |
| prometheus/procfs | `quickemu-proc` | `NewProc(pid).Stat().Starttime` — correct `/proc/<pid>/stat` parsing when `comm` contains `)` or spaces |

No site uses both libs for the same datum; responsibilities partition
cleanly (memory vs process-stat, Linux-precise vs portable-fallback).

## Decision
Keep both. Alternatives measured against:

- **Drop gopsutil** → HostFreeMemMB becomes `/proc/meminfo` on Linux +
  `vm_stat`/sysctl on Darwin + `sysctl` on BSD — ~3 platform-specific
  implementations to own for one number; `CreateTime` on Darwin is a
  sysctl dance. More code, more platform risk, smaller SBOM.
- **Drop procfs** → hand-parse `/proc/<pid>/stat`; `comm` is wrapped
  in parens and may itself contain `)` and spaces — the naive
  `strings.LastIndex(")")` is right *today* but the parser is
  security-adjacent (a wrong starttime lets a reused PID be signalled).
  procfs is tiny and handles the edge cases.
- **Drop both** → reimplement two parsers with known traps. Rejected.

## Consequences
- (+) PID-reuse verification stays correct across `comm` edge cases.
- (+) macOS/BSD memory + start-time paths exist without #ifdef code.
- (−) ~9 indirect deps in vendor/; accepted, they are leaf libs with
  no runtime surface beyond what we call.
- Domain code sees no third-party types (qelib returns `int`,
  quickemu-proc returns ints/strings in the JSON data map).

## Review when
Either dep is abandoned upstream, a CVE lands in the chain, or a
stdlib `x/sys` API covers the same reads portably.
