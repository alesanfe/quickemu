# Roadmap

## Done (this cycle)

- Go backend is the only backend — Bash implementation removed,
  `bin/` entries are the Go binaries (Phase 4 milestone complete)
- QE-03 trust levels + `--minimum-trust`
- QE-04 QMP socket hardening + allowlist
- QE-09 OSS governance (SECURITY.md, templates, threat model)
- QE-02 E2E real-QEMU spec (TCG, skip-gated)
- QE-13 verbosity levels
- QE-14 UAT spec scenarios

## In progress

- QE-05 adapter contract spec — per-OS contract validation
- QE-10 supply chain — pinned actions, SBOM plan

## Next milestones

### Phase 2 (target 8.2)
- Coverage instrumentation (go) with quality gates
- Full platform matrix (Linux/macOS/WSL2 + multi-QEMU)
- Journal-based recovery + rollback
- ~~Golden tests Bash ↔ Go~~ — Bash removed; parity asserted on artifacts (ADR 0011)

### Phase 3 (target 8.5)
- SBOM + signed releases + provenance
- Property-based testing + mutation testing
- Full catalog health automation
- NIST SSDF alignment

### Phase 4 (target 9.0)
- Reproducible releases
- Quality SLOs with historical baselines
- Formal state-machine for all ops
