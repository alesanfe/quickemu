# QE-03.4: adversarial download scenarios — MITM-ish edge cases the
# download layer must reject. Static checks on the shipped code.
Describe 'adversarial: download layer defenses'
    It 'web_get restricts protocol to http+https'
        When run bash -c 'grep -q "proto .=http,https" lib/quickget/download/web.sh'
        The status should be success
    End
    It 'web_get restricts redirects to http+https'
        When run bash -c 'grep -q "proto-redir .=http,https" lib/quickget/download/web.sh'
        The status should be success
    End
    It 'download retries are bounded'
        When run bash -c 'grep -qE "retry [0-9]|retry=[0-9]" lib/quickget/download/web.sh'
        The status should be success
    End
    It 'web_check has a HEAD->GET range fallback for signed URLs'
        When run bash -c 'grep -q "range 0-0\|-r 0-0\|Range:" lib/quickget/download/web.sh'
        The status should be success
    End
    It 'downloads are bounded by time and speed limits'
        When run bash -c 'grep -qE "speed-limit|max-time" lib/quickget/download/web.sh'
        The status should be success
    End
    It 'downloads resume rather than restart partial files'
        When run bash -c 'grep -qE "continue-at|continue=true" lib/quickget/download/web.sh'
        The status should be success
    End
    It 'trust levels can refuse below minimum'
        When run bash -c 'grep -q "trust_meets_minimum\|minimum.trust\|MIN_TRUST" lib/quickget/catalog/*.sh lib/quickget/download/*.sh 2>/dev/null'
        The status should be success
    End
    It 'trust D/E emits a warning'
        When run bash -c 'grep -rn "warning:" lib/quickget/ | grep -qi "trust\|integrity"'
        The status should be success
    End
End
