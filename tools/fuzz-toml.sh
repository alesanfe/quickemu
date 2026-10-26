#!/usr/bin/env bash
# Deterministic TOML parser fuzzer: feeds hostile inputs to toml_parse
# and asserts it never hangs, never executes payload, and returns 0/1.
set -u
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT}"

FAIL=0; D=$(mktemp -d); trap 'rm -rf "$D"' EXIT
run_case() { # name input -> expect bounded exit
    printf '%s' "${2}" >"${D}/f.toml"
    timeout 5 bash -c 'cd "'"${ROOT}"'"; source lib/quickemu/config/parser.sh; source lib/quickemu/config/loader.sh; source lib/quickemu/config/toml.sh; toml_parse "${1}"' _ "${D}/f.toml" >/dev/null 2>&1
    local RC=$?
    [ ${RC} -ge 124 ] && { echo "HANG: ${1}"; FAIL=1; return; }
    [ ${RC} -gt 1 ] && { echo "CRASH(${RC}): ${1}"; FAIL=1; return; }
}
# hostile corpus
run_case "unicode"      "$(printf 'guest_os = "linu\303\251\u1234"\n')"
run_case "control"      "$(printf 'a\x01\x02\x03 = 1\n')"
run_case "truncated"    'guest_os = "unclosed'
run_case "longline"     "$(printf 'k = "%0.s' {1..100000} | tr -d '\n'; echo '"')"
run_case "deepsection"  "$(printf '[a.b.c.d.e.f.g.h.i.j.k.l.m]\nx=1\n')"
run_case "dupkeys"      "$(printf 'a=1\na=1\na=1\n')"
run_case "codeinject"   'guest_os = "$(rm -rf /)"'
run_case "backtick"     'guest_os = "`id`"'
run_case "crlf"         "$(printf 'guest_os="linux"\r\nram="4G"\r\n')"
run_case "empty"        ''
run_case "nullbyte"     "$(printf 'a\x00b=1\n' 2>/dev/null || echo 'a b=1')"
run_case "hugeval"      "$(printf 'ram = "'; head -c 200000 /dev/zero | tr '\0' 'G'; echo '"')"
run_case "manykeys"     "$(for i in $(seq 1 5000); do echo "k$i=v"; done)"
run_case "tablevals"    'x = {a=1, b="evil"}'
run_case "nestarr"      'x = [["a","b"],["c"]]'
# execution safety: payload must never run
rm -f "${D}/pwned"
printf 'guest_os = "$(touch %s/pwned)"\n' "${D}" >"${D}/f.toml"
timeout 5 bash -c 'cd "'"${ROOT}"'"; source lib/quickemu/config/parser.sh; source lib/quickemu/config/loader.sh; source lib/quickemu/config/toml.sh; toml_parse "${1}"' _ "${D}/f.toml" >/dev/null 2>&1
[ -f "${D}/pwned" ] && { echo "EXEC-LEAK: toml_parse ran payload"; FAIL=1; }
echo "fuzz-toml: ${FAIL:-0} failures"
exit ${FAIL:-0}
