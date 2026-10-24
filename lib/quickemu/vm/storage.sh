# Storage helpers — resolve disk paths relative to the VM dir.
vm_disk_path() { # vmdir diskimg -> absolute-ish path
    printf '%s/%s' "${1}" "${2}"
}
