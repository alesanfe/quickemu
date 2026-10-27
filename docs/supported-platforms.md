# Supported Platforms Matrix

## Host — Level 1 (supported, CI-tested)

| OS | Arch | Notes |
|---|---|---|
| Ubuntu 24.04 LTS | x86_64 | primary CI runner |
| Debian 13 stable | x86_64 | verified via spec suite |
| Fedora 44 | x86_64 | verified via spec suite |
| Arch Linux | x86_64 | verified via spec suite |
| macOS 14+ | arm64 + x86_64 | QEMU via Homebrew; no KVM → TCG |

## Host — Level 2 (compatible, community-verified)

- openSUSE, Linux Mint, Pop!_OS — same Bash/QEMU requirements
- **WSL2** — works with limitations:
  - No KVM → TCG (slower); nested-virt possible on Windows 11 +
    `wsl --set-version` + enabled nestedVirtualization
  - `jq` CRLF handling fixed in v-next (`validate.sh` `tr -d '\r\n'`)
  - NTFS-mounted workspaces have POSIX-permission caveats
  - sockets work, but Windows-side process inspection doesn't

## Host — Level 3 (experimental)

- WSL1 — no KVM, limited syscall coverage
- QEMU < 6.0 — deprecated args may warn
- aarch64 host — TCG only for x86_64 guests

## Guest

| Arch | Support | Firmware |
|---|---|---|
| x86_64 | full | OVMF/UEFI + BIOS fallback |
| aarch64 | full on arm64 host | AAVMF + virt machine |
| aarch64 on x86_64 | TCG emulation | slow but functional |

## QEMU compatibility

| QEMU | Status |
|---|---|
| >= 9.0 | tested |
| 8.x | supported |
| 7.x | supported, some args deprecated |
| < 7.0 | untested, may fail |

## Bash

| Bash | Status |
|---|---|
| >= 5.x | tested |
| 4.x | minimum supported (explicit check) |
| < 4.0 | refused at runtime |
