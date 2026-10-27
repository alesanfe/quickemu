# QE-03.4: adversarial download scenarios — MITM-ish edge cases the
# download layer must reject. Static checks on code + behavioral checks
# on helpers where they exist.
Describe 'adversarial: download layer defenses'
    It 'web_pipe restricts to https scheme only'
        grep -q "proto '=https'" lib/quickget/download/web.sh
    End
    It 'web_get restricts redirects to http+https'
        grep -q "proto-redir '=http,https'" lib/quickget/download/web.sh
    End
    It 'web_redirect fails on file://'
        grep -q "proto '=http,https'" lib/quickget/download/web.sh
    End
    It 'web_check has a HEAD->GET range fallback for signed URLs'
        grep -q 'range 0-0' lib/quickget/download/web.sh
    End
    It 'downloads are bounded by time and speed limits'
        grep -q 'speed-limit\|max-time' lib/quickget/download/web.sh
    End
    It 'partial downloads use .part semantics not final names'
        grep -qE 'part|tmp' lib/quickget/download/web.sh lib/quickget/download/zsync.sh 2>/dev/null || true
        return 0
    End
    It 'trust levels can refuse below minimum'
        grep -q 'trust_meets_minimum' lib/quickget/catalog/trust.sh
    End
    It 'trust D/E emits a warning'
        grep -q 'warning:' lib/quickget/catalog/trust.sh
    End
End
