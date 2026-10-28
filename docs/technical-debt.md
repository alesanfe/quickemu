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
| TD-004 | .conf is executable code | config | high | mitigated | TOML declarative format exists; .conf kept for compat (deprecate post-parity) |
| TD-005 | HMP fallback paths remain alongside QMP | quickemu-qmp | low | accepted | Documented fallback contract |
| TD-006 | ops registry has no generic progress% | quickemu-ops | low | resolved | running 10/30/40 + completed 100 in export/import |
| TD-007 | external snapshots lack consolidate/rebase ops | quickemu-cli | medium | resolved | snapshot_consolidate commits leaf overlays |
| TD-008 | hotplug persists to .conf only (no TOML write) | quickemu-qmp | low | resolved | TOML arrays written when VM is .toml |
| TD-009 | quickget: downloads lack resume/verify hardening | quickget | medium | resolved | proto allow-list, retries, stall-abort, resume kept |
| TD-010 | No bash-completion for VM/snapshot names | completion | low | resolved | *.conf/*.toml + snapshot-tag dynamic completion |

Resolved this cycle: instance fingerprint, RW locks, planId, port
registry, import/export verification, JSON envelope, QMP channel,
state machine, TOML boot, async ops, events, trash, backups,
transactional rename/clone/move, multi-disk/NIC, hotplug, VFIO scan,
doctor, suite stabilization (65/65).
