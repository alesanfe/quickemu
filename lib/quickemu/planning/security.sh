# Security plan gates (secureboot needs OVMF vars support).
plan_check_secureboot() { [ "${1}" == "on" ] && ! host_has_uefi; }
