#!/bin/bash
# Real behavioral coverage via kcov — measures lines executed by the
# spec suite, not functions mentioned. kcov is the standard bash
# coverage tool (the same one bats/spec use).
# Usage: tools/coverage-kcov.sh [specdir] — defaults to spec/
set -u
cd "$(dirname "${0}")/.." || exit 1
command -v kcov >/dev/null || { echo "kcov not installed — apt install kcov or see coverage.sh"; exit 1; }
OUT=$(mktemp -d)
# kcov wraps a command; we run the suite under it. Output has coverage
# per file in ${OUT}/index.html + cobertura XML.
kcov "${OUT}" ./shellspec-0.28.1/shellspec "${1:-spec/}" >/dev/null 2>&1 || true
# Extract the summary: kcov prints coverage percentage in index.html.
cov=$(grep -oE 'covered":"[0-9.]+%?"' "${OUT}/index.html" 2>/dev/null | head -1 | grep -oE '[0-9.]+' || echo "0")
lines=$(find lib/quickemu -name '*.sh' | xargs wc -l | tail -1 | awk '{print $1}')
echo "kcov behavioral coverage: ${cov}% (lib/quickemu, ${lines} lines)"
echo "report: ${OUT}/index.html"
