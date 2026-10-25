# shellcheck disable=SC2034
Describe 'quickget catalog info and list emitters'
Include lib/quickget/catalog/info.sh
Include lib/quickget/catalog/list.sh
Include lib/quickget/catalog/arch.sh
Include lib/quickget/catalog/data.sh

setup() {
    OS=alpine RELEASE='' EDITION='' I18N='' I18NS=() CSV_DATA=''
}
BeforeEach 'setup'
web_pipe() { :; }
web_pipe_json() { :; }
web_check() { return 0; }

Describe 'os_info'
    It 'returns the pipe-separated record for a known OS'
        When call os_info alpine
        The output should include 'Alpine Linux|'
        The output should include 'alpinelinux.org'
    End
    It 'is empty for an unknown OS'
        When call os_info nosuchos
        The output should be blank
    End
End

Describe 'pretty_name / os_homepage'
    It 'extracts the display name'
        When call pretty_name alpine
        The output should eq 'Alpine Linux'
    End
    It 'extracts the homepage'
        When call os_homepage alpine
        The output should eq 'https://alpinelinux.org/'
    End
End

Describe 'show_os_info'
    It 'prints name, credentials, website and description'
        When call show_os_info alpine
        The output should include 'Alpine Linux'
        The output should include 'Credentials'
        The output should include 'alpinelinux.org'
        The output should include 'musl'
    End
End

Describe 'os_support'
    It 'lists well known OS names'
        When call os_support
        The output should include 'alpine'
        The output should include 'ubuntu'
        The output should include 'windows'
    End
End

Describe 'csv_data / list_csv / list_supported / list_json'
    It 'csv_data emits one row per release with icon URLs'
        releases_alpine() { echo '3.20'; }
        When call csv_data
        The output should include 'Alpine Linux,alpine,3.20'
        The output should include 'quickemu-icons'
    End
    It 'csv_data expands editions into rows'
        releases_debian() { echo '12'; }
        When call csv_data
        The output should include 'Debian,debian,12,gnome'
    End
    It 'csv_data expands windows languages into rows'
        When call csv_data
        The output should include 'windows,11,'
    End
    It 'list_csv prints the header plus sorted rows'
        releases_alpine() { echo '3.20'; }
        When run list_csv
        The status should be success
        The output should include 'Display Name,OS,Release,Option,Downloader,PNG,SVG'
        The output should include ',alpine,'
    End
    It 'list_supported flattens the CSV to os release option'
        releases_alpine() { echo '3.20'; }
        When run list_supported
        The status should be success
        The output should include 'alpine'
    End
    It 'list_json wraps rows as objects'
        releases_alpine() { echo '3.20'; }
        When run list_json
        The status should be success
        The output should include '"OS": "alpine"'
    End
End
End
