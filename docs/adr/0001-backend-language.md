# ADR 0001: Keep the Bash backend; no Go/Python rewrite for now

## Context

Quickemu is a Bash wrapper around QEMU. Recent hardening (instance
identity, QMP, TOML config, async ops, structured events, verified
import/export, transactional clone/move/rename) removed the technical
drivers that motivated a rewrite discussion.

## Decision

**Keep the Bash backend.** Do not start a Go/Python rewrite.

## Rationale

- The original pain points (weak process identity, no structured
  control channel, executable config, no operation tracking) are now
  solved in Bash: QMP, vm_state, toml_parse, ops registry, JSONL events.
- A rewrite would duplicate the entire surface area (~15k lines of
  OS-specific launch logic across ~1000 supported OSes) for years of
  compatibility burden.
- The remaining scale limits (hundreds of concurrent VMs, daemon mode)
  are not proven requirements today.

## Conditions to revisit

- CI suite flaky beyond the per-spec-runner fix.
- Measured perf problems (boot dispatch >500ms) on real workloads.
- A daemon/remote-API requirement emerging from Quickgui adoption.
- If so: prefer *incremental extraction* (host probes, port registry,
  QMP client as a Go helper binary consumed via the JSON contract)
  over a total rewrite.
