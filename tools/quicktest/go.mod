module github.com/alesanfe/quickemu/tools/quicktest

go 1.22

require (
	github.com/BurntSushi/toml v1.4.0
	github.com/alesanfe/quickemu/tools/qelib v0.0.0
	github.com/diskfs/go-diskfs v1.9.4
	github.com/google/uuid v1.6.0
	github.com/kdomanski/iso9660 v0.4.0
	github.com/simia-tech/crypt v0.5.1
	golang.org/x/crypto v0.46.0
)

replace github.com/alesanfe/quickemu/tools/qelib => ../qelib
