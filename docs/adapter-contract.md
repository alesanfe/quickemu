# Adapter Contract — catalog OSes

Every OS family is a JSON entry in `catalog/data/catalog.json` plus an
`os_info` row in `catalog/data/info.json`. Dynamic families add a
resolver case in `tools/quickget-catalog/scrape*.go` instead of static
`url`/`hash` templates.

## Required fields

| Field | Contract |
|---|---|
| `os_info` (info.json) | name, homepage, license, description — verbatim, no scraping |
| `releases` | supported releases, descending; may scrape stable endpoints |
| `editions` | optional; only declare when the OS actually has editions |
| `arch` | optional; default `amd64` when omitted — only declare an arch when artifacts exist for it |
| `url`/`hash` templates or `"scrape": true` + resolver | resolver emits `URL [HASH]` — one artifact, no side-effects |

## Trust levels

| Level | Definition | Warning shown |
|---|---|---|
| **A** | GPG signature verified before use | none |
| **B** | sha256/sha512 checksum fetched from an independent HTTPS channel | none |
| **C** | checksum served from the same host as the ISO | none |
| **D** | HTTPS only, no checksum available | `warning: integrity not verifiable` |
| **E** | community mirror or unverifiable host | `warning: unverifiable source` |

The user may require a minimum with `quickget --minimum-trust <level>`
(or `MINIMUM_TRUST` env). The gate fails closed at **D** by default:
level E — a source the trust probe could not even grade — is refused
unless the user explicitly passes `--minimum-trust E`. D proceeds with
the warning shown above.
Level D/E sources are documented per-OS in the catalog entry's comment.

## URL rules

- HTTPS only — `--proto '=https'` enforced in `web_pipe`.
- No JS-wrapped downloaders (mega.nz, MediaFire, upfiles) — use archive.org or the provider's official mirror.
- No torrent-only artifacts — those belong to `--import`.
- No commercial/login-gated sources.
- Preserve URL-encoding for filenames with spaces/brackets (`%20`, `%5B`).
- Signed URLs (S3/R2) are allowed for download but `web_check` validates them with `GET --range 0-0`, not HEAD.

## Filename rules

- Match the real upstream filename exactly (case-sensitive).
- If the naming changes per release, use a `case` on `${RELEASE}` with a comment, not a guess.
- If the mirror carries both install and live ISOs, prefer install.

## Verification before merge

- `quickget --check <os> <release> <edition>` must PASS for every declared release+edition on real Linux.
- Entries verified only on Windows Git-Bash must be re-verified on real Linux.

## Dynamic entries

- `releases`/`editions`/resolvers may scrape index pages or APIs, but the endpoint must be stable.
- A scrape returning empty must emit nothing — the validation path treats empty as "unsupported", never as PASS.
- Every resolver branch needs an offline fixture in `tools/quickget-catalog/scrape2_test.go` — tests must not depend on the live network.
