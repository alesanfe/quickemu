#compdef quickemu
# Zsh completion for quickemu — install to a directory in $fpath
_quickemu() {
    _arguments \
        '--vm[VM config file]:conf:_files -g "*.conf"' \
        '--display[display backend]:display:(gtk sdl spice spice-app cocoa vnc none)' \
        '--snapshot[snapshot action]:action:(apply create delete info list)' \
        '--clone[clone VM to new name]:name:' \
        '--disk-resize[resize disk]:size:' \
        '--disk-compact[reclaim disk space]' \
        '--export[export VM]:file:_files -g "*.tar.gz"' \
        '--import[import VM]:file:_files -g "*.tar.gz"' \
        '--rename[rename VM]:name:' \
        '--pause[pause VM]' '--resume[resume VM]' '--kill[kill VM]' '--status[VM status]' \
        '--screenshot[save screenshot]:file:_files' \
        '--send-keys[send keys]:combo:' '--type[type text]:text:' \
        '--list-vms[list VMs in directory]' '--doctor[check VM vs host]' \
        '--autostart[create systemd user unit]' \
        '--usb-passthrough[USB vid:pid]:id:' \
        '--dry-run[print QEMU command only]' \
        '--offline[start VM offline]' '--shortcut[create desktop shortcut]' \
        '--status-quo[do not commit disk changes]' \
        '--monitor[monitor type]:type:(socket telnet none)' \
        '--monitor-cmd[send monitor command]:cmd:' \
        '--serial[serial type]:type:(socket telnet none)' \
        '--fullscreen[fullscreen]' '--offline[offline]' \
        '--version[version]' '--help[help]'
}
_quickemu
