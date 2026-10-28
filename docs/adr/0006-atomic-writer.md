# ADR 0006: Atomic writes via renameio (temp + fsync + rename)

## Status: accepted

## Context
Op records, snapshot metadata and runtime files are read by other
processes at any time. A truncated `json.Marshal` → `os.WriteFile`
during a crash leaves a corrupt file that readers then surface as
`OP_CORRUPT` or silently reset (the `snapMetaRead` data-loss bug).

## Decision
`Op.save` uses `renameio.TempFile` + `f.Sync()` +
`CloseAtomicallyReplace`: the new content lands in a staging file on
the same filesystem, is fsynced, then atomically renamed over the old
path. `snapMetaWrite` follows the same tmp+rename pattern. Because
`die()` calls `os.Exit`, temp files are cleaned explicitly on each
failure branch rather than relying on defer.

## Alternatives
- Plain WriteFile — rejected: torn writes on crash.
- Write-then-fsync-directory only — rejected: still non-atomic content.

## Consequences
- (+) Readers never observe a partially-written file.
- (+) Durability survives process kill between write and rename.
- (−) Small extra latency per save — irrelevant at ops frequency.

## Review when
Write volume grows enough for batching/journaling to matter.
