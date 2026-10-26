# Vision

Quickemu is a **CLI VM manager** (Bash today, Go helpers behind the
same contract) that also serves as the **backend for QuickGUI** via
a stable machine-readable v1 surface.

## Is

- A shell-first automation layer over QEMU (no daemon).
- A structured-API backend: envelopes, ops, events, plans.
- An incremental Go migration target (ADR-0001).

## Is not

- A virtualisation platform / daemon / hypervisor manager.
- A REST/gRPC service (2.0 scope, needs an ADR).
- A replacement for libvirt where lifecycle orchestration is needed.

## Non-negotiables

- Bash compatibility: every Go path falls back to the shell one.
- The contract (`schemas/v1/`) is stable from 4.9.9 — changes are
  additive or versioned, never silently breaking.
- Exit codes and `errors[].code` are the API, not stderr text.
