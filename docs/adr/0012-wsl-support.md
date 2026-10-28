# ADR 0012: WSL is experimental — detect, degrade to TCG, document

## Status: accepted

## Context
The tools run under WSL2, but KVM availability is not guaranteed:
`/dev/kvm` may exist while ioctls fail with EPERM (observed after a
WSL reboot), and WSL1 has no KVM at all. Treating "linux" as
"KVM-capable" produced ambiguous 90-second hangs instead of a clear
diagnostic.

## Decision
WSL is a **detected, experimental** platform. `qelib.IsWSL` detects it
via `WSL_DISTRO_NAME`/`WSL_INTEROP` or "microsoft" in
`/proc/sys/kernel/osrelease` (fixture-seamed as `qelib.ProcOSRelease`).

`quickemu-launch` reality-checks the `accel=kvm` default against the
host: on Darwin it resolves to `hvf`; when `/dev/kvm` is unusable
(WSL1, containers, VT-x off) it falls back to TCG and prints a
warning — WSL hosts get a hint about nested virtualization instead of
an opaque QEMU ioctl failure. `QEMU_ACCEL`/`qemu_accel` still override.

## Alternatives
- Officially supported — rejected: KVM quality depends on the Windows
  build, out of our control.
- Rejected outright — rejected: dev usage on WSL is real and TCG works.

## Consequences
- (+) Clear error vs opaque hang; TCG fallback is explicit.
- (+) e2e tests remain runnable under TCG on WSL CI.
- (−) Users may run VMs slowly without noticing the warning.

## Review when
WSL2 ships reliable nested virtualization on the supported matrix.
