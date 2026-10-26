#!/usr/bin/env bash
# Phase H: Go quality gate — vet, tests (race with cgo), build,
# govulncheck/staticcheck when present.
set -u
cd "$(dirname "${BASH_SOURCE[0]}")/.."
export PATH="${PATH}:${HOME}/go/bin:/usr/local/go/bin"
RC=0
for D in tools/quickemu-*/; do
    [ -f "${D}/go.mod" ] || continue
    echo "=== ${D}"
    RACE=""
    command -v gcc >/dev/null 2>&1 && RACE="-race"
    (cd "${D}" && go vet ./... && go test ${RACE} ./... &&         CGO_ENABLED=0 go build -o "/tmp/$(basename "${D%/}")" .) || RC=1
    command -v govulncheck >/dev/null 2>&1 &&         { (cd "${D}" && govulncheck ./...) || RC=1; }
    command -v staticcheck >/dev/null 2>&1 &&         { (cd "${D}" && staticcheck ./...) || RC=1; }
done
exit "${RC}"
