# shellcheck disable=SC2034
# Coverage: quickreport public contract — report composition, missing-
# dependency wording and multi-GPU listing — exercised through the
# canonical entrypoint (bin/quickreport -> tools/quickreport).
Describe 'quickreport entrypoint'
Include spec/spec_helper.sh

setup() {
    TESTDIR=$(mktemp -d)
    FAKEBIN="$TESTDIR/bin"
    mkdir -p "$FAKEBIN"
    # Fake the probed binaries so the report is deterministic.
    printf '#!/bin/sh\necho "9.9.9"\n' >"$FAKEBIN/quickemu"
    printf '#!/bin/sh\necho "QEMU emulator version 8.2.2"\n' >"$FAKEBIN/qemu-system-x86_64"
    chmod +x "$FAKEBIN/quickemu" "$FAKEBIN/qemu-system-x86_64"
}
cleanup() { rm -rf -- "$TESTDIR"; }
BeforeEach 'setup'
AfterEach 'cleanup'

run_quickreport() {
    PATH="$FAKEBIN:$PATH" "${QE_ROOT}/bin/quickreport"
}

Describe 'report composition'
    It 'renders the report with fake probed tools'
        When run run_quickreport
        The output should include 'Quickemu'
        The output should include 'Memory'
        The output should include 'QEMU'
        The output should include 'CPU'
    End
    It 'succeeds (exit 0) when quickemu and qemu are present'
        When run run_quickreport
        The status should be success
        The output should include 'Quickemu 5.0.0'
    End
End

Describe 'missing-dependency wording'
    It 'reports quickemu missing when it cannot be resolved at all'
        # Copy the real binary into an isolated dir: sibling lookups
        # (dir/quickemu, dir/../../bin/quickemu) then miss, so PATH is
        # the only resolver — and PATH carries no quickemu here.
        no_qe() {
            local iso="$TESTDIR/iso" nb="$TESTDIR/none"
            mkdir -p "$iso" "$nb"
            cp "${QE_ROOT}/tools/quickreport/quickreport" "$iso/"
            printf '#!/bin/sh\necho "QEMU emulator version 8.2.2"\n' >"$nb/qemu-system-x86_64"
            chmod +x "$nb/qemu-system-x86_64"
            # PATH carries the fake qemu + coreutils only — no quickemu.
            PATH="$nb:/usr/bin:/bin" "$iso/quickreport"
        }
        When run no_qe
        The status should be failure
        The output should include 'Quickemu missing'
    End
End
End
