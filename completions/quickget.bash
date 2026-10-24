# Bash completion for quickget — source or install to /etc/bash_completion.d/
_quickget() {
    local cur prev opts
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"
    opts="--arch --check --check-all-arch --cloud-init --create-config --download \
--help --insecure --list --list-csv --list-json --open-homepage --show \
--unattended --update --url --version"

    case "${prev}" in
        --arch)   COMPREPLY=($(compgen -W "amd64 arm64" -- "${cur}")); return 0;;
        --update|--open-homepage|--show|--url|--check|--check-all-arch)
            if command -v quickget >/dev/null 2>&1; then
                COMPREPLY=($(compgen -W "$(quickget --list 2>/dev/null | awk 'NR>1{print $1}' | sort -u)" -- "${cur}"))
            fi
            return 0;;
        *) ;;
    esac
    if [[ "${cur}" == -* ]]; then
        COMPREPLY=($(compgen -W "${opts}" -- "${cur}"))
    elif command -v quickget >/dev/null 2>&1; then
        COMPREPLY=($(compgen -W "$(quickget --list 2>/dev/null | awk 'NR>1{print $1}' | sort -u)" -- "${cur}"))
    fi
    return 0
}
complete -F _quickget quickget
