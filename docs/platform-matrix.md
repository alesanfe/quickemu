# Platform support matrix

Tier 1 = tested in CI/dev + qespec e2e; Tier 2 = implemented,
partially verified; experimental = detected with degraded behaviour.

| Host | Status | Notes |
|---|---|---|
| Linux amd64 + KVM | **Tier 1** | primary target; accel=kvm via `KVM_GET_API_VERSION` ioctl probe |
| Linux amd64, no KVM | Tier 1 | TCG fallback with warning (containers, VT-x off) |
| Linux arm64 + KVM | Tier 2 | hostArch→aarch64, AAVMF firmware, `virt` machine |
| macOS arm64 | Tier 2 | accel=hvf (reality-check branch); cocoa display |
| macOS amd64 | Tier 2 | accel=hvf; no OVMF — UEFI guests not supported |
| **WSL2** | experimental | `IsWSL` via env/osrelease; nested-virt KVM works on some builds, device may exist with ioctls EPERM → TCG + warning; `nestedVirtualization=true` in `.wslconfig` enables KVM |
| **WSL1** | experimental-reject | no KVM at all → TCG always; unambiguous warning |
| Guest aarch64 on amd64 | Tier 2 | cross-arch → tcg + `qemu64`/`max` |
| Nested hypervisor (qemu/vbox) | Tier 2 | `hostHypervisor` forces tcg |

## Environment matrix

| Factor | Handling |
|---|---|
| `$HOME` with spaces | filepath joins only; no shell interpolation in paths |
| XDG overrides | `XDG_DATA_HOME`/`CACHE`/`CONFIG` honored (qelib.XDG*) |
| Non-English locale | no localized-string scraping except sendkey map (ASCII) |
| Unicode filenames | Go strings are UTF-8; archive member validation is byte-level |
| Case-insensitive FS (APFS default) | no case-colliding paths in code; conf keys lowercase |
| Missing optional tools | probes degrade (`toolExists` gates viewer/daemon features) |
| `/dev/kvm` EPERM ioctls | treated as absent → TCG (WSL-reboot scenario) |

## Known limits

- Windows host: not supported (POSIX paths, /proc usage).
- WSL↔Windows filesystem interop (`/mnt/c`) works for storage but
  inode/lock semantics differ — VM dirs on `/mnt/c` are discouraged.
- `quickemu-api` token file perms rely on POSIX mode bits.
