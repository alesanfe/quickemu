# ShellSpec helpers — shared across specs.
export QE_ROOT="${SHELLSPEC_PROJECT_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)}"

# Run a library function in a subshell so spec-scoped globals
# (VMNAME, VM_PID, OUTPUT_FORMAT...) cannot leak between examples.
qe_in_subshell() { ( "$@" ); }

# Kill leftover fake background helpers (fake-VM `sleep`s, monitor
# listeners) whose pids were recorded under TESTDIR, then remove TESTDIR.
# Without this a stopped (SIGSTOP'd) child keeps the runner's pipes open
# forever and deadlocks the suite; live ones stall each example's EOF.
qe_spec_cleanup() {
    local pidfile pid pids
    # Snapshot the job table then disown: killing a listed job prints a
    # 'Killed' job-status message on stderr which ShellSpec flags as an
    # after-hook error. Disowned children are reaped silently.
    pids=$(jobs -p)
    disown -a 2>/dev/null
    for pidfile in "${TESTDIR:-/nonexistent}"/*/*.pid "${TESTDIR:-/nonexistent}"/*.pid; do
        [ -f "$pidfile" ] || continue
        pid=$(cat "$pidfile" 2>/dev/null) && [ -n "$pid" ] && pids="${pids} ${pid}"
    done
    # shellcheck disable=SC2086
    [ -n "${pids}" ] && kill -9 ${pids} 2>/dev/null
    rm -rf -- "${TESTDIR:?}"
}
