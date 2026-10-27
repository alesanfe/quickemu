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
| **unified CLI** | `bin/quickemu` + `lib/quickemu/cli/*.sh` | `quickemu-cli` | ✅ core — same flag surface: lifecycle (start/dry-run/status/pause/resume/shutdown/reset/stop/kill/restart), list-vms (text+JSON), snapshots/backup/clone/rename/move/trash/disk ops via quickemu-vm, monitor-cmd/send-keys/type/screenshot via quickemu-qmp HMP, op-* via quickemu-ops, conf overrides (--display/--access/--public-dir/--fullscreen/--braille/--keyboard-layout/--cpu-pinning/--extra-args) materialised into a temp conf. **Attach actions**: `vnc` (VNC client at vnc_listen, 5900+display), `spice-console` (viewer attach to running VM, same URI rules as start_viewer), `publicdir` (xdg-open public_dir), `serial-console`/`monitor-console` (interactive telnet attach) — all meaningful where Bash has dead aliases |
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
- `--doctor` (doctor_vm parity), `--delete-plan`, `--ignore-msrs-always`
- Full options.sh surface: `--export/--import`, `--backup-verify/
  --backup-restore/--backup-prune`, `--trash*/--trash-restore`,
  `--autostart`, `--shortcut/--delete-shortcut`, `--vfio-scan`,
  `--host-info`, `--capabilities`, `--inventory`, `--stats`,
  `--migrate-conf` (quickemu-toml `from-conf`), `--inspect-conf`
  (exec-free conf reader), `--events [cursor]` (JSONL seq filter),
  `--plugin <name>`/`--plugins` (quickemu-plugin-* on PATH),
  `--hotplug-usb/--hotplug-disk/--hotunplug/--persist` (QMP device_add/
  device_del/blockdev-add via internal raw channel), `--snapshot
  <act>` compound + `--external` (qcow2 overlay chains in
  snapshots.json + qemu-img commit consolidate), `--snapshot-tree/
  --snapshot-verify`, `--delete-disk` (disk+OVMF vars+shortcut),
  `--clone [--full]` (linked backing-disk default / convert full),
  `--echo`=`--dry-run`, `--check`=`--doctor`, single-dash long opts
  and `--flag=value`.
- `monitor_cmd` reaches every HMP command: raw `-monitor` unix socket
  first, then telnet, then QMP `human-monitor-command` — the same
  precedence as `monitor_send_cmd` (greeting and per-char readline
  echo drained, payload identical).
- QMP channel is fail-closed like `qmp_command_allowed`: query-*,
  screendump, system_powerdown/wakeup, stop, cont, block-dirty-bitmap,
  drive-backup, human-monitor-command; `QUICKEMU_QMP_RAW=1` sends any
  command (verified: quit → SHUTDOWN event → VM exit). Internal
  trusted flows (hotplug, lifecycle) bypass via the raw channel —
  Bash fixed identically with `qmp_query_raw`.

## quickget pipeline (quickget-cli + quickget-vm)

- `quickget-vm get <os>` — catalog url → fetch (.part resume + sha256)
  → `imageNormalize` (batocera resize, easyos/openfyde qcow2, redoxos
  zstd, freedos/kolibrios/reactos unpack) → `mkconf`
- `mkconf` — full guest_os/image-type table + per-OS tweaks +
  disk-image overrides (.qcow2/.vmdk/.raw/.img/.vdi) + `--cloud-init`
  seed + `--unattended` (ubuntu-server subiquity autoinstall)
- `customconf` — `--create-config` parity: local path or URL input,
  OS detect by filename, retail Win ISO auto-unattend,
  `--disable-unattended`
- `getUbuntu`/`getUbuntu-server` — SHA256SUMS/MD5SUMS resolution,
  launchpad daily codename, `zsync` delta download (curl fallback),
  `OS-devel.iso` naming
- `getWindows` — full Mido flow: workstation (software-download API,
  session permit, mdt.js w/rticks, SKU table, product links, Sentinel
  block messaging) and server (evalcenter fwlink scrape + culture/
  country map); virtio-win.iso + `unattendedWindows` (37-locale XML
  template, qxldod w10-only, mkisofs unattended.iso)
- `getMacos` — Apple osrecovery session → InstallationPayload →
  RecoveryImage.dmg+chunklist (AssetToken headers) → chunkcheck →
  dmg→raw convert → pinned OSX-KVM OpenCore/OVMF fetch → mkconf
- `quickreport` — host probes + redaction + output file

## Verification

```bash
tools/build-go.sh        # builds all 15 binaries
tools/go-check.sh        # vet + test -race -coverprofile + build, all modules
tools/run-go-tests.sh    # go test -race -cover per package
tools/compare-backends.sh  # zero divergences on extracted subsystems
```

E2E (real QEMU, TCG): `spec/e2e/go_cli_boot_spec.sh` boots a VM via
`quickemu-cli`, checks pidfile/QMP/status running, pause→resume,
`--monitor-cmd` HMP round-trip, QMP allowlist (query-* ok, `quit`
denied without `QUICKEMU_QMP_RAW=1`), and clean `--stop` teardown.
Skip-gated when qemu-system-*/go are unavailable.
