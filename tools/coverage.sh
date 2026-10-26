#!/usr/bin/env bash
# Coverage report (matrix item 37): map every lib/* function to spec
# files that mention it. Crude but honest — a function never named by
# any spec is a coverage hole.
set -u
total=0; covered=0
for lib in lib/quickemu/**/*.sh; do
    for fn in $(grep -oP '^function \w+|^\w+\(\) ' "${lib}" | grep -oP '\w+' | sort -u); do
        total=$((total+1))
        grep -rl "${fn}" spec/ >/dev/null 2>&1 && covered=$((covered+1))
    done
done
pct=$(( covered * 100 / (total > 0 ? total : 1) ))
echo "functions: ${total}  covered-by-spec: ${covered}  coverage: ${pct}%"
[ "${pct}" -ge 60 ] || echo "WARN: coverage below 60%"
