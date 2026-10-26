# Compatibility Matrix (verified states)

Legend: ✅ tested | 🟡 partial | ⚪ untested | ❌ unsupported | 🧪 experimental

| Combination | State | Notes |
|---|---|---|
| Linux x86_64 host → x86_64 guest | ✅ | specs green; QEMU smoke script |
| Linux x86_64 → aarch64 guest (TCG) | 🟡 | conf path exists; no HW test |
| macOS aarch64 host | 🟡 | code paths present; env untested |
| QEMU ≥6.1 (minimum) | ✅ | version gate enforced |
| QEMU 10.x | 🧪 | passt needs ≥10.1 |
| KVM | ✅ | accel path default on Linux |
| TCG (no KVM) | 🟡 | fallback exists |
| SPICE / GTK / SDL / VNC / none | ✅ | display modes validated |
| TOML configs | ✅ | parse+validate+boot+schema |
| Multi-disk/NIC | ✅ | extra_disks/extra_nics wired |
| Windows guest | 🟡 | config paths exist; needs real boot |
| macOS guest | 🟡 | OpenCore path; needs real boot |
| Ports: TCP/UDP/IPv4 | ✅ | registry covers TCP; UDP parity documented |
| IPv6 listeners | 🟡 | address validation only |
| WSL2 | ✅ | dev environment (this session) |
| Full disk / read-only FS | 🟡 | staging paths exist; needs fault injection |
