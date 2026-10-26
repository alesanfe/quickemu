# Security Guide

## Configuration files

- `.conf` is executable Bash — treat like code; `--inspect-conf` reads
  it without executing; prefer TOML (`--migrate-conf`) which is parsed,
  never executed.
- Imported `.conf` keeps that risk — review before booting an import.

## Remote exposure

- Monitor, serial, VNC, SPICE and telnet listeners default to loopback.
  Never expose monitor/serial telnet on LAN — telnet is plaintext.
- `vnc_listen`/`--access`/`public_dir` change the threat surface;
  capabilities stay local-only by default.

## Runtime & identity

- Process actions verify instanceId+uid+startTime+commandFingerprint
  before signalling — a reused PID is never killed.
- Runtime artifacts live in XDG_RUNTIME_DIR/TMPDIR (per-user, not
  world-shared); sockets are created by QEMU with umask of the caller.

## Archives

- Export includes a manifest + sha256sums; import verifies before
  installing. Member whitelist: no symlinks, hardlinks, devices,
  FIFOs, absolute paths, `..` traversal, or undeclared content.
  Anti-bomb limits: 100k members, depth ≤16, staging extraction.

## Reports

- quickreport passes all output through qr_redact (HOME, username,
  hostname, IP, MAC, credentialed URLs) — verify no personal data
  before sharing quickreport.txt.

## Operations

- Destructive ops take the exclusive VM lock; readers take shared
  locks. --delete-plan → --delete-vm --plan-id is the transactional
  path; prefer --trash when unsure.
