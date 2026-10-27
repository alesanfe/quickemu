# Contributing to quickemu

## Before you start

- Read `AGENTS.md` — project structure, test commands, conventions.
- Read `docs/adapter-contract.md` before adding an OS to quickget.
- Read `docs/error-codes.md` before adding error paths.

## Development setup

```bash
# Linux: bash 4.0+, jq, curl, socat, qemu-utils
# Verify your change:
bash -n <edited-file>
./shellspec-0.28.1/shellspec
shellcheck -S warning bin/* lib/**/*.sh
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
