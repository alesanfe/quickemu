// quickemu-toml: ADR-0001 phase 5 — native TOML parsing, replacing the
// line-oriented Bash parser.
//
// # quickemu TOML schema v1 (QE-110)
//
// A VM configuration TOML document is split into three zones:
//
//	schema_version = 1          # MANDATORY root key — always first.
//
//	# Declarative keys at the root — the canonical set in
//	# declarativeKeys. Every value a .conf can express lives here;
//	// these are the only keys (with [extra]) that round-trip to .conf.
//	guest_os      = "windows"
//	ssh_port      = 2222        # typed values accepted: int, string
//	port_forwards = ["2222:22"] # list keys accept string or string[]
//
//	[extra]                     # unknown .conf keys preserved verbatim
//	my_hook       = "on"        # by `config migrate` — free-form values
//
//	[runtime]                   # OPTIONAL volatile state (pid, bound
//	pid = 4242                  # ports…) — NEVER round-tripped to .conf.
//
// Rules:
//   - schema_version must be the integer 1 (the only defined version).
//     The legacy formatVersion emitted by early migrators is accepted
//     as a deprecated alias and produces a warning.
//   - Only [extra] and [runtime] tables are defined; other tables
//     produce warnings and their keys are ignored by conf emission.
//   - Known root keys are type-checked (int, port, on/off, size, MAC,
//     path, enum, HOST:GUEST list) — see schema.go for the table.
//   - Unknown root keys produce warnings; migrate lifts them to [extra].
//   - [extra]/[runtime] keys are free-form: preserved, never rejected.
//
// # Comment preservation (QE-110 honesty note)
//
// BurntSushi/toml drops comments on decode, so a parse→encode cycle
// loses them. quickemu-toml therefore never round-trips TOML through
// the decoder for emission: `migrate` generates TOML text that carries
// .conf comments across verbatim, `to-conf` re-emits comment lines from
// a source scan, and TomlSet/ConfSet patch the original bytes in place
// — every byte not belonging to the edited key survives untouched.
//
// # Transactional editing (QE-112)
//
// ConfSet/TomlSet implement validate → tmp-file → fsync → atomic
// rename (ADR-0006 via renameio), preserving the original mode bits
// (0600/0644) and guarding concurrent modification through EditGuard
// (mtime+size+SHA-256 snapshot the caller takes before editing).
// Optional history keeps the last N versions in <dir>/.hist.
package main
