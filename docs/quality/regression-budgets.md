# Regression budgets (QE-08.4)

Enforced by tools/metrics-trend.sh in CI — exits 1 on violation.

| Metric | Budget | Enforced |
|---|---|---|
| coverage_bash | never decreases | metrics-trend exit 1 |
| catalog_os | never shrinks | metrics-trend exit 1 |
| spec_files | never shrinks | metrics-trend exit 1 |
| flaky specs (required) | 0 | CI required checks |
| JSON schema v1 | 100% compat same major | contract specs |
| catalog health | ≥98% excl. upstream outage | catalog-health.yml |
| E2E minimal | 100% before release | e2e job |
