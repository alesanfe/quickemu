#!/usr/bin/env bash

CACHE_FILE="quickget_data.json.zst"
CACHE_URL="https://github.com/lj3954/quickget_configs/releases/download/daily/quickget_data.json.zst"
IMAGE_TYPES=("iso" "img" "fixed_iso" "floppy")
QUICKGETC_VERSION="0.1.0"


# Library modules — bin/../lib/quickgetc resolves identically in the
# source tree and when installed as <prefix>/bin/quickgetc.
QGC_BINDIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
QGC_LIBDIR="$(cd "${QGC_BINDIR}/../lib/quickgetc" 2>/dev/null && pwd)"
if [ ! -d "${QGC_LIBDIR}" ]; then
    echo "ERROR! quickgetc library not found: ${QGC_LIBDIR}" >&2
    exit 1
fi

# shellcheck source=lib/quickgetc/support/host.sh
source "${QGC_LIBDIR}/support/host.sh"
# shellcheck source=lib/quickgetc/download/cache.sh
source "${QGC_LIBDIR}/download/cache.sh"
# shellcheck source=lib/quickgetc/download/web.sh
source "${QGC_LIBDIR}/download/web.sh"
# shellcheck source=lib/quickgetc/catalog/query.sh
source "${QGC_LIBDIR}/catalog/query.sh"
# shellcheck source=lib/quickgetc/catalog/info.sh
source "${QGC_LIBDIR}/catalog/info.sh"
# shellcheck source=lib/quickgetc/catalog/validate.sh
source "${QGC_LIBDIR}/catalog/validate.sh"
# shellcheck source=lib/quickgetc/catalog/arch.sh
source "${QGC_LIBDIR}/catalog/arch.sh"
# shellcheck source=lib/quickgetc/catalog/list.sh
source "${QGC_LIBDIR}/catalog/list.sh"
# shellcheck source=lib/quickgetc/catalog/check.sh
source "${QGC_LIBDIR}/catalog/check.sh"
# shellcheck source=lib/quickgetc/vm/config.sh
source "${QGC_LIBDIR}/vm/config.sh"
# shellcheck source=lib/quickgetc/cli/usage.sh
source "${QGC_LIBDIR}/cli/usage.sh"
# shellcheck source=lib/quickgetc/cli/main.sh
source "${QGC_LIBDIR}/cli/main.sh"
