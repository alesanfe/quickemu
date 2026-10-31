module github.com/alesanfe/quickemu/tools/quicktest

go 1.25.0

require (
	github.com/BurntSushi/toml v1.4.0
	github.com/alesanfe/quickemu/tools/qelib v0.0.0
	github.com/diskfs/go-diskfs v1.9.4
	github.com/google/uuid v1.6.0
	github.com/kdomanski/iso9660 v0.4.0
	github.com/simia-tech/crypt v0.5.1
	golang.org/x/crypto v0.46.0
)

require (
	github.com/anchore/go-lzo v0.1.0 // indirect
	github.com/djherbis/times v1.6.0 // indirect
	github.com/ebitengine/purego v0.8.0 // indirect
	github.com/elliotwutingfeng/asciiset v0.0.0-20260129054604-cfde2086bc57 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/klauspost/compress v1.18.5 // indirect
	github.com/lufia/plan9stats v0.0.0-20211012122336-39d0f177ccd0 // indirect
	github.com/pierrec/lz4/v4 v4.1.26 // indirect
	github.com/pkg/xattr v0.4.12 // indirect
	github.com/power-devops/perfstat v0.0.0-20210106213030-5aafc221ea8c // indirect
	github.com/shirou/gopsutil/v4 v4.24.9 // indirect
	github.com/sirupsen/logrus v1.9.4 // indirect
	github.com/tklauser/go-sysconf v0.3.12 // indirect
	github.com/tklauser/numcpus v0.6.1 // indirect
	github.com/ulikunitz/xz v0.5.15 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	golang.org/x/sys v0.43.0 // indirect
)

replace github.com/alesanfe/quickemu/tools/qelib => ../qelib
