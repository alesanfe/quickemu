// image — image handling: shared helpers (split of image.go).
package main

import (
	"os/exec"

	"github.com/alesanfe/quickemu/tools/qelib"
)

// finalURL: web_redirect parity — follow redirects and return the
// resolved URL (used to name URL-sourced images).
var finalURL = func(url string) string {
	if u, err := qelib.FinalURL(url); err == nil && u != "" {
		return u
	}
	return url
}

func copyFile(src, dst string) error { return qelib.CopyFile(src, dst) }

func run(name string, args ...string) error {
	bin, err := exec.LookPath(name)
	if err != nil {
		return err
	}
	return execCommand(bin, args...).Run()
}
