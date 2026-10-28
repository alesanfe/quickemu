# ADR 0009: Test seams as package-level function variables

## Status: accepted

## Context
Tools shell out to QEMU, sibling binaries, git, systemctl and the
network. Testing through real subprocesses is non-deterministic; a DI
framework or interface-heavy design for a handful of call sites is
disproportionate.

## Decision
Subprocess and network touch-points are declared as package-level
function variables — `var execCommand = exec.Command`, `var httpFetch`,
`var fetch` — swapped by tests inside a subprocess helper or with a
stub, and always restored via `t.Cleanup`. Important: `run()` must not
clobber `cmd.Env` when the seam already set one (the stub-env bug).

## Alternatives
- Interfaces + injected structs — rejected for these small tools;
  revisited if a package grows >5 seams.
- exec.CommandContext everywhere — only where timeouts are real.

## Consequences
- (+) Deterministic tests without a mocking library.
- (+) Seams document the exact externality each function owns.
- (−) Mutable globals — must not be combined with `t.Parallel()`
  inside the same package; covered by the seam guide in CONTRIBUTING.

## Review when
A package accumulates enough seams that a deps struct is cheaper to
read than N global vars.
