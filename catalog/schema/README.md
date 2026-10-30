# OS catalog data

`catalog/data/catalog.json` is the runtime source, consumed by
`tools/quickget-catalog`. The shipped contract lives in
`schemas/v1/catalog.schema.json` and is enforced by
`TestCatalogMatchesSchema` (tools/quickget-catalog) plus
`qedev check-catalog` against `quickget --list-json` output.

`catalog-entry.schema.json` here mirrors the same entry shape for a
possible future split into one file per OS — and as per-entry
documentation of field semantics (enum meanings, scrape/qarch
flags, template placeholders).
