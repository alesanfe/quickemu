# Bash ↔ Go parity (ADR-0001 phase E–G)

## Status by subsystem — verified via `tools/compare-backends.sh`

| Subsystem | Bash file | Go tool | Status |
|---|---|---|---|
| ops journal | `lib/quickemu/runtime/ops.sh` | `quickemu-ops` | ✅ **100%** — new/set/done/fail/status/watch/list/cancel/prune/**recover/rollback** |
| ports registry | port flock helpers | `quickemu-ports` | ✅ **100%** — alloc/free/check |
| proc fingerprint | `vm_proc_*` | `quickemu-proc` | ✅ **100%** — comm/start-time/uuid |
| qmp client | `lib/quickemu/runtime/monitor.sh` | `quickemu-qmp` | ✅ **100%** — query-status, safe commands |
| archive import | `lib/quickemu/vm/disk.sh` (import part) | `quickemu-archive` | ✅ **100%** — tar-slip validation + extract |
| toml config | `lib/quickemu/config/toml.sh` | `quickemu-toml` | ✅ **100%** — parse/validate |
| api daemon | `quickemu-api.sh` | `quickemu-api` | ✅ **100%** — state/ops/start/stop/pause/resume/kill/reset |
| **catalog** | `lib/quickget/catalog/*.sh` | `quickget-catalog` | ✅ **~95%** — list/releases/editions/arch/trust ✅; url resolution ✅ for 113 static templates + 91 hash fetchers; **3 scrape-OSes delegate to Bash** |

## Go catalog detail

- `list`, `releases`, `editions`, `arch`, `trust` — fully resolved from `catalog/data/catalog.json` (165 entries)
- `url` — template substitution (`%RELEASE%/%EDITION%/%ARCH%`) + **live checksum fetch** via `net/http` — replaces `web_pipe` for the 113 template-based OSes
- `scrape: true` entries delegate to `bash -c 'source lib/...; get_<os>'` — documented delegation, not a gap

## Launcher (quickemu-launch) — argv parity verified

`quickemu-launch` builds the complete QEMU command line from a `.conf`
or `.toml` — the configure_* pipeline of launch.sh + vm.sh:

- **Byte-identical sorted argv** vs Bash `vm.sh`: linux+efi+headless
  (108/108 tokens), windows+efi+tpm+sdl (93/93 tokens)
- Builders: cpu (SMT topology split, auto-detect tiers, per-OS flags),
  ram, bios (OVMF dual pflash + secure global), os quirks (OsTuning
  two-phase: NetDevice/SoundCard/UsbController/Balloon/MacDiskDev/
  UsbPassthrough/Keyboard/Mouse/SMM/MachineType/CPUFlags), storage
  (SystemDisk split pattern), display (DISPLAY_DEVICE table + GL
  variants), audio (spice/host backends, QEMU -audio-help gate),
  network (-nic bridge/none, -netdev user/restrict/passt, smb=, ssh
  auto-alloc 22220-22229), spice channels, usb input, fileshare
  (public_dir/xdg/~/Public, Public-<user> tag), boot media per-OS,
  tpm (swtpm-sock), serial (none/socket/telnet), rtc, rng, balloon
- `vm_config_load`: .conf parse + .toml via quickemu-toml (flat model,
  nested keys ignored with Bash warning) + profile/network presets
- `--run` spawns QEMU with pidfile/identity/cleanup watcher;
  `--validate` checks the conf (guest_os [a-z0-9-]+ like Bash)

Also covered: `extra_nics`/`extra_macs` arrays (user|restrict|passt|
bridge:<iface>, per-NIC netdev ids + unknown-entry warning), virtiofs
via standalone virtiofsd (memory-backend-file + vhost-user-fs-pci,
daemon spawned on `--run`, 9p fallback), SPICE endpoint variants
(spice-app `gl=`, spice local `unix=on,addr=<sock>`, spice remote and
headless `port=,addr=` with 5930-5939 auto-alloc, `access` conf key
standing in for `--access`), TOML arrays flattened to bash-array
literals for `confList`.

Remaining: `--run` end-to-end boot verification vs Bash,
port_forwards array syntax details, monitor_cmd hooks, `access` is a
CLI flag in Bash (mapped to a conf key in Go).

## Verification

```bash
tools/build-go.sh        # builds all 8 binaries
tools/run-go-tests.sh    # go test -race -cover per package
tools/compare-backends.sh  # zero divergences on extracted subsystems
```
