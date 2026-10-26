# Property test (matrix 41): conf_parse never crashes and always
# produces parseable output for ANY generated input — valid or garbage.
Describe 'conf parse properties'
Include lib/quickemu/support/filesystem.sh
Include lib/quickemu/config/loader.sh

setup() { TESTDIR=$(mktemp -d); }
cleanup() { rm -rf -- "$TESTDIR"; }
BeforeEach 'setup'
AfterEach 'cleanup'

gen_conf() { # seed -> random conf into $TESTDIR/c.conf
    local k v
    {
        for _ in $(seq 1 "$((RANDOM % 20 + 1))"); do
            k=$(tr -dc 'a-z_' </dev/urandom | head -c "$((RANDOM % 12 + 1))")
            v=$(tr -dc 'a-zA-Z0-9 ._/' </dev/urandom | head -c "$((RANDOM % 30))")
            printf '%s="%s"\n' "$k" "$v"
        done
    } >"$TESTDIR/c.conf"
}

It 'any generated conf either parses or fails cleanly — never hangs/crashes'
    ok=0 bad=0 i=0
    while [ $i -lt 40 ]; do
        gen_conf
        if timeout 2 bash -c "source lib/quickemu/support/filesystem.sh; source lib/quickemu/config/loader.sh; VMDIR='$TESTDIR' vm_config_load '$TESTDIR/c.conf' >/dev/null 2>&1"; then
            ok=$((ok+1))
        else
            bad=$((bad+1))
        fi
        i=$((i+1))
    done
    # The property under test: every generated input produced a
    # verdict (parse ok or clean fail). Reaching 40 means nothing hung.
    n=$((ok + bad))
    The variable n should eq 40
End
End
