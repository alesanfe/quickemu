# ADR 0004: `go.work` at the repository root

## Status: accepted

## Context
With 19 modules (ADR 0003) there was no single command to vet, test or
build the tree; developers had to loop directories by hand.

## Decision
Commit `go.work` listing all `tools/*` modules. `go.work.sum` is also
committed so workspace-mode builds resolve identically in CI. Each
module keeps its `replace ../qelib` so `GOWORK=off` standalone builds
keep working — the workspace is a development convenience, not a build
requirement.

## Alternatives
- Taskfile loops only — kept as well, but slower and easy to drift.
- Git-ignoring go.work — rejected: CI could not validate the workspace.

## Consequences
- (+) `go vet/test/build <module-path>/...` works from the root.
- (+) Shared dependency versions unify across modules.
- (−) `go work sync` must be re-run when a module's deps change;
  `qedev quality` warns on drift via the standard build failure path.

## Review when
Modules get extracted to their own repositories.
