# Operation recovery & rollback (QE-11)

## Journal layout

`ops.sh` already persists per-operation state:

```
<vmdir>/.ops/<operationId>.json
{"schemaVersion":1,"operationId":"...","type":"...","vm":"...",
 "state":"queued|running|completed|failed|cancelled",
 "progress":0-100,"startedAt":"...","result":{...},"error":{...}}
```

## Recovery on startup (QE-11.3)

At `quickemu` startup, scan `.ops/` for entries in `queued`/`running`
with `startedAt` older than the process start — those are interrupted
operations. The recovery flow:

1. Load all pending ops from `.ops/*.json`
2. For each, check whether the owning process is still alive
   (PID recorded in `owner.pid` when the op acquires the lock)
3. If dead: mark `state: "failed"` with `error.code: "OPERATION_INTERRUPTED"`
   and offer recovery actions
4. If alive: leave it alone — it's still running

## Recovery actions per op type

| Op type | Recovery |
|---|---|
| download | resume or delete `.part` |
| import | delete temp dir, rollback extracted files |
| delete | complete or restore `.trash` |
| clone | delete partial clone |
| rename | revert to old name |
| compact | discard temp disk |

## Idempotency (QE-11.4)

- `vm_stop` on a stopped VM → success, no-op
- `vm_status` on a missing VM → `VM_NOT_FOUND`, no crash
- `op-status` on unknown id → `OPERATION_NOT_FOUND`, no crash
- Re-download same release → resumes `.part` or restarts clean
- Applying an executed plan → `PLAN_MISMATCH` (not silent re-run)
- Cleaning an orphan lock → validates owner dead first

## Not yet implemented

- `op_recover()` — the startup scan described above
- `op_rollback()` — per-type compensations
- Interrupted-op test fixtures
