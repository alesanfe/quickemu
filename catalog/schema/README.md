# OS catalog data

Target layout for the progressive catalog refactor (Phase 3): one
JSON file per OS, validated against `catalog-entry.schema.json`.
`catalog/data/catalog.json` remains the runtime source, consumed by
`tools/quickget-catalog`. `qedev check-catalog` validates
`quickget --list-json` output today.
