# curl adapter — centralised HTTPS download with timeout and diagnostics.
curl_fetch() {  # url dest -> 0 ok
    curl -fSL --retry 3 --connect-timeout 15 -o "${2}" "${1}"
}
