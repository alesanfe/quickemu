# Architecture

Quickemu's Go backend (`tools/`) — one module per tool, joined by
`go.work` with vendored deps for offline reproducible builds.

- [Target architecture](target-architecture.md) — repository layout and
  module responsibilities.
- [Public CLI contract](public-cli-contract.md) — the versioned v1
  surface external consumers rely on.
- [Go parity map](../go-parity.md) — former Bash subsystems → Go tools.
- [Go backend evaluation](../go-evaluation.md) — ISO/IEC 25010
  assessment with measured metrics.
- [ADR index](../adr/) — 15 decision records (0001 structured CLI …
  0015 backend language).

## At a glance

| Layer | Module | Role |
|---|---|---|
| entries | `bin/quickemu` `bin/quickget` `bin/quickgetc` `bin/quickreport` | compiled Go binaries, resolve siblings via `qelib.FindTool` |
| VM lifecycle | `quickemu-cli` `quickemu-launch` `quickemu-vm` | CLI dispatch, QEMU argv + spawn/reaper, filesystem ops |
| State | `quickemu-ops` `quickemu-ports` `quickemu-proc` | op journal, port registry, process identity |
| Control | `quickemu-qmp` `quickemu-api` | monitor channel, HTTP/unix API |
| Config | `quickemu-toml` `quickemu-archive` | .conf⇄TOML, export/import |
| Downloads | `quickget-catalog` `quickget-cli` `quickget-vm` `quickgetc` | catalog + scrapers, CLI, image fetch |
| Support | `qelib` `quickemu-plugin-market` `quickreport` `tools/qespec` `tools/dev` | shared helpers, templates, diagnostics, spec suite, task runner |
