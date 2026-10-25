# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Sourced partial: shares the conf-schema global namespace with bin/quickemu.
# CLI output boundary: usage text, stable exit-code docs and contract emitters.

# ---- usage.sh ----
# Command-line help output.

function usage() {
    echo "             _      _"
    echo "  __ _ _   _(_) ___| | _____ _ __ ___  _   _"
    echo " / _' | | | | |/ __| |/ / _ \ '_ ' _ \| | | |"
    echo "| (_| | |_| | | (__|   <  __/ | | | | | |_| |"
    echo " \__, |\__,_|_|\___|_|\_\___|_| |_| |_|\__,_|"
    echo "    |_| v${VERSION}, using qemu ${QEMU_VER_LONG}"
    echo "--------------------------------------------------------------------------------"
    echo " Project - https://github.com/quickemu-project/quickemu"
    echo " Discord - https://wimpysworld.io/discord"
    echo "--------------------------------------------------------------------------------"
    echo
    echo "Usage"
    echo "  ${LAUNCHER} --vm ubuntu.conf <arguments>"
    echo
    echo "Arguments"
    echo "  --access                          : Enable remote spice access support. 'local' (default), 'remote', 'clientipaddress'"
    echo "  --braille                         : Enable braille support. Requires SDL."
    echo "  --cpu-pinning                     : Choose which host cores correspond to which guest cores."
    echo "  --delete-disk                     : Delete the disk image and EFI variables"
    echo "  --delete-plan                     : Show what --delete-vm would remove (no changes)"
    echo "  --delete-vm                       : Delete the entire VM and its configuration"
    echo "  --delete-shortcut                 : Delete the desktop shortcut for this VM"
    echo "  --display                         : Select display backend. 'gtk' (default), 'sdl', 'cocoa', 'none', 'spice', 'spice-app' or 'vnc'"
    echo "  --fullscreen                      : Starts VM in full screen mode (Ctl+Alt+f to exit)"
    echo "  --ignore-msrs-always              : Configure KVM to always ignore unhandled machine-specific registers"
    echo "  --inspect-conf                    : Print the VM config values without executing the .conf"
    echo "  --ignore-tsc-warning              : Skip TSC stability warning for macOS VMs on AMD"
    echo "  --kill                            : Kill the VM process if it is running"
    echo "  --shutdown                        : Request a graceful ACPI shutdown via the monitor"
    echo "  --reset                           : Hard reset the VM via the monitor (system_reset)"
    echo "  --status                          : Show whether the VM is running or stopped"
    echo "  --capabilities                    : Print the machine-readable capability descriptor (JSON)"
    echo "  --format human|json               : Output format for --status and --list-vms (default: human)"
    echo "  --pause                           : Pause (SIGSTOP) the running VM"
    echo "  --resume                          : Resume (SIGCONT) a paused VM"
    echo "  --screenshot [out.ppm]            : Save a screenshot via the QEMU monitor"
    echo "  --clone <newname>                 : Clone this VM config and disk"
    echo "  --full                            : With --clone, copy the disk instead of using a backing file"
    echo "  --disk-resize <size>              : Resize the VM disk (e.g. 32G)"
    echo "  --disk-compact                    : Reclaim unused space in the qcow2 disk"
    echo "  --export <file.tar.gz>            : Export the VM (conf + files) to an archive"
    echo "  --import <file.tar.gz>            : Import an exported VM archive"
    echo "  --rename <newname>                : Rename this VM (conf + directory)"
    echo "  --send-keys <combo>               : Send keys to the VM via monitor (e.g. ctrl-alt-delete)"
    echo "  --type <text>                     : Type text into the VM via monitor sendkey"
    echo "  --list-vms                        : List VM configs in this directory with state"
    echo "  --doctor                          : Check the VM config against host capabilities"
    echo "  --autostart                       : Create a systemd user unit to autostart this VM"
    echo "  --usb-passthrough <vid:pid>       : Pass a USB device to the guest (e.g. 046d:c52b)"
    echo "  --dry-run                         : Print the QEMU command line without starting the VM"
    echo "  --offline                         : Override all network settings and start the VM offline"
    echo "  --shortcut                        : Create a desktop shortcut"
    echo "  --snapshot apply <tag>            : Apply/restore a snapshot."
    echo "  --snapshot create <tag>           : Create a snapshot."
    echo "  --snapshot delete <tag>           : Delete a snapshot."
    echo "  --snapshot info                   : Show disk/snapshot info."
    echo "  --status-quo                      : Do not commit any changes to disk/snapshot."
    echo "  --viewer <viewer>                 : Choose an alternative SPICE viewer. @Options: 'spicy' (default), 'remote-viewer', 'none'"
    echo "  --width <width>                   : Set VM screen width; requires '--height'"
    echo "  --height <height>                 : Set VM screen height; requires '--width'"
    echo "  --ssh-access                      : Enable remote ssh access support. 'local' (default), 'remote', 'clientipaddress'"
    echo "  --ssh-port <port>                 : Set SSH port manually"
    echo "  --spice-port <port>               : Set SPICE port manually"
    echo "  --public-dir <path>               : Expose share directory. @Options: '' (default: xdg-user-dir PUBLICSHARE), '<directory>', 'none'"
    echo "  --monitor <type>                  : Set monitor connection type. @Options: 'socket' (default), 'telnet', 'none'"
    echo "  --monitor-telnet-host <ip/host>   : Set telnet host for monitor. (default: 'localhost')"
    echo "  --monitor-telnet-port <port>      : Set telnet port for monitor. (default: '4440')"
    echo "  --monitor-cmd <cmd>               : Send command to monitor if available. (Example: system_powerdown)"
    echo "  --serial <type>                   : Set serial connection type. @Options: 'socket' (default), 'telnet', 'none'"
    echo "  --serial-telnet-host <ip/host>    : Set telnet host for serial. (default: 'localhost')"
    echo "  --serial-telnet-port <port>       : Set telnet port for serial. (default: '6660')"
    echo "  --keyboard <type>                 : Set keyboard. @Options: 'usb' (default), 'ps2', 'virtio'"
    echo "  --keyboard_layout <layout>        : Set keyboard layout: 'en-us' (default)"
    echo "  --mouse <type>                    : Set mouse. @Options: 'tablet' (default), 'ps2', 'usb', 'virtio'"
    echo "  --usb-controller <type>           : Set usb-controller. @Options: 'ehci' (default), 'xhci', 'none'"
    echo "  --sound-card <type>               : Set sound card. @Options: 'intel-hda' (default), 'ac97', 'es1370', 'sb16', 'usb-audio', 'virtio-sound-pci', 'none'"
    echo "  --sound-duplex <type>             : Set sound card duplex. @Options: 'hda-micro' (default: speaker/mic), 'hda-duplex' (line-in/line-out), 'hda-output' (output-only)"
    echo "  --extra_args <arguments>          : Pass additional arguments to qemu"
    echo "  --version                         : Print version"
}

# ---- emit.sh ----
# Contract emitters: the machine-readable output boundary (schemas/v1).
# The sibling emitters for --status/--list live inline in cli/status.sh.

# Emit the dry-run execution plan in the requested output format.
emit_execution_plan() {
    local vm_name="$1" command="$2"
    if [ "${OUTPUT_FORMAT}" == "json" ]; then
        printf '{"schemaVersion":%s,"vm":"%s","command":"%s"}\n' \
            "${QE_SCHEMA_VERSION}" "${vm_name}" "$(json_escape "${command}")"
    else
        echo "${command}"
    fi
}
