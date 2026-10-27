#!/usr/bin/env bash
# SC2317: Command appears to be unreachable. Check usage (or ignore if invoked indirectly).
#  - https://www.shellcheck.net/wiki/SC2317
#  - Disable globally because many functions are called indirectly
# shellcheck disable=SC2317
export LC_ALL=C

# Detect host OS for checksum tool compatibility
HOST_OS=$(uname -s)

# Default architecture based on host
HOST_ARCH=$(uname -m)
case "${HOST_ARCH}" in
    aarch64|arm64) ARCH="arm64";;
    *) ARCH="amd64";;
esac


# Library modules — bin/../lib/quickget resolves identically in the
# source tree and when installed as /usr/bin/quickget -> /usr/lib/quickget.
QG_BINDIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
QG_LIBDIR="$(cd "${QG_BINDIR}/../lib/quickget" 2>/dev/null && pwd)"
if [ ! -d "${QG_LIBDIR}" ]; then
    echo "ERROR! quickget library not found: ${QG_LIBDIR}" >&2
    exit 1
fi

# shellcheck source=lib/quickget/support/errors.sh
source "${QG_LIBDIR}/support/errors.sh"
# shellcheck source=lib/quickget/support/host.sh
source "${QG_LIBDIR}/support/host.sh"
# shellcheck source=lib/quickget/download/web.sh
source "${QG_LIBDIR}/download/web.sh"
# shellcheck source=lib/quickget/download/windows.sh
source "${QG_LIBDIR}/download/windows.sh"
# shellcheck source=lib/quickget/image/normalize.sh
source "${QG_LIBDIR}/image/normalize.sh"
# shellcheck source=lib/quickget/image/generate.sh
source "${QG_LIBDIR}/image/generate.sh"
# shellcheck source=lib/quickget/image/windows.sh
source "${QG_LIBDIR}/image/windows.sh"
# shellcheck source=lib/quickget/catalog/info.sh
source "${QG_LIBDIR}/catalog/info.sh"
# shellcheck source=lib/quickget/catalog/validate.sh
source "${QG_LIBDIR}/catalog/validate.sh"
# shellcheck source=lib/quickget/catalog/arch.sh
source "${QG_LIBDIR}/catalog/arch.sh"
# shellcheck source=lib/quickget/catalog/data.sh
source "${QG_LIBDIR}/catalog/data.sh"
# shellcheck source=lib/quickget/catalog/get.sh
source "${QG_LIBDIR}/catalog/get.sh"
# shellcheck source=lib/quickget/catalog/trust.sh
source "${QG_LIBDIR}/catalog/trust.sh"
# shellcheck source=lib/quickget/catalog/list.sh
source "${QG_LIBDIR}/catalog/list.sh"
# shellcheck source=lib/quickget/catalog/check.sh
source "${QG_LIBDIR}/catalog/check.sh"
# shellcheck source=lib/quickget/vm/config.sh
source "${QG_LIBDIR}/vm/config.sh"
# shellcheck source=lib/quickget/cli/usage.sh
source "${QG_LIBDIR}/cli/usage.sh"
# shellcheck source=lib/quickget/cli/main.sh
source "${QG_LIBDIR}/cli/main.sh"
