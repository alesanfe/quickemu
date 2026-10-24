# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Sourced partial: shares the conf-schema global namespace with bin/quickemu.
# Input validation helpers shared across modules.

is_numeric() { # value -> 0 when it is a non-negative integer
    [[ "${1}" =~ ^[0-9]+$ ]]
}
