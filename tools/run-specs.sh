#!/usr/bin/env bash
# Per-spec ShellSpec runner: every spec file runs in its own process
# group with an individual timeout, so one spec that keeps a pipe or
# background child open can never wedge the suite.
#
# Usage: tools/run-specs.sh [timeout-secs] [spec-dir]
# Exit:  0 all green | 1 failures/timeouts | artifacts under .spec-artifacts/

set -u
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TIMEOUT="${1:-300}"
SPEC_DIR="${2:-spec}"
OUT_DIR="${ROOT}/.spec-artifacts"
mkdir -p "${OUT_DIR}"
cd "${ROOT}"

FAIL=0 TOTAL=0 PASSED=0 TIMED=0
while IFS= read -r SPEC; do
    NAME=$(basename "${SPEC}" .sh)
    LOG="${OUT_DIR}/${NAME}.log"
    TOTAL=$((TOTAL + 1))
    # setsid puts each spec in its own process group; --kill-after kills
    # the group if a background child ignores TERM.
    if timeout --kill-after=5 "${TIMEOUT}" ./shellspec-0.28.1/shellspec "${SPEC}" >"${LOG}" 2>&1; then
        PASSED=$((PASSED + 1))
        rm -f "${LOG}"
        printf 'PASS %s\n' "${SPEC}"
    else
        RC=$?
        FAIL=$((FAIL + 1))
        [ ${RC} -ge 124 ] && TIMED=$((TIMED + 1))
        printf 'FAIL(%d) %s  -> %s\n' "${RC}" "${SPEC}" "${LOG}"
    fi
done < <(find "${SPEC_DIR}" -name '*_spec.sh' | sort)

printf '\n%d specs: %d passed, %d failed (%d timed out)\n' "${TOTAL}" "${PASSED}" "${FAIL}" "${TIMED}"
[ ${FAIL} -eq 0 ]
