# shellcheck shell=bash
Describe 'vm network helpers'
Include lib/quickemu/support/filesystem.sh
Include lib/quickemu/vm/network.sh

Describe 'vm_net_mode'
  Parameters
    "#default" ""        user
    "#slirp"    "slirp"  user
    "#none"     "none"   none
    "#restrict" "restrict" restrict
    "#passt"    "passt"  passt
    "#bridge"   "br0"    bridge
  End
  It "maps config '$2' to mode $3"
    When call vm_net_mode "$2"
    The output should eq "$3"
  End
End

Describe 'vm_net_bridge'
It 'returns the iface for a bridge name'
  When call vm_net_bridge br0
  The output should eq 'br0'
End
It 'returns empty for user mode'
  When call test -z "$(vm_net_bridge slirp)"
  The status should be success
End
End
End
