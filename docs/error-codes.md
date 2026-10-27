# Stable Error Codes (JSON contract v1)

All structured-error output uses one of these codes in `error.code`. Codes
are stable within the contract version — do not reuse or rename them without
a schema bump.

| Code | Meaning | Retryable |
|---|---|---|
| `LOCK_BUSY` | another operation holds the VM lock | yes |
| `LOCK_TIMEOUT` | waited for the lock past the deadline | yes |
| `PLAN_EXPIRED` | the execution plan's deadline passed | no — replan |
| `PLAN_MISMATCH` | plan arguments differ from current state | no — replan |
| `OPERATION_NOT_FOUND` | no journal entry for
