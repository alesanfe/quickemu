# Changelog

## 4.9.9 — Structured-API release

The contract declared by `schemas/v1/` is stable from this version;
`meta.operationId`, `data.allowedOps`, typed `errors[]` codes and the
events/ops registries are part of the public surface.

minStructuredQuickemuVersion = **4.9.9**

- Locks: shared/exclusive RW locks, stale/pid-less reclaim by mtime.
- Identity: instanceId + pid + startTime + uid + fingerprint.
- Registry: global port reservations (TCP/UDP, IPv6, lease expiry).
- Archives: manifest + sha256 per member, canonical paths, slip/link/
  bomb guards. Export is self-verifying; import verifies before install.
- Contract: envelope v1 `{schemaVersion,command,success,data,warnings,
  errors[],meta{quickemuVersion,timestamp,operationId}}`.
- QMP: channel with capabilities, correlated ids, event separation,
  Bash socat or Go `quickemu-qmp` (`--raw`/envelope, watch streams).
- States: canonical `vm_state`/`vm_allowed_ops` in `vm.status`.
- Config: TOML parse/validate/migrate (`--migrate-conf`, `--dry-run`),
  schema `schemas/v1/vm-toml.schema.json`, double-read deprecation plan.
- Ops: async registry `<vmdir>/ops/*.json`, UUID ids, cancelRequested →
  cancelled, workerPid, prune, `op watch` (fsnotify+polling fallback).
- Events: JSONL `vm.events.jsonl` with seq, cursor read, ~1MB rotation.
- VM ops: trash+restore, backups verify/restore/prune, backing-chain
  delete plan (`PLAN_EXPIRED`/`PLAN_MISMATCH`), transactional
  clone/rename/move, disk compact/resize, extra_disks/extra_nics,
  external qcow2 snapshot trees (`--snapshot tree|verify|consolidate`),
  QMP hotplug + `--persist`, `doctor --fix`, `--host-info`, `--vfio-scan`,
  subcommand aliases, XDG dirs, bash/zsh/fish completion.
- Go helpers (ADR-0001): quickemu-qmp, quickemu-proc, quickemu-ops,
  quickemu-ports, quickemu-archive, quickemu-toml — each behind the v1
  contract with Bash fallback.
