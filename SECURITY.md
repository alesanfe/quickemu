# Security Policy

## Supported Versions

| Version | Supported |
|---|---|
| master (rolling) | yes |
| Tagged releases | yes (latest only) |

Quickemu follows a rolling-release model. Only the current `master` and the
latest tagged release receive security fixes.

## Reporting a Vulnerability

**Do not open a public issue for security vulnerabilities.**

Report privately via GitHub Security Advisories:
`https://github.com/alesanfe/quickemu/security/advisories/new`

Include:

- quickemu version / commit hash
- OS and QEMU version
- minimal reproduction steps
- impact assessment (what an attacker could achieve)
- suggested fix if you have one

## Response Commitments

| Stage | Target |
|---|---|
| Acknowledgement | 72 hours |
| Severity triage | 7 days |
| Fix or mitigation | 30 days (critical), 90 days (other) |
| Public disclosure | after fix release or 90 days, whichever first |

## Scope

**In scope:**

- Path traversal / escape outside the VM workspace
- Command injection via OS names, releases, editions, or config values
- QMP socket hijacking / PID reuse attacks
- Malicious archives (tar-slip) on import
- Download integrity bypass (tampered checksums, signed-URL abuse)
- Lock/operation-state manipulation

**Out of scope:**

- Bugs in QEMU itself (report to qemu-devel)
- Malicious guest content inside a VM (VMs are untrusted by design)
- Vulnerabilities in third-party OS images (report to the vendor)

## Third-Party Images

The `quickget` catalog links to upstream ISO providers. Trust levels are
documented in `docs/adapter-contract.md` — a download succeeding is not proof
of authenticity unless the source offers signature verification (level A/B).

## Upstream

This repository is a hardening fork of `quickemu-project/quickemu`. Issues
that affect upstream will be coordinated with the upstream maintainers before
disclosure where feasible.
