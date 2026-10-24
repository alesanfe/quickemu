# Display plan gates.
plan_check_viewer() { # viewer -> 0 installed, 1 missing
    case "${1}" in
    "" | none) return 0 ;;
    spicy) host_has_tool spicy ;;
    remote-viewer) host_has_tool remote-viewer ;;
    *) return 1 ;;
    esac
}
