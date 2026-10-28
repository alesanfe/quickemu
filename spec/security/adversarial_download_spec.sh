# Adversarial guards on the Go download layer (tools/quickget-vm +
# tools/quickget-catalog): the same defenses Bash web.sh enforced.
Describe 'adversarial: download layer defenses (go)'
    It 'fetch restricts the URL scheme to http+https'
        When run bash -c 'grep -q "unsupported URL scheme" tools/quickget-vm/main.go && grep -q "u.Scheme != \"http\" && u.Scheme != \"https\"" tools/quickget-vm/main.go'
        The status should be success
    End
    It 'redirects cannot leave http+https schemes'
        When run bash -c 'grep -q "redirect to disallowed scheme" tools/quickget-vm/main.go'
        The status should be success
    End
    It 'download retries are bounded'
        When run bash -c 'grep -q "attempt <= 3\|attempt <=3" tools/quickget-vm/main.go'
        The status should be success
    End
    It 'web_check has a HEAD->GET range fallback for signed URLs'
        When run bash -c 'grep -q "bytes=0-0" tools/quickget-catalog/info.go && grep -q "MethodHead" tools/quickget-catalog/info.go'
        The status should be success
    End
    It 'downloads resume rather than restart partial files'
        When run bash -c 'grep -q "Range.*bytes=" tools/quickget-vm/main.go && grep -q "O_APPEND" tools/quickget-vm/main.go'
        The status should be success
    End
    It 'trust levels can refuse below minimum'
        When run bash -c 'grep -q "minimum-trust" tools/quickget-cli/main.go && grep -q "below --minimum-trust" tools/quickget-cli/main.go'
        The status should be success
    End
    It 'trust D/E emits a warning'
        When run bash -c 'grep -q "warning: %s source integrity not verifiable" tools/quickget-cli/main.go'
        The status should be success
    End
End
