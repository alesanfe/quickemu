#compdef quickemu
# Zsh completion for quickemu — install to a directory in $fpath
_quickemu() {
    _arguments \
        '--vm[VM config file]:conf:_files -g "*.(conf|toml)"' \
        '--start[start VM]' '--stop[stop VM via QMP quit]' '--restart[restart VM]' \
        '--status[VM status]' '--status-quo[do not commit disk changes]' \
        '--pause[pause VM]' '--resume[resume VM]' \
        '--shutdown[ACPI shutdown]' '--reset[hard reset]' '--kill[kill VM]' \
        '--list-vms[list VMs in directory]' '--inventory[list all VM dirs]' \
        '--delete-vm[delete VM]' '--delete-disk[delete disk image]' \
        '--delete-plan[show delete plan]' '--plan-id[plan id]:id:' \
        '--delete-shortcut[delete desktop shortcut]' \
        '--clone[clone VM]:name:' '--full[copy disk fully with --clone]' \
        '--linked-clone[clone to another dir]:dir:_files -/' \
        '--rename[rename VM]:name:' '--move[move VM dir]:dir:_files -/' \
        '--backup[write disk backup]' '--backup-list[list backups]' \
        '--backup-verify[verify backup]:file:_files -g "*.tar.gz"' \
        '--backup-restore[restore backup]:file:_files -g "*.tar.gz"' \
        '--backup-prune[keep newest n backups]:n:' \
        '--export[export VM]:file:_files -g "*.tar.gz"' \
        '--import[import VM]:file:_files -g "*.tar.gz"' \
        '--trash[delete to trash]' '--trash-list[list trashed VMs]' \
        '--trash-restore[restore trashed VM]:vm:' '--trash-empty[empty trash]' \
        '--snapshot[snapshot action]:action:(apply create delete info list tree verify)' \
        '--snapshot-tree[snapshot tree]' '--snapshot-verify[verify snapshots]' \
        '--external[external snapshot]' \
        '--monitor-cmd[send monitor command]:cmd:' \
        '--send-keys[send keys]:combo:' '--type[type text]:text:' \
        '--screenshot[save screenshot]:file:_files' \
        '--disk-compact[reclaim disk space]' '--disk-resize[resize disk]:size:' \
        '--spice-console[attach SPICE viewer]' '--vnc[launch VNC client]' \
        '--publicdir[open public dir]' \
        '--serial-console[attach serial console]' '--monitor-console[attach monitor console]' \
        '--hotplug-usb[hotplug USB vid:pid]:id:' '--hotplug-disk[hotplug disk]:file:_files' \
        '--hotunplug[remove hotplugged device]:id:' '--persist[write hotplug to conf]' \
        '--autostart[create systemd user unit]' '--shortcut[create desktop shortcut]' \
        '--doctor[check VM vs host]' '--check[alias of doctor]' '--fix[fix doctor findings]' \
        '--inspect-conf[print conf values]' '--migrate-conf[write .toml from .conf]' \
        '--capabilities[capability descriptor]' '--host-info[host inventory]' \
        '--vfio-scan[scan VFIO candidates]' '--events[stream events]:n:' '--stats[resource stats]' \
        '--plugins[list marketplace plugins]' '--plugin[run plugin]:name:' \
        '--profile[apply hardware profile]:profile:(macos windows linux linux_old kolibrios freedos)' \
        '--display[display backend]:display:(gtk sdl spice spice-app cocoa vnc none)' \
        '--viewer[SPICE viewer]:viewer:(spicy remote-viewer none)' \
        '--monitor[monitor type]:type:(socket telnet none)' \
        '--monitor-telnet-host[telnet host]:host:' '--monitor-telnet-port[telnet port]:port:' \
        '--serial[serial type]:type:(socket telnet none)' \
        '--serial-telnet-host[telnet host]:host:' '--serial-telnet-port[telnet port]:port:' \
        '--usb-controller[usb controller]:type:(ehci xhci none)' \
        '--usb-passthrough[USB vid:pid]:id:' \
        '--keyboard[keyboard]:type:(usb virtio ps2)' \
        '--keyboard-layout[keyboard layout]:layout:' \
        '--mouse[mouse]:type:(tablet virtio ps2 usb)' \
        '--sound-card[sound card]:type:(intel-hda ac97 es1370 sb16 usb-audio virtio-sound-pci none)' \
        '--sound-duplex[sound duplex]:type:(hda-micro hda-duplex hda-output)' \
        '--public-dir[share directory]:dir:_files -/' \
        '--access[remote spice access]:mode:(local remote)' \
        '--ssh-access[remote ssh access]:mode:(local remote)' \
        '--ssh-port[ssh port]:port:' '--spice-port[spice port]:port:' \
        '--width[screen width]:px:' '--height[screen height]:px:' \
        '--cpu-pinning[host core mapping]:cores:' '--braille[braille support]' \
        '--fullscreen[start fullscreen]' '--offline[start offline]' \
        '--ignore-msrs[ignore KVM msrs]' '--ignore-msrs-always[always ignore msrs]' \
        '--ignore-tsc-warning[skip TSC warning]' \
        '--extra-args[extra qemu args]:args:' \
        '--echo[print args without running]' '--dry-run[print QEMU command only]' \
        '--format[output format]:fmt:(human json text)' \
        '--version[version]' '--help[help]'
}
_quickemu
