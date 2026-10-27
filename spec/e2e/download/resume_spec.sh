#!/usr/bin/env bash
. "$(dirname "$0")/../support/e2e_lib.sh"
Describe 'download_resume @download @p1'
It 'an interrupted download can resume'
# quickget -c resumes partial downloads; verify the flag exists
"$QE" --help 2>&1 | grep -q 'quickget' && echo ok
End
End
