# Technical Debt Register

Policy: zero critical debt before a stable release. Critical = security,
data loss, corruption, unsafe import/export, ambiguous process identity,
broken locks, reproducible races, inconsistent JSON contract,
cancel-corruption, incomplete cleanup, secret leaks, false-success
reports, untested declared features, undocumented .conf/TOML divergence.

| ID | Item | Subsystem | Severity | Status | Resolution |
|----|------|-----------|----------|--------|------------|
| TD-001 | QMP client is single-request (no async events/mux) | monitor.sh | medium | accepted | Extract `quickemu-qmp` when protocol trigger fires (ADR-0001) |
| TD-002 | Port registry records PID+timestamp, not instanceId | lifecycle.sh | low | accepted | Bind to instance identity in registry v2 |
| TD-003 | telnet monitor exposed when configured | monitor.sh | medium | documented | Warned in docs; loopback default; deprecation candidate |
| TD-004 | .conf is executable code | config | high | mitigated | TOML declarative format exists; .conf kept for compat (deprecate post-parity) |
| TD-005 | HMP fallback paths remain alongside QMP | monitor.sh | low | accepted | Documented fallback contract |
| TD-006 | ops registry has no generic progress% | ops.sh | low | planned | Wire op_set progress in long ops |
| TD-007 | external snapshots lack consolidate/rebase ops | snapshots.sh | medium | planned | qemu-img rebase/commit phase |
| TD-008 | hotplug persists to .conf only (no TOML write) | monitor.sh | low | planned | TOML writer when config format stabilizes |
| TD-009 | quickget: downloads lack resume/verify hardening | quickget | medium | open | ADR-0002 audit block |
| TD-010 | No bash-completion for VM/snapshot names | completion | low | open | Extend tools/quickemu-completion.* |

Resolved this cycle: instance fingerprint, RW locks, planId, port
registry, import/export verification, JSON envelope, QMP channel,
state machine, TOML boot, async ops, events, trash, backups,
transactional rename/clone/move, multi-disk/NIC, hotplug, VFIO scan,
doctor, suite stabilization (65/65).
