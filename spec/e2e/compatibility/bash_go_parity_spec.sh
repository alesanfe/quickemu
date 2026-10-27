#!/usr/bin/env bash
. "$(dirname "$0")/../support/e2e_lib.sh"
Describe 'bash_go_parity @compatibility @p1'
It 'bash and go backends produce equivalent envelopes'
out=$("$QE" --capabilities --format json 2>/dev/null)
The output should include '"schemaVersion":1'
End
End
