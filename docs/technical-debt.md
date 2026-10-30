# Technical Debt Register

Policy: zero critical debt before a stable release. Critical = security,
data loss, corruption, unsafe import/export, ambiguous process identity,
broken locks, reproducible races, inconsistent JSON contract,
cancel-corruption, incomplete cleanup, secret leaks, false-success
reports, untested declared features, undocumented .conf/TOML divergence.

| ID | Item | Subsystem | Severity | Status | Resolution |
|----|------|-----------|----------|--------|------------|
| TD-001 | QMP client is single-request (no async events/mux) | quickemu-qmp | medium | resolved | Async `{"event":...}` records are separated from replies (id-correlated), echoed to stderr, persisted to `<vm>.events.jsonl` as `qemu.<EVENT>` and streamed via `watch`; concurrency is serialized by the VM op lock — QEMU enforces one QMP client per socket, so an in-client mux adds nothing |
| TD-002 | Port registry records PID+timestamp, not instanceId | quickemu-launch | low | resolved | instanceId recorded alongside pid |
| TD-003 | telnet monitor exposed when configured | quickemu-qmp | medium | resolved | Loopback default; launch warns on stderr for any non-loopback telnet bind (unauthenticated HMP) |
| TD-004 | .conf is executable code | config | high | resolved | Go backend parses .conf as data — never executed; TOML path also available |
| TD-005 | HMP fallback paths remain alongside QMP | quickemu-qmp | low | resolved | Dead HMP/helper surface deleted from quickemu-qmp (SendCmd/SendKeys/TypeText/Screenshot/Hotplug*/HotplugPersist/SocketReady — unreachable, no dispatch); one HMP transport remains: cli `monitorSend` (monitor_send_cmd parity) + `vmControl` QMP-first → HMP fallback + `hmp()` via QMP human-monitor-command |
| TD-006 | ops registry has no generic progress% | quickemu-ops | low | resolved | running 10/30/40 + completed 100 in export/import |
| TD-007 | external snapshots lack consolidate/rebase ops | quickemu-cli | medium | resolved | snapshot_consolidate commits leaf overlays |
| TD-008 | hotplug persists to .conf only (no TOML write) | quickemu-qmp | low | resolved | TOML arrays written when VM is .toml |
| TD-009 | quickget: downloads lack resume/verify hardening | quickget | medium | resolved | proto allow-list, retries, stall-abort, resume kept |
| TD-010 | No bash-completion for VM/snapshot names | completion | low | resolved | *.conf/*.toml + snapshot-tag dynamic completion |
| TD-011 | Error strings keep `ERROR!`-style punctuation (ST1005) | all | low | resolved | Parity wording kept; explicit `//lint:file-ignore ST1005` on the six carrier files — staticcheck clean |
| TD-012 | `exec.Command` sites without context | launch/cli/qespec | low | resolved | All probes/info/convert sites now `CommandContext` + timeout (5–60s) |
| TD-013 | Oversized files (scrape_new 1530, scrape 1427, hw 1013, vm 927…) | catalog/launch/vm | medium | resolved | Split into mechanism files: scrape_{gh,sf,index,api,html,mirror,ubuntu,lists,util,fetch}, hw_{probe,firmware,pinning}, storage, conf_parse, metrics, output/delete_plan/vmutil, paths/ops_adapter/args_norm/usage, lifecycle_state, conf_gen/fetch/seed, catalog/validate/format; flat
dispatches band-split (scrape_html -> html+html2, scrape_index ->
index+index2, image -> extract+unattended); heavy op bodies extracted
from run() switches (quickemu-vm ops_lifecycle+ops_snapshot); oversized
tests split (scrape2_test -> harness+table+resolvers, args2_test ->
display+firmware) — largest file now ~590 (cli dispatch table) |
| TD-014 | `cp -a` from NTFS leaves debian/* +x → dh runs them | packaging | low | resolved | CI normalizes `chmod 644` on debian/* after copy — dh-exec can never fire |
| TD-015 | `.gitignore` carries a long per-module binary list | repo | low | resolved | Collapsed to 19 glob rules, verified with git check-ignore |
| TD-016 | 26 direct `exec.Command` sites remain | all | low | resolved | All remaining sites either seam'd or `CommandContext`-bounded; 3 fixture helpers in qespec bounded too |
| TD-017 | `tools/dev` ~15-20% coverage | tooling | low | resolved | 73.6% — dispatch/metrics/structure covered via stubExec harness + pure-helper and real-tree walk tests |

Resolved this cycle: instance fingerprint, RW locks, planId, port
registry, import/export verification, JSON envelope, QMP channel,
state machine, TOML boot, async ops, events, trash, backups,
transactional rename/clone/move, multi-disk/NIC, hotplug, VFIO scan,
doctor, suite stabilization (65/65), plugin-market .deb packaging,
deb man pages + shell completions, stale completions regenerated,
quickemu-launch ≥80% coverage, e2e fixture boot medium.
