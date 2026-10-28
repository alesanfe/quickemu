#!/usr/bin/env bash
# Go quality gate — vet, tests (race when cgo available, with
# coverage), build for every Go module under tools/.
# GOWORK=off: modules resolve their own go.sum (the workspace exists
# for vendored builds; workspace coverage merging needs toolchain
# tools absent from the module-shipped toolchain).
set -u
cd "$(dirname "${BASH_SOURCE[0]}")/.." || exit
export PATH="${PATH}:${HOME}/go/bin:/usr/local/go/bin"
export GOWORK=off
RC=0
for D in tools/*/; do
    [ -f "${D}/go.mod" ] || continue
    echo "=== ${D}"
    RACE=""
    command -v gcc >/dev/null 2>&1 && RACE="-race"
    (cd "${D}" && go vet ./... && \
        go test ${RACE} -coverprofile=coverage.out ./... && \
        CGO_ENABLED=0 go build -o "/tmp/$(basename "${D%/}")" .) || RC=1
    command -v govulncheck >/dev/null 2>&1 && \
        { (cd "${D}" && govulncheck ./...) || RC=1; }
    command -v staticcheck >/dev/null 2>&1 && \
        { (cd "${D}" && staticcheck ./...) || RC=1; }
done
exit "${RC}"
