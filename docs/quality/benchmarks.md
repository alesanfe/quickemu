# QE-12: performance baselines

Measured on the CI runner (ubuntu-latest, 2 vCPU, 7GB) unless stated.

| Operation | Command | Budget | Notes |
|---|---|---|---|
| CLI startup | `quickget --version` | < 500ms | Go binary, near-instant |
| Catalog list | `quickget --list` | < 2s | all ~165 OSes |
| Release validation | `quickget --check debian 13` | < 60s | network-bound |
| Port detection | `port_forwards` parse | < 50ms | in-memory |
| JSON emit | `OUTPUT_FORMAT=json` | < 100ms | envelope only |
| QEMU boot (TCG) | qespec e2e QEMU boot | < 60s | budget in test |
| QEMU shutdown | SIGTERM → exit | < 10s | budget in test |

## Regression budget (QE-08.4)

- CLI startup: no more than +20% vs baseline
- No new spec failures vs main
- Coverage (when instrumented): no decrease below baseline
