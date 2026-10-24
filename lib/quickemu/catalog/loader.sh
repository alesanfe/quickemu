# shellcheck shell=bash
# Catalog loader: wraps `quickget --list-json` into the v1 catalog shape.
catalog_list_json() {
    "${QUICKGET:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)/quickget}" --list-json
}
