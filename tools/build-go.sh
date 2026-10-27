#!/bin/bash
# Build all Go tool binaries in-place (needed for compare-backends.sh
# and manual CLI parity checks).
export PATH="${HOME}/go/bin:/usr/local/go/bin:${PATH}"
cd "$(dirname "${0}")/.." || exit 1
RC=0
for d in tools/quickemu-*/; do
    [ -f "${d}go.mod" ] || continue
    name=$(basename "${d}")
    if (cd "${d}" && go build -o "${name}" . 2>&1); then
        echo "built ${name}"
    else
        echo "FAIL ${name}"
        RC=1
    fi
done
exit ${RC}
