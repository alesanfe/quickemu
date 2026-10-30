// tools/quickget-cli/catalog.go: catalog/info loading, entry lookup, sibling-tool resolver
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/alesanfe/quickemu/tools/qelib"
)

func hostArch() string {
	// Normalised quickget arch names (amd64/arm64, not QEMU's
	// x86_64/aarch64) — a hardcoded amd64 picked x86 URLs on ARM hosts.
	if runtime.GOARCH == "arm64" {
		return "arm64"
	}
	return "amd64"
}

func dataPath(name string) string {
	if p := os.Getenv("QUICKEMU_CATALOG_DIR"); p != "" {
		return filepath.Join(p, name)
	}
	return qelib.DataFile("catalog", "data", name)
}

type Entry struct {
	OS       string   `json:"os"`
	Releases []string `json:"releases"`
	Editions []string `json:"editions"`
	Arch     []string `json:"arch"`
}

type catalogFile struct {
	Entries []Entry `json:"entries"`
}

type InfoEntry struct {
	Name, Credentials, Homepage, Info string
}

type infoFile struct {
	Entries map[string]InfoEntry `json:"entries"`
}

func loadCatalog() []Entry {
	b, err := os.ReadFile(dataPath("catalog.json"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "catalog: %v\n", err)
		os.Exit(1)
	}
	var c catalogFile
	if err := json.Unmarshal(b, &c); err != nil {
		fmt.Fprintf(os.Stderr, "catalog: parse: %v\n", err)
		os.Exit(1)
	}
	return c.Entries
}

func loadInfo() map[string]InfoEntry {
	b, err := os.ReadFile(dataPath("info.json"))
	if err != nil {
		return map[string]InfoEntry{}
	}
	var f infoFile
	if err := json.Unmarshal(b, &f); err != nil {
		fmt.Fprintf(os.Stderr, "info: parse: %v\n", err)
		os.Exit(1)
	}
	return f.Entries
}

func findEntry(cat []Entry, name string) *Entry {
	for i := range cat {
		if cat[i].OS == name {
			return &cat[i]
		}
	}
	return nil
}

// findTool resolves a sibling Go tool (PATH → exe dir → dev-tree).
func findTool(name string) (string, error) { return qelib.FindTool(name) }

func tool(name string, args ...string) (string, error) {
	bin, err := findTool(name)
	if err != nil {
		return "", err
	}
	out, err := execCommand(bin, args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func runPassthru(name string, args ...string) int {
	bin, err := findTool(name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 1
	}
	cmd := execCommand(bin, args...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return 1
	}
	return 0
}
