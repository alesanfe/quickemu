// info.go — OS metadata (os_info parity) + web_check reachability.
package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"time"
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
	list := e.Arch
	if len(list) == 0 {
		list = []string{"amd64"}
	}
	for _, a := range list {
		if a == arch {
			return true
		}
	}
	return false
}

var checkClient = &http.Client{Timeout: 30 * time.Second}

// webCheck mirrors web_check(): HEAD with redirects first, then a
// single-byte ranged GET fallback for presigned URLs that reject HEAD.
func webCheck(url string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return false
	}
	resp, err := checkClient.Do(req)
	if err == nil {
		resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 400 {
			return true
		}
	}
	req, err = http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	req.Header.Set("Range", "bytes=0-0")
	resp, err = checkClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 400
}
