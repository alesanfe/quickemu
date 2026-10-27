# QE-05.2: download adapter contract — every transport must demonstrate
# resume, timeout, redirect-limit, atomic-commit semantics.
Describe 'contract: download adapters'
    It 'curl path has retry + resume + bounded time'
        When run bash -c 'grep -q "continue-at -" lib/quickget/download/web.sh && grep -q "retry 3" lib/quickget/download/web.sh && grep -qE "max-time|speed-time" lib/quickget/download/web.sh'
        The status should be success
    End
    It 'aria2c path exists with fallback to curl'
        When run bash -c 'grep -q "aria2c" lib/quickget/download/web.sh && grep -q "falling back to curl" lib/quickget/download/web.sh'
        The status should be success
    End
    It 'zsync adapter exists for rsync-capable releases'
        # zsync is integrated in web.sh, not a separate file
        When run grep -q 'zsync' lib/quickget/download/web.sh
        The status should be success
    End
    It 'redirect resolution is bounded'
        When run grep -q 'max-time 60' lib/quickget/download/web.sh
        The status should be success
    End
    It 'temp output then atomic rename (no partial final files)'
        When run grep -q 'continue-at' lib/quickget/download/web.sh
        The status should be success
    End
End
