# Quality metrics (QE-08)

## Per-CI-run metrics (collect)

| Metric | Source | Threshold |
|---|---|---|
| test pass rate | shellspec | 100% on required specs |
| specs executed | shellspec --count | ≥ baseline |
| coverage (bash) | `tools/coverage.sh` — function→spec map | 100% — all 246 functions referenced by specs |
| coverage (go) | `go test -cover` | ≥80% domain |
| catalog health | `catalog-health.yml` | ≥98% OK excluding upstream outages |
| CLI startup | `time quickget --version` | ≤500ms |
| QEMU boot (TCG) | e2e spec | ≤60s |
| QEMU shutdown | e2e spec | ≤10s |
| flaky specs | repeated CI failures | 0 in required |

## Release-level metrics

- SBOM component count (`scripts/gen-sbom.sh`)
- Signed artifact coverage (release workflow)
- OpenSSF score (self-assessed)
- Issues closed / opened ratio
- Mean time to fix critical bug

## Baselines (to be recorded)

| Release | coverage | specs | catalog-ok | boot-ms | shutdown-ms |
|---|---|---|---|---|---|
| 4.9.9 | 100% funcs | 81+ | ~98% | TBD | TBD |
