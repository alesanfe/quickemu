#!/bin/bash
# QE-01: minimal bash coverage baseline via PS4/XTRACE instrumentation.
# Wraps each shellspec run and records which lines of lib/ were executed.
# Output: coverage-bash.txt with "file:line hits" entries (approximate).
# Usage: scripts/coverage-bash.sh > coverage-bash.txt
set -euo pipefail
REPO="${1:-.}"
COV="${REPO}/.coverage"
rm -rf "${COV}" && mkdir -p "${COV}"

# Instrument: a tiny PS4 that logs "file:line" per executed command.
# This gives approximate line coverage — branch precision would need
# a real tracer (kcov).
run_instrumented() {
    export PS4='+${BASH_SOURCE}:${LINENO} '
    export BASH_XTRACEFD=9
    ( exec 9>"${COV}/trace.log"; ./shellspec-0.28.1/shellspec 2>/dev/null || true )
    unset BASH_XTRACEFD PS4
}

run_instrumented

# Tally executed file:line pairs under lib/ only
grep -oE 'lib/[^:]+:[0-9]+' "${COV}/trace.log" 2>/dev/null | \
    sort -u | awk -F: '{print $1}' | sort | uniq -c | \
    awk '{printf "%6d %s\n", $1, $2}'

echo "---"
echo "approximate line coverage (lib/): $(grep -oE 'lib/[^:]+:[0-9]+' "${COV}/trace.log" 2>/dev/null | sort -u | wc -l) lines touched"
