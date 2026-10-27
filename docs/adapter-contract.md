# Adapter Contract — catalog OSes

Every `get_<os>`/`releases_<os>`/`editions_<os>`/`arch_<os>`/`os_info` entry
must satisfy this contract before it ships.

## Required functions

| Function | Contract |
|---|---|
| `os_info` | name, homepage, license, description — verbatim, no scraping |
| `releases_<os>` | space-separated supported releases, descending; may scrape stable endpoints |
| `editions_<os>` | optional; only emit when the OS actually has editions |
| `arch_<os>` | optional; default `amd64` when omitted |
| `get_<os>` | emit `URL [HASH]` — one line, no side-effects, no `cd` |

## Trust levels

| Level | Definition | Warning shown |
|---|---|---|
| **A** | GPG signature verified before use | none |
| **B** | sha256/sha512 checksum fetched from an independent HTTPS channel | none |
| **C** | checksum served from the same host as the ISO | none |
| **D** | HTTPS only, no checksum available | `warning: integrity not verifiable` |
| **E** | community mirror or unverifiable host | `warning: unverifiable source` |

The user may require a minimum with `quickget --minimum-trust <level>`.
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

- `releases_<os>`/`editions_<os>`/`get_<os>` may scrape index pages or APIs, but the endpoint must be stable.
- A scrape returning empty must emit nothing — the validation path treats empty as "unsupported", never as PASS.
