// scrape_api.go — JSON-API resolvers (scrape batch).
package main

import (
	"encoding/json"
	"fmt"
)

func scrapeAPI(osName, release, edition, arch string) (string, string, error) {
	switch osName {
	case "fedora-coreos":
		body, err := fetchString("https://builds.coreos.fedoraproject.org/streams/" + release + ".json")
		if err != nil {
			return "", "", err
		}
		var j struct {
			Architectures map[string]struct {
				Artifacts map[string]struct {
					Formats map[string]struct {
						Disk struct {
							Location string `json:"location"`
							Sha256   string `json:"sha256"`
						} `json:"disk"`
					} `json:"formats"`
				} `json:"artifacts"`
			} `json:"architectures"`
		}
		if err := json.Unmarshal([]byte(body), &j); err != nil {
			return "", "", err
		}
		f := j.Architectures["x86_64"].Artifacts["qemu"].Formats["qcow2.xz"]
		if f.Disk.Location == "" {
			return "", "", fmt.Errorf("fedora-coreos: no qemu artifact")
		}
		return f.Disk.Location, f.Disk.Sha256, nil
	}
	return "", "", errScrapeSkip
}
