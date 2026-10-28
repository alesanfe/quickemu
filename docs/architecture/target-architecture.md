# Architecture (current — migration complete)

Canonical entries: `bin/quickemu`, `bin/quickget`, `bin/quickgetc`,
`bin/quickreport` — thin wrappers that exec the Go backend in
`tools/`. The former Bash implementation (`lib/`, `libexec/`,
`bin/*.bash`) has been removed.

Layout:

- `bin/` — canonical entries (bash wrappers → Go binaries; resolve
  `libexec/quickemu/` in packages and `tools/*/` in dev trees).
- `tools/` — Go modules, one dir per tool: `quickemu-cli` (unified
  CLI), `quickemu-launch` (QEMU argv + spawn/reaper), `quickemu-vm`
  (lifecycle/filesystem ops), `quickemu-ops` (op journal),
  `quickemu-qmp` (QMP/HMP), `quickemu-ports` (port registry),
  `quickemu-proc` (process identity), `quickemu-archive`,
  `quickemu-toml`, `quickemu-api` (HTTP daemon), `quickget-catalog`
  (data + scrapers), `quickget-cli`, `quickget-vm` (fetch/mkconf),
  `quickgetc`, `quickreport`.
- `go.work` + `vendor/` — workspace vendoring; builds are offline and
  reproducible (`GOFLAGS=-mod=vendor`).
- `catalog/` — OS data: `data/catalog.json` (static entries) +
  `tools/quickget-catalog/scrape.go` (dynamic resolution).
- `schemas/v1/` — versioned public contract.
- `probes/` — external-source checks (nightly, not PR-blocking).
- `spec/` — ShellSpec taxonomy (contract/functional/e2e/security
  against the Go binaries; Bash-internal unit specs retired — their
  behaviors live in Go `*_test.go` files).
- `packaging/{nix,debian}` — build the 15 tools and install them to
  `libexec/quickemu/`.
