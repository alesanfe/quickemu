# Contributing to Quickemu (alesanfe fork)

## Ground rules

- **Bash 4.0+**, ShellCheck clean (CI blocks on severity: warning).
- Conventional Commits (`feat(scope):`, `fix(scope):` — see AGENTS.md).
- Every defect is a regression test BEFORE the fix, not after.
- New behaviour ships with a ShellSpec example in `spec/` plus the
  matching `schemas/v1/*.schema.json` when it emits structured output.

## Run the gate locally

```sh
bash tools/run-specs.sh 180 spec          # per-spec isolated suite
bash tools/go-check.sh                    # vet + test + build helpers
shellcheck -S warning bin/* lib/**/*.sh   # lint
bash tools/compare-backends.sh            # Bash vs Go divergence check
```

## Adding a command

1. Envelope command name goes in `docs/guides/command-map.md` first
   (it's the contract — changing it later is a breaking change).
2. Add the flag in `lib/quickemu/cli/options.sh`, the action in
   `lib/quickemu/cli/commands.sh`, the impl in the right `lib/` layer.
3. Emit `qe_ok`/`qe_fail` with a TYPED code — never freeform errors.
4. State-changing ops: verify `vm_transition_ok <state> <op>` first;
   mutating ops take `vm_lock_acquire`, readers take `shared`.

## Adding a Go helper (ADR-0001)

- Contract v1 envelope identical to Bash; `--raw` for text fallback.
- New deps need: justification vs stdlib, versioned pin in go.sum,
  and a POC behind `-tags poc` when they replace a hot path.
- Run `tools/go-check.sh` — vet, tests (race when cgo), build.

## PRs

- One concern per PR; keep unrelated diffs out.
- CI must be green: shellcheck + specs + build.
- Changes to `schemas/v1/` are breaking — bump the envelope version
  only after a deprecation period (see `toml-deprecation.md` model).
