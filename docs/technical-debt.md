# Technical Debt Register

Policy: zero critical debt before a stable release. Critical = security,
data loss, corruption, unsafe import/export, ambiguous process identity,
broken locks, reproducible races, inconsistent JSON contract,
cancel-corruption, incomplete cleanup, secret leaks, false-success
reports, untested declared features, undocumented .conf/TOML divergence.

| ID | Item | Subsystem | Severity | Status | Resolution |
|----|------|-----------|----------|--------|------------|
| TD-001 | QMP client is single-request (no async events/mux) | quickemu-qmp | medium | accepted | Extract `quickemu-qmp` when protocol trigger fires (ADR-0001) |
| TD-002 | Port registry records PID+timestamp, not instanceId | quickemu-launch | low | resolved | instanceId recorded alongside pid |
| TD-003 | telnet monitor exposed when configured | quickemu-qmp | medium | documented | Warned in docs; loopback default; deprecation candidate |
| TD-004 | .conf is executable code | config | high | resolved | Go backend parses .conf as data — never executed; TOML path also available |
| TD-005 | HMP fallback paths remain alongside QMP | quickemu-qmp | low | accepted | Documented fallback contract |
| TD-006 | ops registry has no generic progress% | quickemu-ops | low | resolved | running 10/30/40 + completed 100 in export/import |
| TD-007 | external snapshots lack consolidate/rebase ops | quickemu-cli | medium | resolved | snapshot_consolidate commits leaf overlays |
| TD-008 | hotplug persists to .conf only (no TOML write) | quickemu-qmp | low | resolved | TOML arrays written when VM is .toml |
| TD-009 | quickget: downloads lack resume/verify hardening | quickget | medium | resolved | proto allow-list, retries, stall-abort, resume kept |
| TD-010 | No bash-completion for VM/snapshot names | completion | low | resolved | *.conf/*.toml + snapshot-tag dynamic completion |
| TD-011 | Error strings keep `ERROR!`-style punctuation (ST1005) | all | low | accepted | User-facing wording is a parity contract; tests match it |
| TD-012 | 12 `exec.Command` sites without context | launch/cli | low | accepted | All short-lived local probes (lscpu, taskset, qemu-img) |
| TD-013 | Oversized files (scrape_new 1530, scrape 1427, hw 1013, vm 927…) | catalog/launch/vm | medium | open | Split candidates: scrape families, host probes, CLI dispatch |
| TD-014 | `cp -a` from NTFS leaves debian/* +x → dh runs them | packaging | low | documented | dh treats executable config as dh-exec scripts; CI (ext4) unaffected — chmod after copy on WSL |
| TD-015 | `.gitignore` carries a long per-module binary list | repo | low | open | Could collapse into `tools/*/` short-name globs |
| TD-016 | 26 direct `exec.Command` sites remain | all | low | open | Wrap behind seams for testability (was 47) |
| TD-017 | `tools/dev` ~15-20% coverage | tooling | low | accepted | Internal runner; not part of the public surface |

Resolved this cycle: instance fingerprint, RW locks, planId, port
registry, import/export verification, JSON envelope, QMP channel,
state machine, TOML boot, async ops, events, trash, backups,
transactional rename/clone/move, multi-disk/NIC, hotplug, VFIO scan,
doctor, suite stabilization (65/65), plugin-market .deb packaging,
deb man pages + shell completions, stale completions regenerated,
quickemu-launch ≥80% coverage, e2e fixture boot medium.
