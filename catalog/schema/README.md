# OS catalog data

Target layout for the progressive catalog refactor (Phase 3): one
JSON file per OS, validated against `catalog-entry.schema.json`.
quickget remains the runtime source until `lib/catalog/` consumes
these files. `tools/check-catalog` validates `quickget --list-json`
output today.
