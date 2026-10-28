# Contract: the Go download path (tools/quickget-vm) must carry the
# same hardening the Bash web_get had — resume, retry, atomic staging,
# bounded redirects, zsync delegation.
Describe 'contract: download adapters (go)'
    It 'fetch retries and resumes partial downloads'
        When run bash -c 'grep -q "attempt <= 3\|attempt <=3" tools/quickget-vm/main.go && grep -q "Range" tools/quickget-vm/main.go && grep -q "bytes=%d-" tools/quickget-vm/main.go'
        The status should be success
    End
    It 'downloads are bounded by dial/header timeouts'
        When run bash -c 'grep -q "ResponseHeaderTimeout" tools/quickget-vm/main.go && grep -q "DialContext" tools/quickget-vm/main.go'
        The status should be success
    End
    It 'zsync adapter exists for rsync-capable releases'
        When run bash -c 'grep -q "zsync" tools/quickget-vm/main.go && grep -q "zsyncGet" tools/quickget-vm/ubuntu.go'
        The status should be success
    End
    It 'redirect resolution is bounded'
        When run bash -c 'grep -q "too many redirects" tools/quickget-vm/main.go'
        The status should be success
    End
    It 'temp output then atomic rename (no partial final files)'
        When run bash -c 'grep -q ".part" tools/quickget-vm/main.go && grep -q "os.Rename" tools/quickget-vm/main.go'
        The status should be success
    End
End
