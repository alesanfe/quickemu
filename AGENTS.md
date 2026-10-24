# AGENTS.md

Quickemu is a Bash wrapper around QEMU that automates VM creation for ~1000 operating systems (Linux, macOS, Windows, BSDs). Two main tools: `quickget` downloads ISOs and creates configs; `quickemu` launches VMs with optimised hardware detection.

## Project structure

```
bin/quickemu      # Main VM launcher (canonical entry)
bin/quickget      # OS downloader and config generator
bin/quickgetc     # Data-driven downloader (JSON catalog variant, dev tool)
bin/quickreport   # System diagnostics tool
lib/quickemu/     # Modules sourced by the entries (cli, config, catalog,
                  # host, planning, runtime, vm, adapters, support)
lib/quickget/     # quickget layers (cli, catalog, download, image, vm, support)
lib/quickgetc/    # quickgetc layers (same layout as lib/quickget)
lib/quickreport/  # quickreport layers (host probes + cli report)
libexec/quickemu/chunkcheck.py  # Download verification utility
schemas/v1/       # Versioned JSON CLI contract (--format json)
catalog/          # OS catalogue data (schema today, entries phased in)
spec/             # ShellSpec suite (unit, integration, contract)
probes/           # External-source checks (nightly, not PR-blocking)
flake.nix         # Nix flake for packaging
packaging/nix/package.nix  # Nix package definition
devshell.nix      # Development environment
```

## Development environment

```shell
nix develop                    # Enter devshell with all dependencies
direnv reload                  # Update .direnv/bin/quickemu for testing
```

The devshell patches `quickemu` to use Nix store paths for OVMF and Samba, writing to `.direnv/bin/quickemu`.

## Build and test commands

- Lint scripts: `shellcheck -S warning bin/quickemu bin/quickget bin/quickgetc bin/quickreport $(find lib -name '*.sh')`
- Test quickget URLs: `./bin/quickget --check <os> [release] [edition]`
- Test ARM64 downloads: `./bin/quickget --arch arm64 --check <os> [release] [edition]`
- Test all architectures: `./bin/quickget --check-all-arch <os> [release] [edition]`
- List all supported OSes: `./bin/quickget --list`
- Run VM: `./bin/quickemu --vm <name>.conf`

## Code style

All scripts are Bash 4.0+ with ShellCheck compliance.

| File | Indent | Notes |
|------|--------|-------|
| bin/quickemu | 2 spaces | Main VM launcher |
| bin/quickget | 4 spaces | OS definitions and download logic |

**General conventions:**

- UTF-8, LF line endings, final newline
- Trim trailing whitespace (except Markdown)
- Functions use `function name() {` syntax
- Many functions are invoked indirectly via dynamic dispatch - `SC2317` is disabled globally in quickget

## Adding a new OS to quickget

Follow the [guide in the wiki](https://github.com/quickemu-project/quickemu/wiki/06-Advanced-quickget-features#adding-a-new-os-to-quickget). Each OS requires:

1. Entry in `os_info()` case statement
2. `releases_<os>()` function returning available versions
3. `editions_<os>()` function if multiple editions exist
4. `arch_<os>()` function if ARM64 is supported (defaults to amd64 only if omitted)
5. Download URL construction logic

## Commit message format

Commits must follow [Conventional Commits](https://www.conventionalcommits.org/). PR titles are validated against this format.

**Common prefixes:**

- `fix(quickget):` - Fix distro downloads, update editions/releases
- `fix(quickemu):` - Fix VM configuration or runtime issues
- `feat(quickget):` - Add new OS support
- `feat(quickemu):` - Add new VM features
- `ci:` - CI/workflow changes
- `docs:` - Documentation updates

**Examples from recent history:**

```
fix(quickget): update Zorin OS download method and editions
fix(quickget): handle Solus Xfce beta naming for older releases
fix(quickget): remove Athena OS (no longer getting updates)
```

## Pull request guidelines

- PR titles must match Conventional Commits format (enforced by CI)
- Single-commit PRs: commit message must match PR title
- Run `shellcheck` before submitting
- Test affected OS downloads with `./bin/quickget --check <os> <release>`

## CI workflows

| Workflow | Trigger | Purpose |
|----------|---------|---------|
| lint-shellcheck | PR | ShellCheck with severity: warning |
| lint-pr | PR | Validates PR title matches Conventional Commits |
| test-quickget | PR | Tests quickget functionality |
| test-build-quickemu | PR | Build verification |
| flake-checker | PR | Nix flake validation |

## Platform support

- Host: Linux (x86_64, aarch64), macOS (x86_64, aarch64)
- Guest: x86_64 (default), aarch64 (set `arch="aarch64"` in VM config)
- ARM64 guests use AAVMF firmware and `virt` machine type
- Cross-arch emulation uses TCG (no KVM acceleration)
- OVMF/UEFI firmware: Linux only
- Bash 4.0+ required (explicit version check at runtime)

## Key dependencies

Runtime: qemu, cdrtools, curl, jq, spice-gtk, swtpm, samba, zsync
Linux-specific: OVMF (x86_64 guests), AAVMF (aarch64 guests), usbutils, mesa-demos
