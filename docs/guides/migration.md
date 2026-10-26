# Migration Guide

## .conf → TOML

    quickemu --vm myvm.conf --migrate-conf          # writes myvm.toml
    quickemu --vm myvm.conf --migrate-conf --dry-run # preview/diff
    quickemu --vm myvm.toml                          # boots declaratively

TOML is parsed, never executed. Keep the .conf until parity is
declared; both can coexist (explicit --vm wins).

## Flags → subcommands

Every action flag works as a bare subcommand:
`quickemu status --vm x` ≡ `--status`. Both forms remain supported.

## Delete with planId (transactional)

    quickemu --vm x --delete-plan            # prints resources + planId
    quickemu --vm x --delete-vm --plan-id <id>

Any filesystem change between plan and delete → DELETE_PLAN_STALE.

## Trash instead of delete

    quickemu --vm x --delete-vm --trash      # to ~/.local/share/quickemu/trash
    quickemu --trash-list                    # inventory
    quickemu --trash-restore x               # restores to origin dir
    quickemu --trash-empty                   # purge all

## Backups

    quickemu --vm x --backup                 # verified archive in x/backups/
    quickemu --vm x --backup-list --format json
    quickemu --backup-verify <file>
    quickemu --backup-restore <file>
    quickemu --vm x --backup-prune 5         # keep newest 5

## External snapshots

    quickemu --vm x --snapshot create <tag> --external
    quickemu --vm x --snapshot tree|verify   # chain view / integrity

## Async operations + events

    quickemu --vm x --op-list                # all operation states
    quickemu --vm x --op-status <id>
    quickemu --vm x --op-cancel <id>
    quickemu --vm x --events [cursor]        # JSONL, seq-resumable

## Doctor

    quickemu --vm x --doctor                 # findings
    quickemu --vm x --doctor --fix           # safe repairs only
