module github.com/alesanfe/quickemu/tools/quickemu-cli

go 1.22

require golang.org/x/sys v0.30.0

require (
	github.com/jaypipes/pcidb v1.0.1 // indirect
	github.com/mitchellh/go-homedir v1.0.0 // indirect
)

require github.com/alesanfe/quickemu/tools/qelib v0.0.0

replace github.com/alesanfe/quickemu/tools/qelib => ../qelib
