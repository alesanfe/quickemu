# Quality metrics (QE-08)

## Per-CI-run metrics (collect)

| Metric | Source | Threshold |
|---|---|---|
| test pass rate | `go test ./tools/qespec` | 100% on required tests |
| tests executed | `go test -v` count | ≥ baseline |
| coverage (go) | `go test -cover` per module | ≥80% domain |
| catalog health | `catalog-health.yml` (`qedev check-catalog`) | ≥98% OK excluding upstream outages |
| CLI startup | `time quickget --version` | ≤500ms |
| QEMU boot (TCG) | qespec e2e test | ≤60s |
| QEMU shutdown | qespec e2e test | ≤10s |
| flaky tests | repeated CI failures | 0 in required |

## Release-level metrics

- SBOM component count (`qedev sbom`)
- Signed artifact coverage (release workflow)
- OpenSSF score (self-assessed)
- Issues closed / opened ratio
- Mean time to fix critical bug

## Baselines (to be recorded)

| Release | coverage | tests | catalog-ok | boot-ms | shutdown-ms |
|---|---|---|---|---|---|
| 5.0.0 | go per-module | 46 | ~98% | TBD | TBD |
