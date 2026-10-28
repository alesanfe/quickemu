# AGENTS.md

Quickemu is a Go wrapper around QEMU that automates VM creation for ~1000 operating systems (Linux, macOS, Windows, BSDs). Two main tools: `quickget` downloads ISOs and creates configs; `quickemu` launches VMs with optimised hardware detection.

## Project structure

```
bin/quickemu      # VM launcher entrypoint → tools/quickemu-cli
bin/quickget      # OS downloader entrypoint → tools/quickget-cli
bin/quickgetc     # Data-driven downloader → tools/quickgetc
bin/quickreport   # Diagnostics entrypoint → tools/quickreport
                  # (bin/ entries ARE the Go binaries; siblings resolve
                  # via qelib.FindTool: libexec/ and dev-tree tools/*/)
tools/            # Go modules — one dir per tool
  qelib           #   shared helpers: v1 envelope, NormArgs, Alive,
                  #   FindTool/Tool sibling resolution
  quickemu-cli    #   unified CLI: lifecycle, flags, JSON envelopes
  quickemu-launch #   QEMU arg builder + spawn/reaper (the old vm_boot)
  quickemu-vm     #   VM ops: delete/export/clone/snapshots/create-disk
  quickemu-ops    #   op journal: new/set/done/fail/status/watch/cancel
  quickemu-qmp    #   QMP/HMP client, allowlist, socket defenses
  quickemu-ports  #   port registry: reserve/release/list/check/find
  quickemu-proc   #   process identity + comm checks
  quickemu-archive#   export/import with tar-slip validation
  quickemu-toml   #   .conf ⇄ TOML parse/emit
  quickemu-api    #   v1 HTTP/unix-socket API daemon
  quickget-catalog#   catalog data + URL resolution + scrapers
  quickget-cli    #   quickget CLI dispatch (list/check/get/config)
  quickget-vm     #   image fetch/verify + conf generation + mkiso
  quickgetc       #   data-driven downloader (quickgetc entrypoint)
  quickreport     #   host diagnostics (the quickreport entrypoint)
go.work           # Workspace joining all modules
vendor/           # `go work vendor` output — offline reproducible builds
schemas/v1/       # Versioned JSON CLI contract (--format json)
catalog/          # OS catalogue data — catalog/data/catalog.json feeds
                  # tools/quickget-catalog; releases/editions left empty
                  # for dynamic OSes are resolved live via scrape.go
spec/             # ShellSpec suite (contract, functional, e2e, security)
probes/           # External-source checks (nightly, not PR-blocking)
flake.nix         # Nix flake for packaging
packaging/nix/package.nix  # Nix package definition
packaging/debian/          # Debian packaging (install, rules, control)
devshell.nix      # Development environment
```

## Development environment

```shell
nix develop                    # Enter devshell with all dependencies
direnv reload                  # Update .direnv/bin/* for testing
go run ./tools/dev build       # Build all Go tool binaries (entries → bin/)
```

The devshell exports `QE_FIRMWARE_DIR` so the Go launcher finds
nix-store OVMF/AAVMF firmware without patching sources.

## Build and test commands

- Build Go tools: `go run ./tools/dev build` (required before bin/ entries exist)
- Full Go gate (vet+test+race+coverage+build, all modules): `go run ./tools/dev check`
- Lint scripts: `shellcheck -S warning tools/*.sh`
- Run tests (ShellSpec 0.28.1 is vendored): `./shellspec-0.28.1/shellspec [spec/file.sh]` — needs `qemu-img`, `qemu-system-*`, `socat` and `python3` for full coverage; contract/quickget/quickgetc specs are hermetic via `QUICKGETC_CATALOG` overrides.
- TOML hostile-corpus: `go test ./tools/quickemu-toml` (was tools/fuzz-toml.sh)
- Test quickget URLs: `./bin/quickget --check <os> [release] [edition]`
- Test ARM64 downloads: `./bin/quickget --arch arm64 --check <os> [release] [edition]`
- Test all architectures: `./bin/quickget --check-all-arch <os> [release] [edition]`
- List all supported OSes: `./bin/quickget --list`
- Run VM: `./bin/quickemu --vm <name>.conf`

## Code style

Go (gofmt, `go vet` clean); the thin bin/ wrappers and tools/*.sh are
Bash 4.0+ with ShellCheck compliance.

**General conventions:**

- UTF-8, LF line endings, final newline
- Trim trailing whitespace (except Markdown)
- Go tools emit the v1 JSON envelope (`schemas/v1/`) for `--format json`
- User-facing errors go to **stdout** (historical contract); only
  integrity/trust warnings go to stderr

## Adding a new OS to quickget

Follow the [guide in the wiki](https://github.com/quickemu-project/quickemu/wiki/06-Advanced-quickget-features#adding-a-new-os-to-quickget). Each OS requires:

1. An entry in `catalog/data/catalog.json` (releases, editions, arch,
   url/hash templates) — or `"scrape": true` plus a case in
   `tools/quickget-catalog/scrape.go` for dynamic resolution
2. An `os_info` row (tools/quickget-catalog/info.go data)
3. Guest-OS quirks in `tools/quickget-vm` (guestFor/osTweaks) if it
   needs non-default hardware or post-download fixups

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
- Run `go run ./tools/dev check` and `shellcheck` before submitting
- Test affected OS downloads with `./bin/quickget --check <os> <release>`

## CI workflows

| Workflow | Trigger | Purpose |
|----------|---------|---------|
| go | PR | `go-check.sh` gate (vet, tests, race, build) |
| shellspec-suite | PR | Contract/functional/e2e specs vs Go binaries |
| lint-shellcheck | PR | ShellCheck on wrappers + tools/*.sh |
| lint-pr | PR | Validates PR title matches Conventional Commits |
| test-quickget | PR | Per-distro download checks via `bin/quickget` |
| test-build-quickemu | PR | .deb + .nix build verification |
| coverage | PR/push | `go test -cover` per module |
| flake-checker | PR | Nix flake validation |

## Platform support

- Host: Linux (x86_64, aarch64), macOS (x86_64, aarch64)
- Guest: x86_64 (default), aarch64 (set `arch="aarch64"` in VM config)
- ARM64 guests use AAVMF firmware and `virt` machine type
- Cross-arch emulation uses TCG (no KVM acceleration)
- OVMF/UEFI firmware: Linux only (`QE_FIRMWARE_DIR` overrides the
  distro search paths)

## Key dependencies

Runtime (exec'd by the Go tools): qemu-system-*, qemu-img, swtpm,
virtiofsd, samba (smbd via QEMU netdev), spice-gtk, zsync, xrandr,
usbutils, util-linux. Spec tooling only: socat (QMP socket probing),
jq (schema assertions).
Firmware: OVMF (x86_64), AAVMF (aarch64) — `QE_FIRMWARE_DIR` override
Linux-specific: usbutils, mesa-demos
