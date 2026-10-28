# Contributing to quickemu

## Before you start

- Read `AGENTS.md` — project structure, test commands, conventions.
- Read `docs/adapter-contract.md` before adding an OS to quickget.
- Read `docs/error-codes.md` before adding error paths.

## Development setup

```bash
# Linux: go 1.25+, qemu-utils, qemu-system-x86 (for e2e)
# Verify your change:
go run ./tools/dev check          # vet + test + build, all modules
go run ./tools/dev build          # rebuild bin/ entries
go run ./tools/dev specs          # qespec suite (contract + e2e)
```

## Commits

Conventional Commits — CI validates PR titles:

- `feat(quickget):` new OS / releases / editions
- `fix(quickget):` catalog fix  
- `feat(quickemu):` runtime feature
- `fix(quickemu):` runtime fix
- `docs:` / `ci:` / `test:` / `refactor:` / `chore:`

## Adding an OS

1. Follow `docs/adapter-contract.md` exactly
2. Verify `quickget --check <os> <release> <edition>` on real Linux
3. No torrent-only, appliance-only, or JS-wrapped download URLs

## Reporting bugs

Use the issue template — include commit hash, host env, repro commands,
and `--verbose` output or JSON envelope.
