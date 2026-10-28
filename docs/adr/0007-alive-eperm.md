# ADR 0007: `Alive()` treats EPERM as alive

## Status: accepted

## Context
Lock files, port registries, op worker records and `.paused` markers
store a PID. Checking `kill(pid, 0)` naively treats a process owned by
another user as dead (EPERM ≠ ESRCH), which let locks be "reclaimed"
from live foreign owners and marked foreign workers as dead.

## Decision
`qelib.Alive(pid)` returns true when `kill` succeeds OR fails with
EPERM — only ESRCH (and pid ≤ 0) counts as dead. All liveness checks
route through it (`quickemu-ports`, `quickemu-launch` reaper,
`quickemu-ops` cancel/recover, `quickemu-cli` lock, `quickemu-api`
state).

## Alternatives
- `/proc/<pid>` existence — rejected: Linux-only, same EPERM blind
  spot on hidepid mounts.
- Comparing UIDs — rejected: races and less portable.

## Consequences
- (+) Locks cannot be stolen from processes owned by other users.
- (−) A foreign process can squat a lock/port we cannot verify or
  reap — the conservative direction: stale-lock recovery uses an age
  window, not liveness, for corrupt registrations.

## Review when
A per-VM owner model makes foreign-PID cases distinguishable.
