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
| **unified CLI** | `bin/quickemu` + `lib/quickemu/cli/*.sh` | `quickemu-cli` | ✅ core — same flag surface: lifecycle (start/dry-run/status/pause/resume/shutdown/reset/stop/kill/restart), list-vms (text+JSON), snapshots/backup/clone/rename/move/trash/disk ops via quickemu-vm, monitor-cmd/send-keys/type/screenshot via quickemu-qmp HMP, op-* via quickemu-ops, conf overrides (--display/--access/--public-dir/--fullscreen/--braille/--keyboard-layout/--cpu-pinning/--extra-args) materialised into a temp conf |
| **catalog** | `lib/quickget/catalog/*.sh` | `quickget-catalog` | ✅ **100%** — list/releases/editions/arch/trust ✅; url resolution ✅ for 113 static templates + 91 hash fetchers + **3 scrape-OSes (cachyos/manjaro/reactos) nativos en scrape.go** |

## Go catalog detail

- `list`, `releases`, `editions`, `arch`, `trust` — fully resolved from `catalog/data/catalog.json` (165 entries)
- `url` — template substitution (`%RELEASE%/%EDITION%/%ARCH%`) + **live checksum fetch** via `net/http` — replaces `web_pipe` for the 113 template-based OSes
- `scrape: true` entries resolve natively in `scrape.go` (page/JSON/redirect parity with `get_<os>`) — **no Bash delegation remains**

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
port_forwards array syntax details, `access` is a
CLI flag in Bash (mapped to a conf key in Go).

`monitor_cmd` is covered: `quickemu-cli --monitor-cmd` and
`--send-keys`/`--type`/`--screenshot` reach the guest via QMP
`human-monitor-command` through `quickemu-qmp`.

## Unified CLI (quickemu-cli)

`tools/quickemu-cli` is the Go `quickemu` binary — the user-facing
entry point that previously only existed as Bash. It parses the same
flag surface as `bin/quickemu` + `lib/quickemu/cli/options.sh` and
dispatches every action to a Go backend tool:

- lifecycle → `quickemu-launch` (`--run`) / pid+marker state machine
- QMP actions → `quickemu-qmp execute` (`.paused` marker parity with
  `pause_vm`/`resume_vm`)
- filesystem ops → `quickemu-vm`
- journal ops → `quickemu-ops`
- `--format json` emits the v1 envelope on vm.status / vm.list

Still Bash-only CLI surface (documented residual): doctor, plugins,
shortcuts, vfio-scan, migrate-conf, inspect-conf, inventory, events,
autostart, backup-verify/restore/prune, trash-restore/empty,
delete-disk, delete-plan, snapshot-info/tree/verify, viewer/vnc/
spice-console, telnet monitor/serial attach, --echo/--export/--import.

## Verification

```bash
tools/build-go.sh        # builds all 8 binaries
tools/run-go-tests.sh    # go test -race -cover per package
tools/compare-backends.sh  # zero divergences on extracted subsystems
```
