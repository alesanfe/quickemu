# ADR 0001: Bash backend — kept, with measurable extraction triggers

## Context

Quickemu is a Bash wrapper around QEMU. Hardening this cycle (instance
identity, QMP, TOML, async ops, structured events, verified archives,
transactional ops) removed the original rewrite drivers.

## Decision

**Keep Bash as the primary backend.** Extraction to Go happens only
when a measurable trigger below fires, and always incrementally
(helper binary behind the existing JSON contract — never a rewrite).

## Extraction triggers (any one fires → open the phase)

- **Protocol**: QMP needs >1 concurrent request, or event-multiplexing
  that Bash cannot express without races.
- **Concurrency**: an async-op race reproduced in tests that locks
  cannot fix; port-registry TOCTOU returns after the critical section.
- **Portability**: a function needs >3 GNU-vs-BSD fallbacks (ps, stat,
  date, readlink) to stay correct.
- **Performance**: `list_vms` >500ms at 100 VMs; `toml_parse` >200ms
  on a 100-key file; events write >50ms/record.
- **Security**: a requirement to `eval` config, or descriptor-inheritance
  control Bash cannot provide.

## Extraction order (only when a trigger fires)

1. `quickemu-qmp` — QMP client helper (connect/handshake/execute/watch;
   JSON envelope on stdout, logs on stderr). **DONE** — `tools/quickemu-qmp`
   (Go 1.22, stdlib only, 5 unit tests with a fake-QMP socket). Wired:
   `qmp_command` prefers the binary, falls back to socat.
2. Process identity reader (pid/starttime/uid/tree verification).
3. Async-op registry worker.
4. Port registry.
5. TOML parser — only if measurable parser faults appear.
6. Archive/import helpers — only if limits appear.

Each phase is a standalone Go helper behind the JSON envelope
(schemaVersion, command, data, warnings, errors, meta.backendVersion).
`QUICKEMU_BACKEND=compare` runs old+new on fixtures and diffs envelopes
before Bash code is retired; the helper-missing case keeps Bash
fallback during a defined transition window.

## Language criteria

- **Go**: anything concurrent or protocol-persistent (QMP, ops, ports,
  daemon-adjacent). Single binary, no runtime dep.
- **Python**: migration/validation tooling only, never hot path.
- Never both in the runtime without an ADR.

## Review

Re-evaluate at each release candidate or when a trigger fires.
