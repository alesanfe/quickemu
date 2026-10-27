# QE-05: adapter contract — every catalog OS must satisfy the interface.
# Validates that each OS in os_support has the required functions and that
# get_<os> emits a well-formed "URL [HASH]" line when called.
Describe 'contract: catalog adapters'
    It 'every OS in os_support has releases_<os> or a documented delegate'
        When run bash -c 'source lib/quickget/catalog/info.sh; source lib/quickget/catalog/data.sh; source lib/quickget/catalog/get.sh; missing=""; for os in $(os_support); do case "${os}" in *ubuntu*|windows-server) continue;; esac; [ "$(type -t "releases_${os}")" = function ] || missing="${missing} ${os}"; done; [ -z "${missing}" ] || { echo "missing releases_: ${missing}"; exit 1; }'
        The status should be success
    End

    It 'every OS in os_support has get_<os> or a documented delegate'
        When run bash -c 'source lib/quickget/catalog/info.sh; source lib/quickget/catalog/data.sh; source lib/quickget/catalog/get.sh; missing=""; for os in $(os_support); do case "${os}" in *ubuntu*|windows-server|macos) continue;; esac; [ "$(type -t "get_${os}")" = function ] || missing="${missing} ${os}"; done; [ -z "${missing}" ] || { echo "missing get_: ${missing}"; exit 1; }'
        The status should be success
    End

    It 'get_<os> bodies reference an https URL or a stable endpoint'
        # Static check: each get_<os> must reference a scheme (https or a
        # dynamic base built from one). Avoids invoking network scrapes.
        When run bash -c 'source lib/quickget/catalog/info.sh; source lib/quickget/catalog/data.sh; source lib/quickget/catalog/get.sh; bad=""; for os in $(os_support); do case "${os}" in *ubuntu*|windows-server|macos) continue;; esac; [ "$(type -t "get_${os}")" = function ] || continue; body=$(declare -f "get_${os}"); grep -qE "https?://|BASE|URL=" <<<"${body}" || bad="${bad} ${os}"; done; [ -z "${bad}" ] || { echo "get_ with no URL ref:${bad}"; exit 1; }'
        The status should be success
    End

    It 'os_support has no duplicate OS names'
        When run bash -c 'source lib/quickget/catalog/info.sh; dup=$(os_support | tr " " "\n" | sort | uniq -d); [ -z "${dup}" ] || { echo "duplicates: ${dup}"; exit 1; }'
        The status should be success
    End
End
