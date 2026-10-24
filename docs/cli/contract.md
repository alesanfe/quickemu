# CLI contract for tooling

| Command | Output | Exit |
|---|---|---|
| `--capabilities` | JSON descriptor (schema v1) | 0/1 |
| `--list-vms [--format json]` | VM list | 0/1 |
| `--status --format json` | `{name,state,pid}` | 0/1 |
| `--delete-plan [--format json]` | paths that `--delete-vm` removes | 0/1 |
| `--dry-run [--format json]` | QEMU command plan | 0/1 |

All JSON envelopes carry `"schemaVersion": 1`. Human output is the
default; `--format json` opts in per invocation.
