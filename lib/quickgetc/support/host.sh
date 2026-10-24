# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154,SC2317
# Sourced partial: shares the global namespace of bin/quickgetc.
# Support: process/session helpers.


function cleanup() {
    if [ -n "$(jobs -p)" ]; then
        kill "$(jobs -p)" 2>/dev/null
    fi
}
