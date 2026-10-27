#!/usr/bin/env bash
# E2E-DOWNLOAD-001: interrupted downloads resume — the download layer must
# pass a resume flag to curl/aria2 so partial files are not restarted.
Describe 'download_resume @download @p1'
    It 'the download path uses resume-capable fetch'
        When run bash -c 'grep -qE "continue-at|--continue=true" lib/quickget/download/web.sh'
        The status should be success
    End
End
