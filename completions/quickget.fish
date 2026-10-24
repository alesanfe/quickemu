# Fish completion for quickget — install to ~/.config/fish/completions/
function __quickget_oses
    quickget --list 2>/dev/null | awk 'NR>1{print $1}' | sort -u
end
complete -c quickget -f -a '(__quickget_oses)'
complete -c quickget -l arch -x -a 'amd64 arm64'
complete -c quickget -l download -d 'Download image only'
complete -c quickget -l create-config -d 'Create VM config'
complete -c quickget -l cloud-init -d 'Generate NoCloud seed'
complete -c quickget -l unattended -d 'Unattended install (ubuntu-server)'
complete -c quickget -l update -r -a '(__quickget_oses)' -d 'Show latest release'
complete -c quickget -l check -r -a '(__quickget_oses)' -d 'Check image URL'
complete -c quickget -l check-all-arch -r -a '(__quickget_oses)' -d 'Check all archs'
complete -c quickget -l url -r -a '(__quickget_oses)' -d 'Show image URL'
complete -c quickget -l show -r -a '(__quickget_oses)' -d 'Show OS info'
complete -c quickget -l open-homepage -r -a '(__quickget_oses)' -d 'Open OS homepage'
complete -c quickget -l list -d 'List supported OSes'
complete -c quickget -l list-csv -d 'List as CSV'
complete -c quickget -l list-json -d 'List as JSON'
complete -c quickget -l insecure -d 'Skip SSL verification'
complete -c quickget -l version -d 'Show version'
complete -c quickget -l help -d 'Show help'
