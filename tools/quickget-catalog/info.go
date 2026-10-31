// info.go — OS metadata (os_info parity) + web_check reachability.
package main

import (
	"encoding/json"
	"github.com/alesanfe/quickemu/tools/qelib"
	"os"
)

type InfoEntry struct {
	Name        string `json:"name"`
	Credentials string `json:"credentials"`
	Homepage    string `json:"homepage"`
	Info        string `json:"info"`
}

type infoFile struct {
	Entries map[string]InfoEntry `json:"entries"`
}

func loadInfo(path string) (map[string]InfoEntry, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f infoFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	return f.Entries, nil
}

func archSupported(e *Entry, arch string) bool {
	return qelib.ArchSupported(e.Arch, arch)
}

// webCheck mirrors web_check(): HEAD with redirects first, then a
// single-byte ranged GET fallback for presigned URLs that reject HEAD.
var webCheck = func(url string) bool { return qelib.WebCheck(url, nil) }
