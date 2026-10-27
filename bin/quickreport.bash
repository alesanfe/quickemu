#!/usr/bin/env bash

# Library modules — bin/../lib/quickreport resolves identically in the
# source tree and when installed as <prefix>/bin/quickreport.
QR_BINDIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
QR_LIBDIR="$(cd "${QR_BINDIR}/../lib/quickreport" 2>/dev/null && pwd)"
if [ ! -d "${QR_LIBDIR}" ]; then
    echo "ERROR! quickreport library not found: ${QR_LIBDIR}" >&2
    exit 1
fi

# shellcheck source=lib/quickreport/host/probes.sh
source "${QR_LIBDIR}/host/probes.sh"
# shellcheck source=lib/quickreport/cli/report.sh
source "${QR_LIBDIR}/cli/report.sh"

OS_KERNEL=$(uname -s)

# --format json emits the structured envelope; default is the human
# report. All output passes through qr_redact (HOME/user/host/IP/MAC/
# credentialed-URL scrubbing).
if [ "${1:-}" = "--format=json" ] || { [ "${1:-}" = "--format" ] && [ "${2:-}" = "json" ]; }; then
    quick_report_json | tee quickreport.json
else
    clear
    quick_report | qr_redact | tee quickreport.txt
fi
