#!/usr/bin/env bash
# Mutation gate (matrix item 41): inject a semantic mutation into a lib
# file, run the affected spec, and REQUIRE the suite to catch it. If a
# mutation survives, the tests are weak — report the survivor.
set -u
SPEC_DIR="${1:-spec/unit/vm}"
LIB="${2:-lib/quickemu/vm/disk.sh}"
MUT="${LIB}.mut"
cp "${LIB}" "${MUT}"
# Mutation: invert a trivially observable guard (`-f` -> `! -f`).
sed -i 's/\[ -f /[ ! -f /g' "${MUT}"
cp "${MUT}" "${LIB}"
RC=0
if timeout 120 ./shellspec-0.28.1/shellspec "${SPEC_DIR}" >/dev/null 2>&1; then
    echo "MUTANT SURVIVED: suite did not detect inverted -f guards in ${LIB}"
    RC=1
else
    echo "MUTANT KILLED: suite detected the mutation in ${LIB}"
fi
mv "${MUT}" "${LIB}"
exit "${RC}"
