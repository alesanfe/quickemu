# Contract Reference (schema v1)

## Envelope

Every `--format json` response is:

    {"schemaVersion":1,"command":"<ns.action>","success":true|false,
     "data":{...},"warnings":[],"errors":[{"code","message"}],
     "meta":{"quickemuVersion","timestamp"}}

Errors are typed strings (DELETE_PLAN_STALE, CHECKSUM_MISMATCH,
SNAPSHOT_CONFLICT, NOT_FOUND, PERMISSION_DENIED, IMPORT_CONFLICT,
IMPORT_ABORTED...). Machine output goes to stdout only; human text to
stderr.

## Status

    data: {"name","state","pid","allowedOps":[]}
    state ∈ stopped|running|paused|stale|stopping|unknown

## Operations

<vm>/ops/<operationId>.json: {"operationId","type","state","progress",
"startedAt","result","error"}; state ∈ queued|running|completed|
failed|cancelled|interrupted. Query: --op-status/--op-list; --op-cancel.

## Events

<vm>.events.jsonl: {"seq","ts","event","vm","pid","data"}. Cursor =
last seen seq; `--events [cursor]` resumes. Rotates at ~1MB.

## Capabilities flags

qmpChannel, instanceIdentity, sharedExclusiveLocks, portRegistry,
verifiedArchives, verifiableDeletePlan, structuredEvents,
asyncOperations, tomlConfig, hostInventory + legacy set.

## Exit codes

0 ok · 1 generic · 5 stale/plan mismatch · 12 permission/conflict
errors typed via qe_die*.
