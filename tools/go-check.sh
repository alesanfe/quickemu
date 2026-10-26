#!/usr/bin/env bash
# Phase H: Go quality gate — vet, race tests, govulncheck, staticcheck.
set -u
cd "$(dirname "${BASH_SOURCE[0]}")/.."
export PATH="${PATH}:${HOME}/go/bin:/usr/local/go/bin"
RC=0
for D in tools/quickemu-*/; do
    [ -f "${D}/go.mod" ] || continue
    echo "=== ${D}"
    local RACE=""
    command -v gcc >/dev/null 2>&1 && RACE="-race"
    (cd "${D}" && go vet ./... && go test ${RACE} ./... &&         CGO_ENABLED=0 go build -o "/tmp/$(basename "${D%/}")" .) || RC=1
    if command -v govulncheck >/dev/null 2>&1; then
        (cd "${D}" && govulncheck ./...) || RC=1
    fi
    if command -v staticcheck >/dev/null 2>&1; then
        (cd "${D}" && staticcheck ./...) || RC=1
    fi
done
exit "${RC}"
