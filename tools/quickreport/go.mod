module github.com/alesanfe/quickemu/tools/quickreport

go 1.22

require golang.org/x/sys v0.30.0

require (
	github.com/ebitengine/purego v0.8.0 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/lufia/plan9stats v0.0.0-20211012122336-39d0f177ccd0 // indirect
	github.com/mitchellh/go-homedir v1.0.0 // indirect
	github.com/power-devops/perfstat v0.0.0-20210106213030-5aafc221ea8c // indirect
	github.com/shirou/gopsutil/v4 v4.24.9 // indirect
	github.com/tklauser/go-sysconf v0.3.12 // indirect
	github.com/tklauser/numcpus v0.6.1 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
)

require github.com/jaypipes/pcidb v1.0.1

require github.com/alesanfe/quickemu/tools/qelib v0.0.0

replace github.com/alesanfe/quickemu/tools/qelib => ../qelib
