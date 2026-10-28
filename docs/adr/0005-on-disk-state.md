# ADR 0005: On-disk JSON state instead of a database

## Status: accepted

## Context
Operations, runtime descriptors, port allocations and snapshot
metadata need shared state between short-lived CLI invocations and the
persistent API daemon. A database (sqlite/bbolt) would work, but the
Bash backend already defined stable on-disk formats
(`<vmdir>/ops/*.json`, `runtime.json`, `vms.json`, `*.ports`,
`snapshots.json`).

## Decision
Keep the Bash-defined file formats exactly. Files are the IPC
contract: Bash-era tooling, the Go tools and QuickGUI can all read
them, and `ops/*.json` doubles as the audit trail.

## Alternatives
- sqlite — rejected: breaks format parity, adds a cgo/complex dep.
- In-memory daemon state — rejected: CLI invocations are one-shot.

## Consequences
- (+) Zero-dependency introspection (`cat ops/*.json` is debugging).
- (+) Crash recovery = file presence + worker liveness (`recover`).
- (−) No multi-file transactions: torn state is handled by atomic
  single-file writes (ADR 0006) and idempotent `recover`/`prune`.

## Review when
A consumer needs multi-object transactions or concurrent writes
beyond what single-file atomicity covers.
