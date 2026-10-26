# Threat Model — Quickemu (+ QuickGUI boundary)

OWASP Threat Modeling frame: what we build, what can go wrong, what we
do about it, how we verify it. Assets: VM disks/configs, host
filesystem, other VMs' resources, the user's trust in shown state.

## Scope / trust boundaries

```
user ──CLI──> quickemu ──spawn──> qemu-system-*
                │  ├─ reads: .conf (Bash, TRUSTED input) / .toml (parsed)
                │  ├─ writes: VMDIR, ops/, events.jsonl, locks, ports/
                │  ├─ touches: registry, runtime.json, archives (.tar.gz)
                │  └─ serves: envelope v1 to QuickGUI (untrusted-ish text)
QuickGUI ──spawn──> quickemu (untrusted binary on PATH until verified)
```

## Threats → mitigations → verification

| # | Threat (STRIDE) | Mitigation (code) | Verification (spec/tool) |
|---|---|---|---|
| T1 | `.conf` executes arbitrary Bash if blindly sourced (RCE) | `inspect-conf`/`--migrate-conf` never source; TOML path parses not executes | spec contract, toml parser tests |
| T2 | Malicious TOML → parser DoS | 4KB line cap, quickemu-toml native parser, fuzz | `tools/fuzz-toml.sh` (found real DoS) |
| T3 | TAR.GZ slip/links/bomb → filesystem escape | `IsLocal`, canonical path, dup/equiv reject, size/member/depth limits, staging+verify before install | adversarial spec, archive go tests |
| T4 | PID reuse → killing/attaching wrong process | fingerprint = pid+startTime+uid+exe+instanceId; `verify_pid` | adversarial spec (foreign-pid) |
| T5 | Stale/corrupt lock wedges every op | dead-pid reclaim + pid-less reclaim by mtime | chaos+concurrency specs |
| T6 | Port race → two VMs bind same port | per-port markers under global registry lock | concurrency spec (8/8 unique) |
| T7 | TOCTOU between check and use (delete/import) | `planId` sha256 + `PLAN_EXPIRED`/`PLAN_MISMATCH`; atomic staging+rename | delete_plan spec |
| T8 | Command injection via conf/args | argument arrays, no eval of input, sanitize names, validation (mac/ports/vnc/ram) | hardening spec, conf sanitize tests |
| T9 | Path traversal out of conf dir | realpath confinement under conf dir before destructive ops | hardening spec |
| T10 | Symlink confusion (conf/runtime writes) | resolveSymbolicLinks on write targets; runtime.json perms 600 | atomic test, symlink test |
| T11 | Download tampering (quickget ISO) | chunkcheck + mirrors + integrity where publishers sign | quickget specs, web.sh probes |
| T12 | Partial writes → corrupt runtime/config | tmp+rename / renameio fsync / journal-style delete-plan | ops spec, AtomicStore |
| T13 | Secrets in logs/reports | `qr_redact` scrubbing HOME/user/IP/MAC/credentialed URLs | report spec |
| T14 | Malicious process spoofs quickemu to QuickGUI | `minStructuredQuickemuVersion` gating + envelope schema validation | legacy_policy dart tests |
| T15 | Denial via resource exhaustion (events/ops growth) | JSONL rotation ~1MB, ops pruning, 1MiB QMP bound | events spec |

## Residual risks (accepted or pending)

- `.conf` itself is executable Bash BY DESIGN — threat accepted,
  mitigated by TOML migration + inspect path. Never parse .conf from
  untrusted sources without `--inspect-conf`.
- QEMU-level escapes (guest→host) are upstream QEMU's model — we don't
  add exposure (no root requirement, no extra privileged helpers).
- REST daemon (`quickemu-api`) is READ-ONLY; write paths still go
  through the CLI so process-level auth isn't needed on the socket.

## Update cadence

Reviewed at every contract/version change; each new spec adversarial
case links back to a threat row.
