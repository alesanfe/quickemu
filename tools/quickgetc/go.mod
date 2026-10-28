module github.com/alesanfe/quickemu/tools/quickgetc

go 1.22

require (
	github.com/klauspost/compress v1.17.11
	github.com/ulikunitz/xz v0.5.12
)

require github.com/alesanfe/quickemu/tools/qelib v0.0.0

replace github.com/alesanfe/quickemu/tools/qelib => ../qelib
