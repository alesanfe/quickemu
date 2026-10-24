#compdef quickget
# Zsh completion for quickget — install to a directory in $fpath
_quickget() {
    local -a oses
    oses=(${(f)"$(quickget --list 2>/dev/null | awk 'NR>1{print $1}' | sort -u)"})
    _arguments \
        '--arch[architecture]:arch:(amd64 arm64)' \
        '--download[download only]' '--create-config[create config]' \
        '--cloud-init[generate NoCloud seed]' '--unattended[unattended install]' \
        '--update[show latest release]:os:->oses' \
        '--check[check URL]:os:->oses' '--check-all-arch[check all arch]:os:->oses' \
        '--url[show URL]:os:->oses' '--show[show OS info]:os:->oses' \
        '--open-homepage[open homepage]:os:->oses' \
        '--list[list OSes]' '--list-csv[list CSV]' '--list-json[list JSON]' \
        '--insecure[skip SSL verification]' '--version[version]' '--help[help]' \
        '*: :->oses'
    if [[ ${state} == oses ]]; then
        compadd -- ${oses}
    fi
}
_quickget
