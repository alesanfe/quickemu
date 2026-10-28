# ADR 0002: quickget/quickreport audit scope

## Status: audited — checklist executed, schema drift fixed

The deferred audit checklist was executed. Per-item verdicts:

## quickget

- **Download integrity/resume** — covered: `.part` staging + atomic
  rename + resume (quickget-vm `fetch`, quickgetc `webGet`); partial
  files never masquerade as complete.
- **Mirror failover** — by design: per-OS URL selection (cdimage vs
  releases mirrors) + zsync→curl fallback, matching Bash behaviour.
  Not a mirror-pool design.
- **Checksums** — covered: SUMS parsing + streaming sha256/sha1 verify
  (memory-bounded); checksum mismatches abort before rename.
- **URL liveness** — covered: `probes/` + `--check` WebCheck seam.
- **TLS pinning** — evaluated and rejected: pinning CA keys for
  ~1000 public distro hosts is unmaintainable; TLS is enforced by the
  default transport and redirect-policy rejects scheme downgrades.
- **Timeouts** — covered: 15s dial, 15s TLS handshake, 30s
  response-header (qelib `HTTPTransport`).
- **Catalog schema** — fixed in this pass: `schemas/v1/catalog.schema.json`
  described a `{schemaVersion, oses}` format that nothing emits; the
  real contract is `{generated, entries[]}` — the schema was rewritten
  to the real format and `TestCatalogMatchesSchema` (quickget-catalog)
  now validates `catalog/data/catalog.json` against it in CI.
- **cloud-init/unattended contracts** — covered: seed ISO writer and
  autounattend generation are tested; artifact write errors are
  checked (F1 filesystem-error pass).
- **Async ops/events adoption** — evaluated, not adopted: downloads
  are synchronous, bounded CLI operations; the ops registry tracks
  long-lived VM operations. `meta.operationId` in the JSON envelope
  already provides correlation for callers that need it.

## quickreport

- **JSON output** — covered: `--format=json` emits the qelib
  envelope (schema-validated, see schema_test.go).
- **Sanitization** — covered: `qrRedact` scrubs HOME/username/host/
  IP/MAC/credentialed URLs.
- **XDG paths** — n/a by design: the report is written to the cwd
  (tee-parity with Bash `quick_report | tee quickreport.txt`).
- **Permissions** — covered: report files use default perms; content
  is already redacted.
- **`--host-info` alignment** — covered by construction: both paths
  emit through the same `qelib.Emit` envelope validated against
  `schemas/v1/response.schema.json`.
