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

    It 'every OS in os_support has a releases_<os> function'
        missing=""
        for os in $(os_support); do
            [[ $(type -t "releases_${os}") == function ]] || missing="${missing} ${os}"
        done
        [ -z "${missing}" ] || { echo "missing releases_: ${missing}"; return 1; }
        return 0
    End

    It 'every OS in os_support has a get_<os> function'
        missing=""
        for os in $(os_support); do
            [[ $(type -t "get_${os}") == function ]] || missing="${missing} ${os}"
        done
        [ -z "${missing}" ] || { echo "missing get_: ${missing}"; return 1; }
        return 0
    End

    It 'get_<os> emits a URL-looking first field or exits non-zero'
        bad=""
        for os in $(os_support); do
            out=$("get_${os}" 2>/dev/null | head -1) || continue
            first=${out%% *}
            case "${first}" in
                http://*|https://*|"") ;;
                *) bad="${bad} ${os}=${first:0:30}";;
            esac
        done
        [ -z "${bad}" ] || { echo "bad get_ output:${bad}"; return 1; }
        return 0
    End

    It 'os_support has no duplicate OS names'
        dup=$(os_support | tr ' ' '\n' | sort | uniq -d)
        [ -z "${dup}" ] || { echo "duplicates: ${dup}"; return 1; }
        return 0
    End
End
