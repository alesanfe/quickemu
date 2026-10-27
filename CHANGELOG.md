# Changelog

Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
Versioning follows the structured-contract version in `schemas/v1/`.

## Unreleased

### Added
- SECURITY.md, CODE_OF_CONDUCT.md, threat-model, supported-platforms,
  error-codes, adapter-contract docs (QE-09/04/03 governance)
- `quickget --minimum-trust <A-E>` — enforce source integrity policy
- `lib/quickget/catalog/trust.sh` — trust levels inferred/overridable
- `spec/e2e/qemu_boot_spec.sh` — real-QEMU E2E (TCG, skip-gated)
- `spec/e2e/uat_scenarios_spec.sh` — acceptance scenarios (QE-14)
- `lib/quickemu/cli/verbosity.sh` — quiet/normal/verbose/debug levels
- `docs/quality/benchmarks.md` — performance baselines (QE-12)
- QMP socket owner/symlink validation + command allowlist (QE-04)
- CI: e2e job + `actions/checkout` pinned to SHA (QE-10)

### Fixed
- `web_check` falls back to `GET --range 0-0` on HEAD-403 (presigned URLs)
- `validate.sh` normalizes CRLF in releases/editions lists (Windows jq.exe)
- `elementary` moved to dl.elementaryos.org CDN (8.1)
- `securityonion` via GitHub releases tag (host migrated to SPA)
- `kicksecure` rewritten for libvirt 18 bundle layout
- `uwuntu` via archive.org (site moved to MediaFire)

## [4.9.9] - structured-contract version

- Initial hardening fork of quickemu-project/quickemu
- JSON v1 contract with meta.operationId
- Stable error codes (LOCK_BUSY, PLAN_EXPIRED, OPERATION_NOT_FOUND, PORT_BUSY)
- ~165 OS catalog, verified on real Linux
