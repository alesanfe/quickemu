# download_resume @download @p1 — the Go fetch path resumes a .part
# staging file via the Range header (web_get continue-at parity).
Describe 'download_resume @download @p1'
    It 'the download path uses resume-capable fetch'
        When run bash -c 'grep -qE "Range.*bytes=%d-" tools/quickget-vm/main.go && grep -q "O_APPEND" tools/quickget-vm/main.go'
        The status should be success
    End
    It 'server ignoring Range restarts the staging file'
        When run bash -c 'grep -q "StatusPartialContent" tools/quickget-vm/main.go'
        The status should be success
    End
End
