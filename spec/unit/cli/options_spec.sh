Describe 'command line option parsing'
Include lib/quickemu/cli/options.sh

setup() {
    ACTIONS=()
    SHORTCUT_OPTIONS=''
    ACCESS='' BRAILLE='' CPU_PINNING='' display='' FULLSCREEN=''
    IGNORE_TSC_WARNING='' OUTPUT_FORMAT='' SCREENSHOT_OUT=''
    CLONE_NAME='' CLONE_FULL='' DISK_SIZE_ARG='' EXPORT_FILE=''
    IMPORT_FILE='' RENAME_TO='' SEND_KEYS_ARG='' TYPE_TEXT=''
    SNAPSHOT_ACTION='' SNAPSHOT_TAG='' STATUS_QUO='' VM='' viewer=''
    width='' height='' SSH_ACCESS='' ssh_port='' spice_port=''
    PUBLIC='' monitor='' MONITOR_CMD='' monitor_telnet_host=''
    monitor_telnet_port='' serial='' serial_telnet_host=''
    serial_telnet_port='' keyboard='' keyboard_layout='' mouse=''
    usb_controller='' extra_args='' sound_card='' sound_duplex=''
    usb_devices=() DRY_RUN='' network='' VERSION='1.2.3'
}
BeforeEach 'setup'

# External hooks the parser calls; stubs record they were reached.
usage() { echo 'USAGE-STUB'; }
display_param_check() { :; }
ignore_msrs_always() { echo 'MSRS-STUB'; }

# Options that exit the process must run in a subshell so the spec survives.
cli_in_subshell() { ( cli_parse_args "$@" ); }

Describe 'option to variable mapping'
Parameters
#   option           value        variable          expected
    --access         remote       ACCESS            remote
    --cpu-pinning    0-3          CPU_PINNING       0-3
    --display        spice        display           spice
    --viewer         spicy        viewer            spicy
    --width          1920         width             1920
    --height         1080         height            1080
    --ssh-access     publickey    SSH_ACCESS        publickey
    --ssh-port       2222         ssh_port          2222
    --spice-port     5930         spice_port        5930
    --public-dir     /shared      PUBLIC            /shared
    --monitor        stdio        monitor           stdio
    --monitor-cmd    info\ x      MONITOR_CMD       'info x'
    --monitor-telnet-host 127.0.0.1 monitor_telnet_host 127.0.0.1
    --monitor-telnet-port 4440    monitor_telnet_port 4440
    --serial         telnet       serial            telnet
    --serial-telnet-host 0.0.0.0  serial_telnet_host 0.0.0.0
    --serial-telnet-port 6660     serial_telnet_port 6660
    --keyboard       es           keyboard          es
    --keyboard_layout en-gb       keyboard_layout   en-gb
    --mouse          tablet       mouse             tablet
    --usb-controller xhci         usb_controller    xhci
    --sound-card     intel-hda    sound_card        intel-hda
    --sound-duplex   full         sound_duplex      full
    --vm             windows.conf VM                windows.conf
    --disk-resize    64G          DISK_SIZE_ARG     64G
    --export         vm.qcow2     EXPORT_FILE       vm.qcow2
    --import         in.qcow2     IMPORT_FILE       in.qcow2
    --rename         new-name     RENAME_TO         new-name
    --send-keys      ctrl-alt-del SEND_KEYS_ARG     ctrl-alt-del
    --type           'hello vm'   TYPE_TEXT         'hello vm'
    --format         json         OUTPUT_FORMAT     json
End
It 'stores the option value in its variable'
    When call cli_parse_args "$1" "$2"
    The status should be success
    The variable "$3" should eq "$4"
End
End

Describe 'boolean flags'
Parameters
    --braille              BRAILLE              on
    --fullscreen           FULLSCREEN           --full-screen
    --full-screen          FULLSCREEN           --full-screen
    --ignore-tsc-warning   IGNORE_TSC_WARNING   1
    --full                 CLONE_FULL           true
    --dry-run              DRY_RUN              true
    --offline              network              none
    --status-quo           STATUS_QUO           -snapshot
End
It 'sets the flag without consuming an extra argument'
    When call cli_parse_args "$1" --vm x.conf
    The status should be success
    The variable "$2" should eq "$3"
    The variable VM should eq 'x.conf'
End
End

Describe 'action registration'
Parameters
    --delete        delete_disk
    --delete-disk   delete_disk
    --delete-plan   delete_plan
    --delete-vm     delete_vm
    --kill          kill_vm
    --shutdown      shutdown_vm
    --reset         reset_vm
    --capabilities  capabilities
    --status        status_vm
    --pause         pause_vm
    --resume        resume_vm
    --list-vms      list_vms
    --doctor        doctor_vm
    --autostart     autostart_create
    --shortcut      shortcut_create
    --disk-compact  disk_compact
End
It 'queues the action for execution'
    When call cli_parse_args "$1"
    The status should be success
    The variable 'ACTIONS[*]' should include "$2"
