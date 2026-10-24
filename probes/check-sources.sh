#!/usr/bin/env bash
# Live checks against real upstream sources (nightly CI, not PR-blocking).
cd "$(dirname "$0")/.." || exit 1
fail=0
for os in blackarch blissos kaos midnightbsd nobara openmandriva openmediavault opnsense q4os systemrescue truenas-scale vyos; do
    ./bin/quickget --check "$os" >/dev/null 2>&1 || { echo "FAIL $os"; fail=1; }
done
exit $fail
