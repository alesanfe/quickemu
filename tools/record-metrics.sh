#!/bin/bash
# QE-08/CMMI-4: append a metrics record to docs/quality/history.jsonl.
# Called by CI after each run; the committed file accumulates history.
# Usage: tools/record-metrics.sh [key=value ...]
set -u
cd "$(dirname "${0}")/.." || exit 1
OUT=docs/quality/history.jsonl
mkdir -p "$(dirname "${OUT}")"

kv() {
    local key="${1}" val="${2}"
    printf '"%s":"%s",' "${key}" "${val}"
}

ts=$(date -u +%Y-%m-%dT%H:%M:%SZ)
commit=$(git rev-parse --short HEAD 2>/dev/null || echo unknown)
branch=$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo unknown)

# Collect metrics available locally; CI passes the rest via args.
cov=$(bash tools/coverage.sh 2>/dev/null | grep -oE '[0-9]+%' | head -1 || echo "0%")
specs=$(find spec -name '*_spec.sh' | wc -l)
os_count=$(bash bin/quickget --list 2>/dev/null | tr ' ' '\n' | sort -u | wc -l || echo "0")

printf '{%s}\n' "$(kv timestamp "${ts}")$(kv commit "${commit}")$(kv branch "${branch}")$(kv coverage_bash "${cov}")$(kv spec_files "${specs}")$(kv catalog_os "${os_count}")${*:+$(for a in "$@"; do kv "${a%%=*}" "${a##*=}"; done)}" | sed 's/,}/}/' >> "${OUT}"
tail -1 "${OUT}"
