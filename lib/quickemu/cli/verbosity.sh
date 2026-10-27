# QE-13: output levels — quiet / normal / verbose / debug.
# Level is set via --verbosity or QUICKEMU_VERBOSITY env; defaults to normal.
# In JSON mode the contract envelope is unaffected (levels only apply to
# human-readable progress/log output).

# Ordered levels — numeric comparison only.
_verbosity_rank() {
    case "${1:-normal}" in quiet) echo 0;; normal) echo 1;; verbose) echo 2;; debug) echo 3;; *) echo 1;; esac
}

qe_log_enabled() { # level -> 0 when the message should be emitted
    local req; req=$(_verbosity_rank "${1}")
    local cur; cur=$(_verbosity_rank "${QUICKEMU_VERBOSITY:-${VERBOSITY:-normal}}")
    [ "${cur}" -ge "${req}" ]
}

# Emit a level-gated log line to stderr (never stdout — keeps JSON pure).
qe_log() { # level message
    qe_log_enabled "${1}" || return 0
    printf '%s\n' "${2}" >&2
}

# Convenience wrappers.
qe_debug()   { qe_log debug "${1}"; }
qe_verbose() { qe_log verbose "${1}"; }
qe_info()    { qe_log normal "${1}"; }
qe_quiet()   { qe_log quiet "${1}"; }  # always emitted (essential)
