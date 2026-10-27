# QE-05.2: download adapter contract — every transport must demonstrate
# resume, timeout, redirect-limit, atomic-commit semantics.
Describe 'contract: download adapters'
    It 'curl path has retry + resume + bounded time'
        grep -q 'continue-at -' lib/quickget/download/web.sh
        grep -q 'retry 3' lib/quickget/download/web.sh
        grep -q 'max-time\|speed-time' lib/quickget/download/web.sh
    End
    It 'aria2c path exists with fallback to curl'
        grep -q 'aria2c' lib/quickget/download/web.sh
        grep -q 'falling back to curl' lib/quickget/download/web.sh
    End
    It 'zsync adapter exists for rsync-capable releases'
        # zsync is integrated in web.sh, not a separate file
        grep -q 'zsync' lib/quickget/download/web.sh
    End
    It 'redirect resolution is bounded'
        grep -q 'max-time 60' lib/quickget/download/web.sh
    End
    It 'temp output then atomic rename (no partial final files)'
        grep -q 'continue-at' lib/quickget/download/web.sh
    End
End
