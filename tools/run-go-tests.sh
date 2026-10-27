#!/bin/bash
# Run go test -race -cover for every tool package. Used by go-check
# locally and by CI. Exits non-zero if any package fails.
export PATH="${HOME}/go/bin:/usr/local/go/bin:${PATH}"
cd "$(dirname "${0}")/.." || exit 1
RC=0
for d in tools/quickemu-*/; do
    [ -f "${d}go.mod" ] || continue
    name=$(basename "${d}")
    out=$(cd "${d}" && go test -race -cover ./... 2>&1) || RC=1
    echo "== ${name}"
    echo "${out}" | tail -2
done
exit ${RC}
