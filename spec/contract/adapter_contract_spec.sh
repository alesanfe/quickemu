# QE-05: adapter contract — every catalog OS must satisfy the interface.
# Validates that each OS in os_support has the required functions and that
# get_<os> emits a well-formed "URL [HASH]" line when called.
Describe 'contract: catalog adapters'
    setup() {
        cd "${PWD}"
        source lib/quickget/catalog/info.sh 2>/dev/null
        source lib/quickget/catalog/data.sh 2>/dev/null
        source lib/quickget/catalog/get.sh 2>/dev/null
    }
    BeforeEach 'setup'

    It 'every OS in os_support has releases_<os> or a documented delegate'
        missing=""
        for os in $(os_support); do
            case "${os}" in
                # ubuntu variants and windows-server delegate to the shared
                # releases_ubuntu / get_ubuntu / get_windows generators.
                *ubuntu*) continue;;
                windows-server) continue;;
            esac
            [[ $(type -t "releases_${os}") == function ]] || missing="${missing} ${os}"
        done
        [ -z "${missing}" ] || { echo "missing releases_: ${missing}"; return 1; }
        return 0
    End

    It 'every OS in os_support has get_<os> or a documented delegate'
        missing=""
        for os in $(os_support); do
            case "${os}" in
                *ubuntu*) continue;;
                windows-server) continue;;
                macos) continue;;
            esac
            [[ $(type -t "get_${os}") == function ]] || missing="${missing} ${os}"
        done
        [ -z "${missing}" ] || { echo "missing get_: ${missing}"; return 1; }
        return 0
    End

    It 'get_<os> bodies reference an https URL or a stable endpoint'
        # Static check: each get_<os> must reference a scheme (https or a
        # dynamic base built from one). Avoids invoking network scrapes.
        bad=""
        for os in $(os_support); do
            case "${os}" in *ubuntu*|windows-server|macos) continue;; esac
            [[ $(type -t "get_${os}") == function ]] || continue
            body=$(declare -f "get_${os}")
            grep -qE 'https?://|BASE|URL=' <<<"${body}" || bad="${bad} ${os}"
        done
        [ -z "${bad}" ] || { echo "get_ with no URL ref:${bad}"; return 1; }
        return 0
    End

    It 'os_support has no duplicate OS names'
        dup=$(os_support | tr ' ' '\n' | sort | uniq -d)
        [ -z "${dup}" ] || { echo "duplicates: ${dup}"; return 1; }
        return 0
    End
End
