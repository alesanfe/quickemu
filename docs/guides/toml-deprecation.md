# .conf → TOML: double-read period & deprecation plan

## Phase 0 — now (4.9.9+)

Both formats fully supported. `.conf` wins when both exist unless
`--vm x.toml` is explicit. `--migrate-conf` emits the TOML (or
`--dry-run` diff) without touching the source.

## Phase 1 — double-read (next minor)

When both `x.conf` and `x.toml` exist, quickemu parses **both** and
warns on divergence: `WARNING! x.conf and x.toml disagree on ram
(4G vs 8G) — using .toml`. This surfaces drift during migration.

## Phase 2 — prefer TOML (stable)

TOML becomes the default when both exist; `.conf` still works.
QuickGUI writes TOML only.

## Phase 3 — .conf deprecated (next major)

`.conf` files boot with a deprecation warning pointing at
`--migrate-conf`. No execution-format removal until the ecosystem
(quickget generators, docs, tooling) has fully moved.

## Phase 4 — removal (far future, ADR required)

Only after a full deprecation cycle and a QuickGUI release that no
longer writes .conf.

## Writing config from tools

Always TOML (`x.toml`), never emit `.conf` — it is executable Bash.
`quickemu-toml parse|validate|get` is the native reader.
