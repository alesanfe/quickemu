# VM registry helpers — enumerate *.conf files in a directory.
vm_conf_list() { # dir -> one conf path per line
    find "${1:-.}" -maxdepth 1 -name '*.conf' 2>/dev/null
}
