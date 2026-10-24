# ShellSpec helpers — shared across specs.
export QE_ROOT="${SHELLSPEC_PROJECT_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)}"

# Run a library function in a subshell so spec-scoped globals
# (VMNAME, VM_PID, OUTPUT_FORMAT...) cannot leak between examples.
qe_in_subshell() { ( "$@" ); }
