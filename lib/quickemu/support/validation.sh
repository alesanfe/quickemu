# shellcheck shell=bash
# Input validation helpers shared across modules.

is_numeric() { # value -> 0 when it is a non-negative integer
    [[ "${1}" =~ ^[0-9]+$ ]]
}
