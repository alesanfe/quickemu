# ADR 0001: Structured CLI contract (JSON output)

## Status: accepted

## Context
Downstream tools (GUIs, scripts) parsed human output, which broke on
any formatting change and offered no capability negotiation.

## Decision
quickemu publishes a versioned contract: `--format json` on
`--status`, `--list-vms`, `--delete-plan`, `--dry-run`, plus a
`--capabilities` descriptor. Schemas live in `schemas/*-v1.schema.json`.
Exit codes are stable (0=ok, 1=error). Human output is unchanged.

## Consequences
- Consumers check `schemaVersion`/capabilities, not version strings.
- New capabilities may be added without breaking v1 consumers.
