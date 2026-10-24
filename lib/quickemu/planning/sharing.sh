# File-sharing plan gates (public_dir requires virtiofsd or samba).
plan_check_share() {
    [ -z "${1}" ] && return 0
    host_has_tool virtiofsd || host_has_tool smbd
}
