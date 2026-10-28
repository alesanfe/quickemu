# ADR 0012: WSL is experimental — detect, degrade to TCG, document

## Status: accepted

## Context
The tools run under WSL2, but KVM availability is not guaranteed:
`/dev/kvm` may exist while ioctls fail with EPERM (observed after a
WSL reboot), and WSL1 has no KVM at all. Treating "linux" as
"KVM-capable" produced ambiguous 90-second hangs instead of a clear
diagnostic.

## Decision
WSL is a **detected, experimental** platform: `quickemu-launch` warns
and requires `accel="tcg"` (env `QEMU_ACCEL=tcg`) rather than assuming
KVM works. Detection is via `/proc/sys/kernel/osrelease` containing
"WSL"/"microsoft". TCG runs show a performance warning instead of the
cryptic QEMU ioctl failure.

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
