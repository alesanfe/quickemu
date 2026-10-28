# Go backend evaluation

Assessment of the Go backend (`tools/`) against ISO/IEC 25010, with
design-pattern, ATAM tradeoff and CISQ/ISO 5055 weakness analysis.

- **Audited commit**: see `git log -1` on this file's introduction
  (audit performed on `master` around `507bd4b`, 2026-10-28).
- **Method**: static reading of all 19 modules, pattern sweeps
  (unchecked errors, unbounded reads, traversal, liveness seams),
  module test runs and the `qespec` suite incl. real-QEMU e2e (TCG).

## 0. Measured base data

| Metric | Value | Command |
|---|---|---|
| Go modules | 19 | `ls -d tools/*/go.mod \| wc -l` |
| Product LOC | 16 382 | `find tools -name '*.go' -not -name '*_test.go' \| xargs wc -l` |
| Test LOC | 10 868 | `find tools -name '*_test.go' \| xargs wc -l` |
| Test functions | 365+ | `grep -rh 'func Test' tools --include='*_test.go' \| wc -l` |
| Test/product ratio | ~0.66 | derived |
| Direct `exec.Command` sites | 47 | `grep -rn 'exec.Command' tools --include='*.go' \| grep -v _test` |
| Seam-wrapped call sites | 21 | `grep -rn 'execCommand\|httpFetch\|var fetch' tools` |
| Module coverage | 53–97 % (per-module summary: `qedev quality`) | `go test -cover` |

Module coverage snapshot (audit run): qelib 86.0, qespec 81.4, api 82.6,
archive 88.9, cli 83.6, launch 80.8, ops 80.3, plugin-market 81.0,
ports 82.5, proc 89.4, qmp 87.6, toml 96.7, vm 79.8, catalog 83.9,
quickget-cli 82.8, quickget-vm 53.7, quickgetc 78.2, quickreport 85.4.

## 1. ISO/IEC 25010 verdict

| Characteristic | Grade | Evidence |
|---|---|---|
| Functional suitability | Strong | Full ported spec suite (contract/integration/e2e); parity asserted on artifacts (ADR 0011), not internals |
| Performance efficiency | Strong | Streaming for GB payloads (ADR 0010), `confKV` cache keyed by (path,mtime,size), `WalkDir`, hoisted regexes |
| Compatibility | Strong | On-disk formats identical to Bash; `NormArgs` covers both flag spellings; TOML key order frozen |
| Usability | Adequate | Typed error envelopes (code/severity/retryable); Bash `ERROR!` strings and exit codes preserved |
| Reliability | Strong | Atomic writes (ADR 0006), EPERM-aware liveness (ADR 0007), stale-lock/port reclaim windows, QEMU child reaping |
| Security | Strong | Tar-slip + bomb limits, traversal allowlists, fail-closed QMP (ADR 0008), bounded HTTP reads, loopback-first API (ADR 0013) |
| Maintainability | Strong | 19 focused modules, `qelib` single source, seams (ADR 0009), 0.66 test ratio |
| Portability | Adequate | `runtime.GOARCH` host arch, XDG helpers, darwin probes; WSL experimental (ADR 0012) |

**Overall: Strong (B+/A−).** Residual structural debt is concentrated
in `quickemu-cli/main.go` size; everything else is peripheral.

## 2. Patterns observed

- **Facade** — `runPassthru`/`qelib.Tool` wrap sibling binaries; the
  process boundary carries locking, plans and the state machine.
- **Registry** — port registry (`QE_PORT_REGISTRY`) and ops registry
  (`ops/*.json`) with PID-liveness + age-window reclaim.
- **Template method** — op lifecycle `new → set → done/fail/cancel →
  prune/recover`; terminal states are one-way.
- **Seam (Feathers)** — `execCommand`, `httpFetch`, `var fetch` (ADR
  0009).
- **Atomic writer** — `renameio` + fsync + rename (ADR 0006).
- **Allowlist/sanitizer** — `checkID`, `safeMember`, `validTag`,
  `qmpCommandAllowed`.
- **Producer–consumer** — `captureStdout` pipe + drain goroutine,
  `telnetCopy` IAC parser.
- **Adapter** — `qelib` is the single surface where Bash-era semantics
  are expressed in Go (conf edit, PCI, XDG, sendkey map).

## 3. ATAM tradeoffs

| Decision | Wins | Costs | Verdict |
|---|---|---|---|
| Multi-module layout | Per-binary SBOM, isolation | `replace ../`, no root `./...` | Keep (ADR 0003+0004) |
| Seam vars vs DI framework | Deterministic tests, zero deps | Mutable globals; no `t.Parallel` with them | Keep for ≤5 seams/pkg |
| Streaming vs ReadAll | O(1) memory | Two passes where order matters | Correct (OOM was real) |
| Fail-closed QMP + env hatch | Minimal attack surface | New QEMU cmds need allowlist edits | Correct |
| On-disk JSON vs DB | Interop + audit trail | No multi-file transactions | Correct at this scale |
| EPERM = alive | Locks unstealable | Cannot reap foreign squatters | Correct (conservative) |

## 4. Residual weaknesses (CISQ/ISO 5055 view)

| # | Weakness | Severity | Disposition |
|---|---|---|---|
| 1 | `quickemu-cli/main.go` concentrates many domains (~2.4 kLOC) | Medium | Planned split by domain (P0 backlog) |
| 2 | ~26 `exec.Command` sites without a seam | Low | Mostly probes/viewers; classify before wrapping (P1) |
| 3 | Best-effort `os.Remove`/`MkdirAll` cleanup paths | Low | Documented intent; critical writes are checked |
| 4 | QMP `human-monitor-command` passthrough is broad | Low | Parity with Bash; flag for argument filtering |
| 5 | `docs/quality/history.jsonl` churns the working tree | Trivial | Intentional series; tests now write to `QEDEV_HISTORY` temp |

### Limitations of this evaluation
- Coverage is per-module average of `go test -cover` statement
  coverage — not branch coverage, and `quickget-vm` (53.7 %) contains
  hard-to-mock network paths that inflate the gap.
- Real-QEMU e2e ran under TCG; KVM paths are validated only where the
  host provides `/dev/kvm`.
- Grades compare the backend to the preceding Bash implementation and
  to typical CLI-daemon quality bars — they are not a certification.

## 5. Re-evaluation history

| Date | Commit | Note |
|---|---|---|
| 2026-10-28 | `507bd4b` | Initial evaluation — Strong overall; backlog derived into P0–P3 tasks |
| 2026-10-28 | `571b0fe` | **Re-evaluation: A** — God-file split (cli 2376→713), structural QMP, exec seams everywhere (4 direct sites left, harness-only), API auth hardened (throttle/bind/token-file/constant-time), KVM ioctl probe (`KVM_GET_API_VERSION` — WSL device-EPERM fixed), schema-validated envelopes, adversarial lock/network matrix, metrics automated (`qedev metrics structure`: 17k/12.6k LOC, ratio 0.74, 417 tests, 0 go.work drift). Remaining: darwin CI, optional split of ops.go/scrape.go (>1k LOC, cohesive domains) |
