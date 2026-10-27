# shellcheck shell=bash
# Contract conformance: every JSON emitted by the CLI must satisfy the
# matching schemas/v1/*.schema.json — all required keys present, no
# undeclared top-level keys, and schemaVersion equal to the declared const.
Describe 'schema conformance (schemas/v1)'
Include spec/spec_helper.sh

tmpdir() { mktemp -d; }

# assert_json_conforms <schema-path> <json-string>
assert_json_conforms() {
    local schema="$1" json="$2" key props emitted const
    for key in $(jq -r '.required[]? // empty' "${schema}"); do
        if ! jq -e --arg k "${key}" 'has($k)' >/dev/null 2>&1 <<<"${json}"; then
            echo "missing required key: ${key}"
            return 1
        fi
    done
    props=$(jq -r '.properties | keys[]' "${schema}")
    for key in $(jq -r 'keys[]' <<<"${json}"); do
        if ! grep -qx "${key}" <<<"${props}"; then
            echo "undeclared key emitted: ${key}"
            return 1
        fi
    done
    emitted=$(jq '.schemaVersion' <<<"${json}")
    const=$(jq -r '.properties.schemaVersion.const // empty' "${schema}")
    if [ -n "${const}" ] && [ "${emitted}" != "${const}" ]; then
        echo "schemaVersion ${emitted} != declared const ${const}"
        return 1
    fi
}

# conforms <workdir> <schema-file> <quickemu args...>
conforms() {
    local dir="$1" schema="$2" out json
    shift 2
    # stderr carries host-probe noise (glxinfo/qemu -device help on DRI
    # hosts); the contract under test is stdout, so park it for debugging.
    out=$(cd "${dir}" && "$QE_ROOT/bin/quickemu" "$@" 2>"${dir}/.stderr")
    if jq -e . >/dev/null 2>&1 <<<"${out}"; then
        json="${out}"
    else
        # Some commands print a human preamble before the JSON payload.
        json=$(grep -m1 '^{' <<<"${out}")
    fi
    assert_json_conforms "${QE_ROOT}/schemas/v1/${schema}" "${json}"
}

qemu_missing() { ! command -v qemu-system-x86_64 >/dev/null 2>&1; }

Describe '--capabilities'
It 'conforms to capabilities.schema.json'
    d=$(tmpdir)
    When call conforms "${d}" capabilities.schema.json --capabilities
    The status should be success
End
End

Describe '--list-vms --format json'
It 'conforms to vm-list.schema.json'
    d=$(tmpdir)
    When call conforms "${d}" vm-list.schema.json --list-vms --format json
    The status should be success
End
End

Describe '--status --format json'
It 'conforms to vm-status.schema.json'
    d=$(tmpdir)
    printf 'guest_os="linux"\ndisk_img="v/d.qcow2"\niso="v/i.iso"\n' >"${d}/vm1.conf"
    When call conforms "${d}" vm-status.schema.json --vm vm1.conf --status --format json
    The status should be success
End
End

Describe '--delete-plan --format json'
It 'conforms to delete-plan.schema.json'
    d=$(tmpdir)
    printf 'guest_os="linux"\n' >"${d}/vm1.conf"
    When call conforms "${d}" delete-plan.schema.json --vm vm1.conf --delete-plan --format json
    The status should be success
End
End

Describe '--dry-run --format json'
It 'conforms to execution-plan.schema.json'
    d=$(tmpdir)
    mkdir -p "${d}/v"
    touch "${d}/v/d.qcow2" "${d}/v/i.iso"
    printf 'guest_os="linux"\ndisk_img="v/d.qcow2"\niso="v/i.iso"\n' >"${d}/vm1.conf"
    When call conforms "${d}" execution-plan.schema.json --vm vm1.conf --dry-run --format json
    The status should be success
End
End
End
