module github.com/alesanfe/quickemu/tools/quickget-vm

go 1.25.0

require (
	github.com/bodgit/sevenzip v1.6.5
	github.com/diskfs/go-diskfs v1.9.4
	github.com/kdomanski/iso9660 v0.4.0
	github.com/klauspost/compress v1.19.0
	github.com/simia-tech/crypt v0.5.1
	github.com/ulikunitz/xz v0.5.15
)

require (
	github.com/anchore/go-lzo v0.1.0 // indirect
	github.com/andybalholm/brotli v1.2.2 // indirect
	github.com/bodgit/plumbing v1.3.0 // indirect
	github.com/bodgit/windows v1.0.1 // indirect
	github.com/djherbis/times v1.6.0 // indirect
	github.com/elliotwutingfeng/asciiset v0.0.0-20260129054604-cfde2086bc57 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/pierrec/lz4/v4 v4.1.27 // indirect
	github.com/pkg/xattr v0.4.12 // indirect
	github.com/sirupsen/logrus v1.9.4 // indirect
	github.com/spf13/afero v1.15.0 // indirect
	github.com/stangelandcl/ppmd v0.1.1 // indirect
	go4.org v0.0.0-20260112195520-a5071408f32f // indirect
	golang.org/x/crypto v0.46.0 // indirect
	golang.org/x/sys v0.43.0 // indirect
	golang.org/x/text v0.40.0 // indirect
)

require github.com/alesanfe/quickemu/tools/qelib v0.0.0

replace github.com/alesanfe/quickemu/tools/qelib => ../qelib
