Describe 'lib/host/'
Include lib/quickemu/host/cpu.sh
Include lib/quickemu/host/memory.sh

It 'normalises host arch'
When call host_arch
The output should not be blank
End

It 'reports a positive core count'
When call host_cpu_cores
The output should match pattern '[0-9]*'
End

It 'reports a boolean KVM probe'
When call host_has_kvm
The status should be defined
End
End
