# Go backend parity (ADR-0001 — migration complete)

The Go backend is the **only** implementation: `bin/quickemu`,
`bin/quickget`, `bin/quickgetc` and `bin/quickreport` exec the Go
tools directly. The Bash implementation (`lib/`, `libexec/`,
`bin/*.bash`) has been removed.

## Status by subsystem

| Subsystem | Former Bash file | Go tool | Status |
|---|---|---|---|
| ops journal | `lib/quickemu/runtime/ops.sh` | `quickemu-ops` | ✅ **100%** — new/set/done/fail/status/watch/list/cancel/prune/**recover/rollback** |
| ports registry | port flock helpers | `quickemu-ports` | ✅ **100%** — alloc/free/check |
| proc fingerprint | `vm_proc_*` | `quickemu-proc` | ✅ **100%** — comm/start-time/uuid |
| qmp client | `lib/quickemu/runtime/monitor.sh` | `quickemu-qmp` | ✅ **100%** — query-status, safe commands |
| archive import | `lib/quickemu/vm/disk.sh` (import part) | `quickemu-archive` | ✅ **100%** — tar-slip validation + extract |
| toml config | `lib/quickemu/config/toml.sh` | `quickemu-toml` | ✅ **100%** — parse/validate |
| api daemon | `quickemu-api.sh` (retired) | `quickemu-api` | ✅ **100%** — persistent HTTP daemon (state/ops/start/stop/pause/resume/kill/reset, bearer auth, TCP+unix listen); the socat shim was removed |
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
- `--run` spawns QEMU setsid-detached (survives session close) with
  pidfile/identity, plus a double-forked cleanup watcher that reaps
  swtpm/virtiofsd daemons and runtime state when QEMU exits —
  `vm_runtime_cleanup` parity. `swtpm` is spawned by the launcher
  itself with a pidfile (Bash `configure_tpm` parity), and the TPM
  chardev is only emitted once the socket is ready.
  `--validate` checks the conf (guest_os [a-z0-9-]+ like Bash)

Also covered: `extra_nics`/`extra_macs` arrays (user|restrict|passt|
bridge:<iface>, per-NIC netdev ids + unknown-entry warning), virtiofs
via standalone virtiofsd (memory-backend-file + vhost-user-fs-pci,
daemon spawned on `--run`, 9p fallback), SPICE endpoint variants
(spice-app `gl=`, spice local `unix=on,addr=<sock>`, spice remote and
headless `port=,addr=` with 5930-5939 auto-alloc, `access` conf key
standing in for `--access`), TOML arrays flattened to bash-array
literals for `confList`, `extra_args` appended verbatim after the
channel arguments, `port_forwards` host:guest syntax validation in
`--validate`.

Port registry (`ports.go`, run mode only): mkdir-lock allocation with
stale-owner reclaim, reservations keyed by port number and owned by
the launcher PID (`$$` parity), live TCP probe, release on stop —
`quickemu-ports` parity. `.ports` records ssh/spice/unix/monitor-
telnet/serial-telnet entries; `.spice` records the connect target
(unix path or TCP port) for `start_viewer`.

Verified end-to-end against real QEMU 8.2 (TCG, `qemu64` CPU model
auto-selected under TCG): boot → QMP socket → pause/resume → HMP
monitor commands → allowlisted QMP (denied `quit`, accepted
`query-*`) → raw-channel USB hotplug (`device_add usb-host` with
`vendorid`/`productid` — QEMU's actual property names; Bash used the
same fix after `hostvendor`/`hostport` was rejected by real QEMU) →
stop → watcher cleanup of pidfile/sockets/`.ports`/`.spice`.

Remaining: `access` is a CLI flag in Bash (mapped to a conf key in
Go).

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
  Bash fixed identically with `qmp_query_raw`. USB hotplug uses the
  real QEMU properties `vendorid`/`productid` (verified: real
  `device_add usb-host` accepted, `info usb` shows the device).

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
  RecoveryImage.dmg+chunklist (AssetToken headers) → pure-Go
  chunkcheck → dmg→raw convert → pinned OSX-KVM OpenCore/OVMF fetch
  → mkconf
- `quickreport` — host probes + redaction + output file

## Entry points (all four are Go-only wrappers)

`bin/quickemu`, `bin/quickget`, `bin/quickgetc`, `bin/quickreport`
are thin wrappers that exec the Go tool — resolving
`libexec/quickemu/<tool>` in packaged installs and
`tools/<tool>/<tool>` in a dev tree — and inject the tool dir onto
PATH. There is no Bash fallback and no `QUICKEMU_BACKEND` switch.
Specs exercise the public binaries; behaviours once tested via
sourced Bash internals now live in Go unit tests (lock/state
machine, conf parsing, port registry, QMP allowlist, catalog
contracts, chunklist verify).

Selection-validation parity in `quickget-cli` (the uat/contract
specs pin this): `os_supported` gate on any positional OS;
`error_specify_release` (OS info + releases + editions/archs);
`validate_release`/`resolveEdition` against the catalog; trust gate
+ dynamic `trust_warn` (static D/E levels warn unconditionally; a
checksummed source whose URL probe FAILs also warns — mirrors
`trust_os` running `get_<os>` live). All user-facing validation
errors print to **stdout** like Bash `echo`; only trust/`>&2`
errors go to stderr.

## External-binary reductions (pure-Go libraries)

The Go pipeline drops host-tool dependencies where a library is
available; each substitution was verified output-identical:

| Bash dependency | Go replacement |
|---|---|
| `openssl passwd -6` (autoinstall hash) | `simia-tech/crypt` — byte-identical `$6$` SHA512-crypt |
| `bsdtar`/`7z`/`isoinfo` (ISO bootfiles) | `kdomanski/iso9660` — pure-Go ISO9660+RR walk (external tools remain as UDF fallback) |
| `7z` (kolibrios archive) | `bodgit/sevenzip` — flat extraction parity |
| `mkisofs`/`genisoimage`/`xorrisofs`/`cloud-localds` (ISO creation) | `diskfs/go-diskfs` — ISO9660 writer w/ Rock Ridge + Joliet + volid (verified w/ `isoinfo`) |
| `uname -s -r -m` | `golang.org/x/sys/unix` `unix.Uname` — same syscall, no subprocess |
| `zstd`/`xz`/`gzip`/`bzip2`/`unzip` | `klauspost/compress`, `ulikunitz/xz`, `archive/zip`, stdlib decompressors |
| regex scrape of alpine `latest-releases.yaml` | `gopkg.in/yaml.v3` real parse (flavor+arch+iso+sha256 fields) |
| hand-built `fmt.Sprintf` JSON payloads | `encoding/json` marshalling everywhere (envelopes, QMP commands, journal JSONL, trash-meta, op store) |
| TOML | `BurntSushi/toml` (already in place) |
| `libexec/quickemu/chunkcheck.py` (python3) | `checkChunklist` in `quickget-vm` — CNKL parse + per-chunk SHA-256 + RSA `sig^e mod n` via `math/big` (`crypto/subtle` compare, unit-tested) |
| `lspci -Dnn` (host-info, vfio scan, quickreport) | `/sys/bus/pci/devices` walk + `jaypipes/pcidb` (embedded pci.ids) — `lspci`-shaped lines incl. class hex, rev, bound driver |
| `lscpu` (quickreport dump) | `/proc/cpuinfo` direct read (lscpu itself formats cpuinfo) |
| `telnet` binary (monitor/serial attach) | `telnetAttach` — pure-Go RFC-854 client, inbound IAC/SB…SE filter + outbound 0xFF doubling (verified vs live QEMU monitor) |

Remaining external commands are the ones that ARE the runtime
interfaces: `qemu-system-*`, `qemu-img`, `swtpm`, `virtiofsd`, host
integration (`systemctl`, `sudo`, `tee`, `update-initramfs`,
`mkinitcpio`, `qemu-bridge-helper`), viewers (`spicy`,
`remote-viewer`, `vncviewer`, `xdg-open`), `socat`-class interactive
attach on non-Linux, and optional `zsync`/`curl` for downloads (with
fallbacks; `bsdtar`/`7z`/`isoinfo` remain only for UDF-format ISOs).

Fixed while converting: `actHotplugDisk` sent `device_add` before
`blockdev-add` and referenced a nonexistent `drive` name — now does
`blockdev-add` first, then `device_add drive=<node>` (same as Bash and
`quickemu-qmp`). Also: absolute `disk_img`/`iso` conf values no longer
get double-joined (`filepath.Join` doesn't reset on absolute operands);
`--delete-plan` items are cwd-relative like Bash's `${VM}`/`find`
output; re-launching a live VM attaches the viewer instead of spawning
a second QEMU; the detached reaper verifies pidfile ownership before
cleaning so a stale reaper can't wipe a relaunched VM's state; the
exclusive `vm_lock` (mkdir + pid + stale reclaim) guards boot and
mutating ops; the `.zsync` fallback now fetches the underlying image
URL (`.zsync` stripped) instead of saving the control file as the
image; `quickemu-vm` emits the v1 envelope only under
`--format json`/`OUTPUT_FORMAT=json` (human lines are gated like
`qe_info`); `--export` archives the real `--vm` conf path, not
`<dir>.conf`.

## Verification

```bash
go run ./tools/dev build   # entries → bin/, tool binaries in-place
go run ./tools/dev check   # vet + test -race -coverprofile + build, all modules
go run ./tools/dev test    # go test -race -cover per package
```

E2E (real QEMU, TCG): `tools/qespec/e2e_test.go` boots a VM via
`quickemu-cli`, checks pidfile/QMP/status running, pause→resume,
`--monitor-cmd` HMP round-trip, QMP allowlist (query-* ok, `quit`
denied without `QUICKEMU_QMP_RAW=1`), and clean `--stop` teardown.
Skip-gated when qemu-system-*/go are unavailable.
