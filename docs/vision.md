# Vision

Quickemu is a **CLI VM manager** (Go backend in `tools/`) that also
serves as the **backend for QuickGUI** via a stable machine-readable
v1 surface.

## Is

- A scriptable automation layer over QEMU (no daemon).
- A structured-API backend: envelopes, ops, events, plans.
- A single Go workspace (ADR-0003/0015 superseded).

## Is not

- A virtualisation platform / daemon / hypervisor manager.
- A REST/gRPC service (2.0 scope, needs an ADR).
- A replacement for libvirt where lifecycle orchestration is needed.

## Non-negotiables

- On-disk compatibility: every artifact (`.conf`, TOML, ops/ports
  registries, socket names) matches the formats frozen in ADR 0011.
- The contract (`schemas/v1/`) is stable from 4.9.9 — changes are
  additive or versioned, never silently breaking.
- Exit codes and `errors[].code` are the API, not stderr text.
