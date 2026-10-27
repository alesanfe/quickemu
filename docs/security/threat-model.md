# Threat Model — quickemu / quickget

Scope: the Bash CLI + Go backend + quickget catalog + quickemu runtime.
Assets at risk: the host filesystem, QEMU processes, downloaded images,
user config, and the host network (port forwards, QMP).

## Actors

- Local unprivileged attacker (another user on the host)
- Malicious mirror / network attacker (MitM, DNS poisoning)
- Malicious catalog entry (compromised OS metadata)
- Malicious guest (escape via QEMU — upstream concern)
- Fat-fingered user (destructive commands)

## Trust Boundaries

1. **Host filesystem** — VMs live under the user's workspace; nothing may
   write outside the .conf's directory.
2. **Download path** — URL → mirror → checksum → disk; MitM possible.
3. **QMP socket** — local control channel; hijack = VM takeover.
4. **Process table** — PID reuse could make us signal the wrong process.
5. **Import path** — archives extracted to disk; tar-slip risk.
6. **Config file** — sourced as bash; code injection risk if attacker
   controls the .conf.
7. **CI** — workflow inputs could leak credentials or poison artifacts.

## Threats (STRIDE)

| ID | Threat | Mitigation |
|---|---|---|
| T01 | `.conf` code injection via malicious values | `--inspect-conf` parses without source; quoted assignments only |
| T02 | tar-slip on `import_vm` | temp extraction + member validation + atomic mv |
| T03 | delete escapes workspace via symlinked path | realpath confinement under conf dir |
| T04 | PID reuse — signal wrong process | comm=qemu fingerprint + start-time check |
| T05 | lock starvation / orphan lock blocks VM | lock owner metadata + stale-lock detection |
| T06 | QMP socket hijack | private dir 0700 + owner/type check + allowlist |
| T07 | MitM on download | `--proto '=https'` + checksum + optional signature |
| T08 | redirect to attacker domain | domain allowlist per OS entry |
| T09 | HTML error page saved as ISO | content-type + size sanity + magic check |
| T10 | signed-URL signature replay | GET-range check; signature not reused across calls |
| T11 | plan replay after expiry | planId + expiry + mismatch detection |
| T12 | orphan port after crash | port owner recorded; PORT_BUSY handling |
| T13 | env injection (HOSTILE vars) | sanitise PATH-related vars; no eval of env |
| T14 | CI workflow leak | minimal perms + pinned actions + no secrets on PR |

## Residual Risks

- Sources with trust level D/E (mirror-only, no checksum) — user warned.
- QEMU binary integrity — out of scope (system package manager).
- Multi-user hosts — the per-VM socket dir is user-private but a root
  attacker is out of scope by definition.

## Security Invariants (must hold in any fix)

- Never delete outside the workspace.
- Never report success for a partial operation.
- Never reuse a signature, plan, or nonce across requests.
- Never let a hostname/path from the catalog reach the shell unquoted.
