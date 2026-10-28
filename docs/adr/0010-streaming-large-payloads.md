# ADR 0010: Streaming is mandatory for VM-size payloads

## Status: accepted

## Context
VM disks are routinely multiple GB. `os.ReadFile`/`io.ReadAll` on
export members, downloaded ISOs, tar verification and backup checks
buffered whole payloads in memory — an OOM on real VMs, not just in
theory.

## Decision
Any code path that can touch disk images, ISOs or archives must hash
and copy via `io.Copy`/`io.MultiWriter`/`sha256` streaming. Bounded
`io.LimitReader` reads are only acceptable for metadata with a defined
cap (manifest, checksum file — 1 MiB).

## Alternatives
- Per-member ReadAll — rejected: this was the bug.
- mmap — rejected: needless complexity for one-pass streaming.

## Consequences
- (+) Memory use is O(1) vs payload size; GBs move through small
  buffers.
- (+) Hashing and writing share one pass (`io.MultiWriter`).
- (−) Verify-and-extract needs a second pass when order matters —
  acceptable.

## Review when
Content-defined chunking/dedup makes a second read worthwhile.
