# Contract tests for the structured CLI surface.
Describe 'public contract (schema v1)'
Include spec/spec_helper.sh

tmpdir() { mktemp -d; }

Describe '--capabilities'
It 'emits schemaVersion 1 and a capabilities map'
When call "$QE_ROOT/bin/quickemu" --capabilities
The output should include '"schemaVersion": 1'
The output should include '"structuredStatus": true'
The status should be success
End
End

Describe '--list-vms --format json'
It 'returns an empty vms array when no confs exist'
d=$(tmpdir)
When call sh -c "cd $d && $QE_ROOT/bin/quickemu --list-vms --format json"
The output should equal '{"schemaVersion":1,"vms":[]}'
The status should be success
End
End

Describe '--status --format json'
It 'reports state stopped and null pid for a stopped VM'
d=$(tmpdir)
printf 'guest_os="linux"\ndisk_img="v/d.qcow2"\niso="v/i.iso"\n' >"$d/vm1.conf"
When call sh -c "cd $d && $QE_ROOT/bin/quickemu --vm vm1.conf --status --format json"
The output should include '"name":"vm1"'
The output should include '"state":"stopped"'
The output should include '"pid":null'
End
End

Describe '--delete-plan --format json'
It 'lists the conf without deleting anything'
d=$(tmpdir)
printf 'guest_os="linux"\n' >"$d/vm1.conf"
When call sh -c "cd $d && $QE_ROOT/bin/quickemu --vm vm1.conf --delete-plan --format json"
The output should include '"vm":"vm1"'
The output should include '"vm1.conf"'
The path "$d/vm1.conf" should be file
End
End
End
