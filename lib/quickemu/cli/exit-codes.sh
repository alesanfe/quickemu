# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Sourced partial: shares the conf-schema global namespace with bin/quickemu.
# shellcheck disable=SC2034 # constants consumed by the main script
# Stable exit codes (public contract).
readonly QE_EXIT_OK=0
readonly QE_EXIT_ERROR=1
