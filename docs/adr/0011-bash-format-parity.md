# ADR 0011: Bash↔Go parity is defined by on-disk formats, not internals

## Status: accepted

## Context
"Same behavior as Bash" is untestable at the implementation level —
Go internals necessarily differ. What must be identical is every
observable surface: `.conf` syntax, TOML key order, `ops/*.json`,
`snapshots.json`, `runtime.json`, `vms.json`, port registry records,
`.paused`/`.pid`/socket names, JSON envelope `schemas/v1`, exit codes,
and human `ERROR! …` strings.

## Decision
Parity tests assert on artifacts and contract fields, never on
implementation similarity. Where formats embed quoting rules
(bash `a+=("x")` vs TOML `a = ["x"]`), `qelib.HotplugPersist` owns
per-format escaping so both files round-trip identical values.

## Alternatives
- Line-for-line port — rejected: preserves Bash bugs and grep soup.
- New formats + migration — rejected: destroys interop with existing
  VMs and QuickGUI.

## Consequences
- (+) A VM dir is portable between implementations mid-flight.
- (+) Parity is falsifiable: `qespec` contract tests enumerate it.
- (−) Format quirks (bash-array syntax, `disk_img="path"` keys) must
  be preserved even when they look odd in Go.

## Review when
`schemas/v2` replaces the envelope or the on-disk schema.