End
End

Describe 'actions with payloads'
It 'queues disk_resize with its size argument'
    When call cli_parse_args --disk-resize 64G
    The status should be success
    The variable 'ACTIONS[*]' should include 'disk_resize'
    The variable DISK_SIZE_ARG should eq '64G'
End
It 'queues export_vm and import_vm with their files'
    When call cli_parse_args --export out.qcow2 --import in.qcow2
    The status should be success
    The variable 'ACTIONS[*]' should include 'export_vm'
    The variable 'ACTIONS[*]' should include 'import_vm'
    The variable EXPORT_FILE should eq 'out.qcow2'
    The variable IMPORT_FILE should eq 'in.qcow2'
End
It 'queues rename_vm with the target name'
    When call cli_parse_args --rename vm2
    The status should be success
    The variable 'ACTIONS[*]' should include 'rename_vm'
    The variable RENAME_TO should eq 'vm2'
End
It 'collects repeated usb passthrough devices'
    When call cli_parse_args --usb-passthrough 0456:7890 --usb-passthrough 1234:abcd
    The status should be success
    The variable 'usb_devices[*]' should include '0456:7890'
    The variable 'usb_devices[*]' should include '1234:abcd'
End
It 'appends repeated extra_args into one string'
    extra_args='first'
    When call cli_parse_args --extra_args ' second'
    The status should be success
    The variable extra_args should eq 'first second'
End
End

Describe 'optional argument detection'
It 'consumes a clone name that does not look like an option'
    When call cli_parse_args --clone my-copy --vm a.conf
    The status should be success
    The variable 'ACTIONS[*]' should include 'clone_vm'
    The variable CLONE_NAME should eq 'my-copy'
    The variable VM should eq 'a.conf'
End
It 'leaves CLONE_NAME empty when the next token is another option'
    When call cli_parse_args --clone --vm a.conf
    The status should be success
    The variable 'ACTIONS[*]' should include 'clone_vm'
    The variable CLONE_NAME should eq ''
    The variable VM should eq 'a.conf'
End
It 'consumes a screenshot output path'
    When call cli_parse_args --screenshot shot.png --vm a.conf
    The status should be success
    The variable 'ACTIONS[*]' should include 'screenshot_vm'
    The variable SCREENSHOT_OUT should eq 'shot.png'
End
It 'leaves SCREENSHOT_OUT empty when followed by an option'
    When call cli_parse_args --screenshot --vm a.conf
    The status should be success
    The variable 'ACTIONS[*]' should include 'screenshot_vm'
    The variable SCREENSHOT_OUT should eq ''
End
End

Describe 'shortcut command construction'
It 'records pass-through options for the generated shortcut'
    When call cli_parse_args --access remote --viewer spicy --width 1920 --shortcut
    The status should be success
    The variable SHORTCUT_OPTIONS should include '--access remote'
    The variable SHORTCUT_OPTIONS should include '--viewer spicy'
    The variable SHORTCUT_OPTIONS should include '--width 1920'
    The variable 'ACTIONS[*]' should include 'shortcut_create'
End
End

Describe 'snapshot subcommand parsing'
It 'requires an action'
    When call cli_in_subshell --snapshot
    The status should be failure
    The output should include 'needs an action'
End
It 'requires a tag for apply'
    When call cli_in_subshell --snapshot apply
    The status should be failure
    The output should include 'needs a tag'
End
It 'accepts info without a tag'
    When call cli_parse_args --snapshot info --vm a.conf
    The status should be success
    The variable SNAPSHOT_ACTION should eq 'info'
End
It 'accepts list without a tag'
    When call cli_parse_args --snapshot list --vm a.conf
    The status should be success
    The variable SNAPSHOT_ACTION should eq 'list'
End
It 'stores action and tag for create'
    When call cli_parse_args --snapshot create baseline --vm a.conf
    The status should be success
    The variable SNAPSHOT_ACTION should eq 'create'
    The variable SNAPSHOT_TAG should eq 'baseline'
End
End

Describe 'validation and termination paths'
It 'rejects an unsupported output format'
    When call cli_in_subshell --format xml
    The status should be failure
    The output should include "'--format' must be 'human' or 'json'"
End
It 'rejects unknown parameters with a clear error'
    When call cli_in_subshell --frobnicate
    The status should be failure
    The output should include 'is not a supported parameter'
End
It 'prints usage and fails with no arguments'
    When call cli_in_subshell
    The status should be failure
    The output should include 'USAGE-STUB'
End
It 'prints the version and exits'
    When call cli_in_subshell --version
    The status should be success
    The output should eq '1.2.3'
End
It 'prints usage for --help'
    When call cli_in_subshell --help
    The status should be success
    The output should include 'USAGE-STUB'
End
It 'runs the MSR helper and exits for --ignore-msrs-always'
    When call cli_in_subshell --ignore-msrs-always
    The status should be success
    The output should include 'MSRS-STUB'
End
End
End
