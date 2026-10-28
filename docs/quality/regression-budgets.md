# Regression budgets (QE-08.4)

Enforced by `qedev metrics trend` in CI — exits 1 on violation.

| Metric | Budget | Enforced |
|---|---|---|
| coverage_go | never decreases | metrics-trend exit 1 |
| catalog_os | never shrinks | metrics-trend exit 1 |
| spec_files | never shrinks | metrics-trend exit 1 |
| flaky tests (required) | 0 | CI required checks |
| JSON schema v1 | 100% compat same major | qespec contract tests |
| catalog health | ≥98% excl. upstream outage | catalog-health.yml |
| E2E minimal | 100% before release | ci.yml tests job |
