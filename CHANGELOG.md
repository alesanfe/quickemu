# Changelog

All notable changes to this fork are documented here. The project follows
[Conventional Commits](https://www.conventionalcommits.org/).

## [Unreleased]

### Added

- feat(quickemu): add VM management actions and runtime options
- feat(quickemu): add --pause, --resume, --screenshot, --clone and --snapshot list
- feat(quickget): add 12 OS integrations
- feat(quickemu): add --status flag reporting real VM state
- feat(quickget): add Redox OS support (#1076)
- feat(quickget): add DragonOS support (#1344)
- feat(quickemu): add gpu_passthrough config option for VFIO devices (#900)
- feat(quickget): add Qubes OS and openFyde support
- feat(quickget): add Omarchy Linux support (from closed PR #1947)
- feat: passt networking, TempleOS and GoboLinux support
- feat(quickemu): provide IP for listening SSH port
- feat(quickemu): add support for passing extra arguments to the SPICE viewer
- feat(nix-flake): add virtiofsd for linux
- feat: stop virtiofsd gracefully when VM is killed or fails to start
- feat: add virtiofs support for Linux guests
- feat: add support for KDE Linux


### Fixed

- fix(quickget): fall back to kernel UUID when uuidgen is missing
- fix(quickget): resolve Ubuntu daily-live images in per-codename directories
- fix(debian): depend on qemu-system-x86 instead of the qemu-system metapackage (#1910)
- fix(quickemu): more upstream issue fixes
- fix: assorted upstream issue fixes
- fix(quickget): preserve release order in --list-csv output
- fix(quickget): use w11 VirtIO driver paths for Windows 11 unattend (#1958)
- fix: honor --fullscreen for the GTK display backend
- fix: remove unnecessary echo, flesh out echo
- fix: fix `chown` syntax (`root:user` to `user:root`)
- fix: fix windows iso downloading
- fix(quickemu): select the first complete firmware pair
- fix(quickemu): pair QEMU firmware with correct vars
- fix(quickemu): defer guest QEMU lookup
- fix(quickemu): output IP in SPICE cmd only if --access used with IP
- fix: add host address to spice command in output
- fix: Modify suggested fix to use similar checks to existing ones in quickemu
- fix(quickget): support new kde neon directory structure with editions
- fix(quickget): emit curl progress and size to stdout when piped
- fix(virtiofsd): writing inside guest
- fix(test-nix-build): update nixpkgs
- fix(quickemu): fix unattended.iso mount for windows-server guest
- fix(quickemu): fix hard coded disk format for SystemDisk
- fix: escape socket path regex metacharacters in pgrep fallback
- fix: address remaining cubic review issues — pgrep scope, /proc portability, QEMU device check
- fix: address code review issues from cubic-dev-ai
- fix: correct virtiofsd lifecycle — forking, iso check ordering, and fallback
- fix: skip virtiofs during OS installation to prevent installer hangs
- fix: remove stale virtiofsd socket before starting daemon
- fix: use standalone virtiofsd only; improve failure diagnostics
- fix: specify helper to qemu


### Other

- docs: add gpu_passthrough section to quickemu_conf.5
- style: fix shellcheck warnings from merged upstream PRs
- Merge branch 'pr-1421'
- Merge branch 'pr-1905'
- Merge branch 'pr-1938'
- Merge branch 'pr-1950'
- Merge branch 'pr-1321'
- Merge branch 'pr-1780'
- Merge branch 'pr-1860'
- Merge branch 'pr-1893'
- Merge branch 'pr-1896'
- Merge branch 'pr-1898'
- Merge branch 'pr-1919'
- Merge branch 'pr-1922'
- Merge branch 'pr-1926'
- Merge branch 'pr-1928'
- Merge branch 'pr-1929'
- Merge branch 'pr-1931'
- Merge branch 'pr-1933'
- Merge branch 'pr-1934'
- Merge branch 'pr-1935'
- Merge branch 'pr-1937'
- Merge branch 'pr-1940'
- Merge branch 'pr-1942'
- Merge branch 'pr-1949'
