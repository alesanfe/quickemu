# schemas/v1 — public JSON contract

These JSON Schemas are the public contract between quickemu/quickget and
external clients (QuickGUI, TUIs, scripts). `QE_SCHEMA_VERSION` in
`lib/quickemu/support/json.sh` is the single source of the contract version.

## Enforced schemas (live emitters)

Validated by `spec/contract/schema/schema_conformance_spec.sh` — every
required key must be present, no undeclared top-level keys may be emitted,
and `schemaVersion` must equal the declared `const`:

| Schema | Emitted by |
|--------|-----------|
| `capabilities.schema.json` | `quickemu --capabilities` |
| `vm-list.schema.json` | `quickemu --list-vms --format json` |
| `vm-status.schema.json` | `quickemu --vm x.conf --status --format json` |
| `delete-plan.schema.json` | `quickemu --vm x.conf --delete-plan --format json` |
| `execution-plan.schema.json` | `quickemu --vm x.conf --dry-run --format json` |

## Declared contracts (no live emitter yet)

Reserved shapes for the v1 contract that consumers may rely on when an
emitter lands; they are intentionally not exercised by the conformance
suite today:

| Schema | Intended for |
|--------|-------------|
| `catalog.schema.json` | A future wrapped catalog document. `quickget --list-json` currently emits the legacy bare array that QuickGUI parses. |
| `error.schema.json` | Structured error responses. |
| `vm-config.schema.json` | The `.conf` file shape, when exported as JSON. |
