# ADR 0002: quickget/quickreport audit scope (deferred by design)

## Status: inventory only — no changes made

The five uncommitted foreign files belong to these tools:
`.github/workflows/{ci,lint-shellcheck,test-build-quickemu,test-quickget}.yml`,
`packaging/debian/control` — user's pending work, kept untouched.

## Deferred audit checklist (for a dedicated pass)

- quickget: download integrity/resume, mirror failover, checksums,
  URL liveness (probes/ exists), TLS pinning, timeouts, catalog schema,
  cloud-init/unattended output contracts, async ops/events adoption.
- quickreport: JSON output, sanitization, XDG paths, permissions,
  alignment with --host-info schema.
