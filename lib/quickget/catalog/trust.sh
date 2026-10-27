# Trust-level resolution for catalog sources (QE-03 / docs/adapter-contract.md).
# Levels: A=signature verified, B=independent-channel checksum, C=same-host
# checksum, D=https-only, E=unverifiable mirror.

# Resolve the trust level for an OS. Optional trust_<os>() override wins;
# otherwise inferred from whether get_<os> emits a HASH field.
function trust_os() {
    local OS="${1}"
    if [[ $(type -t "trust_${OS}") == function ]]; then
        "trust_${OS}"
        return
    fi
    local OUT
    OUT=$("get_${OS}" 2>/dev/null || true)
    [ -n "$(echo "${OUT}" | awk '{print $2}')" ] && echo C || echo D
}

# True when LEVEL meets the --minimum-trust threshold.
function trust_meets_minimum() {
    local LEVEL="${1}" MIN="${MINIMUM_TRUST:-E}"
    local -A RANK=( [A]=5 [B]=4 [C]=3 [D]=2 [E]=1 )
    [ "${RANK[${LEVEL}]:-0}" -ge "${RANK[${MIN}]:-0}" ]
}

# Warn when a source is below a signed/checksummed level.
function trust_warn() {
    local OS="${1}" LEVEL
    LEVEL=$(trust_os "${OS}")
    case "${LEVEL}" in
        D) echo "warning: ${OS} source integrity not verifiable (trust D: https only)" >&2;;
        E) echo "warning: ${OS} source unverifiable (trust E: community mirror)" >&2;;
    esac
}
