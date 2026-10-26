# SLI/SLO definitions — Quickemu (+ QuickGUI boundary)

Per SRE practice, each SLO is measurable against the spec suite /
event stream / op registry. Targets are for a healthy single host.

| ID | SLI | SLO | Measured by |
|---|---|---|---|
| SLO-QE-01 | valid `--start` ops that create exactly one VM process | >=99.5% | ops registry + runtime.json pid |
| SLO-QE-02 | port collisions in concurrent reservations | 0 in 10^4 runs | concurrency spec (parallel reservations unique) |
| SLO-QE-03 | envelope validity for `--format json` | 100% schemaVersion+command+success+data+errors+meta | e2e-contract.sh, schemas/v1 |
| SLO-QE-04 | ops reaching a terminal state | 100% (queued→running→succeeded/failed/cancelled) | ops spec, OP terminal rule |
| SLO-QE-05 | `--status` reflects reality | converges <=2s after process change | vm_state + pid fingerprint |
| SLO-QE-06 | destructive ops gated | 100% delete requires valid planId | PLAN_* typed errors |
| SLO-QE-07 | recoverability after kill -9 | locks/ports reclaimed on next op | chaos spec (2/2) |
| SLO-QG-01 | UI feedback latency | <100ms for local state change | widget perf tests |
| SLO-QG-02 | shown state == real state | converges <=2s (op-watch push) | watch stream + reconcile |
| SLO-QG-03 | ops surviving app restart | 100% reconciled to interrupted/resumed | activity_provider tests |

## Error budget policy

Violations surface as typed `errors[]` codes + `doctor` findings; a
breach is a failed spec, not a silent drift. Review on every release.
