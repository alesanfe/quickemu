# shellcheck disable=SC2034
Describe 'windows download helpers'
Include lib/quickget/download/windows.sh

setup() {
    TESTDIR=$(mktemp -d)
    FAKE_BIN="$TESTDIR/bin"; mkdir -p "$FAKE_BIN"
    VM_PATH="$TESTDIR/vm"; mkdir -p "$VM_PATH"
    OS=windows-server RELEASE=2022 I18N='English International'
    PRETTY_OS_NAME='Windows Server' OPERATION=download
    CURL_CALLS="$TESTDIR/curl.calls"
    web_get() { echo "WEBGET $1 -> $2/$3"; }
    handle_curl_error() { echo "CURL-ERR $1"; }
}
cleanup() { rm -rf -- "$TESTDIR"; }
BeforeEach 'setup'
AfterEach 'cleanup'

# Fake curl: first call (page) returns fixture HTML, second call (HEAD
# redirect) returns the final URL. Later calls return empty success.
fake_curl_server() {
    cat >"$FAKE_BIN/curl" <<'EOF'
#!/bin/sh
case "$*" in
    *evalcenter*) printf 'x https://go.microsoft.com/fwlink/p/?LinkID=123&clcid=0x409&culture=en-us&country=US y\n';;
    *url_effective*) echo 'https://cdn.example/win2022.iso';;
esac
EOF
    chmod +x "$FAKE_BIN/curl"
    PATH="$FAKE_BIN:$PATH"
}

Describe 'download_windows_server'
    It 'parses the eval page, resolves the link and downloads'
        fake_curl_server
        When call download_windows_server windows-server-2022 enterprise
        The status should be success
        The output should include 'Downloading Windows Server 2022'
        The output should include 'URL: https://cdn.example/win2022.iso'
        The output should include 'WEBGET https://cdn.example/win2022.iso'
        The output should include 'Parsing download page'
        The variable OS should eq 'windows-server'
    End
    It 'fails when the download page is empty'
        printf '#!/bin/sh\nexit 0\n' >"$FAKE_BIN/curl"; chmod +x "$FAKE_BIN/curl"
        PATH="$FAKE_BIN:$PATH"
        When call download_windows_server windows-server-2022 enterprise
        The status should be failure
        The output should include 'empty response'
    End
    It 'fails when no matching link exists for the culture'
        cat >"$FAKE_BIN/curl" <<'EOF'
#!/bin/sh
case "$*" in
    *evalcenter*) echo 'no links here';;
esac
EOF
        chmod +x "$FAKE_BIN/curl"; PATH="$FAKE_BIN:$PATH"
        When call download_windows_server windows-server-2022 enterprise
        The status should be failure
        The output should include 'no download link'
    End
    It 'maps I18N to culture and country'
        fake_curl_server
        I18N='German'
        When call download_windows_server windows-server-2022 ltsc
        The status should be failure
        The output should include 'no download link'
        The output should include 'Getting download link'
    End
End

Describe 'download_windows_workstation'
    fake_curl_ws() {
        cat >"$FAKE_BIN/curl" <<'EOF'
#!/bin/sh
case "$*" in
    *getskuinformation*) echo '{"Skus":[{"LocalizedLanguage":"English International","Id":"10441"}]}';;
    *GetProductDownloadLinksBySku*) echo '{"ProductDownloadOptions":[{"Uri":"https://cdn.example/win11x64.iso?h=1"},{"Uri":"https://cdn.example/win11arm.iso"}]}';;
    *mdt.js*) echo 'cfg?w=ABC123 rticks="+63812345';;
    *software-download/windows*) echo '<option value="2718">Windows 11</option>';;
    *) exit 0;;
esac
EOF
        chmod +x "$FAKE_BIN/curl"
        PATH="$FAKE_BIN:$PATH"
    }
    It 'walks the Microsoft API chain and downloads the x64 link'
        fake_curl_ws
        printf '#!/bin/sh\necho fixed-uuid\n' >"$FAKE_BIN/uuidgen"; chmod +x "$FAKE_BIN/uuidgen"
        PATH="$FAKE_BIN:$PATH"
        When call download_windows_workstation 11
        The status should be success
        The output should include 'Product edition ID: 2718'
        The output should include 'Getting ISO download link'
        The output should include 'URL: https://cdn.example/win11x64.iso'
        The output should include 'WEBGET https://cdn.example/win11x64.iso'
    End
    It 'falls back to kernel uuid without uuidgen'
        fake_curl_ws
        command() { [ "$2" = uuidgen ] && return 1; builtin command "$@"; }
        When call download_windows_workstation 11
        The status should be success
        The output should include 'Permit Session ID'
    End
    It 'reports failure when the API returns empty'
        cat >"$FAKE_BIN/curl" <<'EOF'
#!/bin/sh
case "$*" in
    *getskuinformation*|*GetProductDownloadLinksBySku*) exit 0;;
    *mdt.js*) echo 'x';;
    *software-download/windows*) echo '<option value="9">Windows</option>';;
    *) exit 0;;
esac
EOF
        chmod +x "$FAKE_BIN/curl"; PATH="$FAKE_BIN:$PATH"
        When call download_windows_workstation 11
        The status should be failure
        The output should include 'Manually download'
        The output should include 'empty response'
    End
End
End
