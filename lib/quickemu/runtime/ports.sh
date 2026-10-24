# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Sourced partial: shares the conf-schema global namespace with bin/quickemu.
# Host TCP port allocation for guests.

function get_port() {
    local PORT_START=$1
    local PORT_RANGE=$((PORT_START + $2))
    local PORT
    for ((PORT = PORT_START; PORT <= PORT_RANGE; PORT++)); do
        # Make sure port scans do not block too long.
        timeout 0.1s bash -c "echo >/dev/tcp/127.0.0.1/${PORT}" >/dev/null 2>&1
        if [ ${?} -eq 1 ]; then
            echo "${PORT}"
            break
        fi
    done
}
