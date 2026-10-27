# QE-06: platform matrix — verify environment detection and feature gating.
# Runs on every host; skips platform-specific assertions cleanly.
Describe 'platform: environment detection'
    It 'detects the host architecture'
        When run bash -c 'uname -m'
        The status should be success
        The stdout should match pattern 'x86_64|aarch64|arm64'
    End

    It 'detects a supported bash version'
        When run bash -c 'echo "${BASH_VERSINFO[0]}.${BASH_VERSINFO[1]}"'
        The status should be success
        The stdout should match pattern '[4-9].*|1[0-9].*'
    End

    It 'reports WSL detection correctly'
        # On WSL the kernel release contains "microsoft" or "WSL"
        When run bash -c 'uname -r | grep -qiE "microsoft|WSL" && echo wsl || echo native'
        The status should be success
        The stdout should match pattern 'wsl|native'
    End

    It 'QEMU binary is discoverable when present'
        When run bash -c 'command -v qemu-system-x86_64'
        The status should be success
        The stdout should include 'qemu-system-x86_64'
    End

    It 'required catalog files are present'
        When run bash -c 'test -f lib/quickget/catalog/data.sh -a -f lib/quickget/catalog/get.sh -a -f lib/quickget/catalog/info.sh'
        The status should be success
    End

    It 'CRLF does not corrupt editions lists (regression: jq.exe on Windows)'
        # The validate.sh tr -d '\r\n' fix must hold — editions must be
        # space-separated single-line output, not multi-line CRLF.
        # releases_debian needs the network (web_pipe); the static
        # editions_alma proves the output-shape contract instead.
        When run bash -c 'source lib/quickget/catalog/data.sh; out=$(editions_alma); a=$(printf %s "${out}" | wc -c); b=$(printf %s "${out}" | tr -d "\r\n" | wc -c); [ "${a}" -eq "${b}" ] || exit 1; printf "%s\n" "${out}"'
        The status should be success
        The stdout should include 'boot minimal dvd'
    End
End
