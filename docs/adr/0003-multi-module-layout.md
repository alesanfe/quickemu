# ADR 0003: Multi-module Go layout (19 go.mod files)

## Status: accepted

## Context
The Bash backend was extracted into ~20 cooperating binaries. A single
`go.mod` for all of `tools/` would couple every tool to every
dependency (e.g. `quickget-vm`'s ISO9660/7z chain vs `quickemu-toml`'s
single TOML dep), inflate each binary's SBOM, and blur which tool owns
which capability.

## Decision
One `go.mod` per `tools/<name>` directory. Shared code lives in
`tools/qelib` and is consumed through a `replace ../qelib` directive so
every module also builds standalone (`GOWORK=off go build`).

## Alternatives
- Single module — rejected: cross-tool dependency leakage, one big SBOM.
- Internal packages under one module — rejected: same coupling problem.

## Consequences
- (+) Each binary's SBOM lists only its own dependency closure.
- (+) Modules can be vendored/packaged independently.
- (−) `replace` paths are relative; a `go.work` (ADR 0004) is needed
  for ergonomic root-level builds.
- (−) `go test ./...` cannot enumerate modules from the root; the
  `qedev` runner iterates `tools/*/go.mod` instead.

## Review when
A module is published outside the repo, or Go workspaces gain a
supported way to nest single-command builds without module paths.
