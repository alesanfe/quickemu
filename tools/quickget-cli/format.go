// tools/quickget-cli/format.go: list/show formatting, usage text, icon mapping
package main

import (
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/alesanfe/quickemu/tools/qelib"
)

// windows language tables — shared in qelib (languages_windows /
// languages_windows-server contract with quickget-vm).
var winLangs = qelib.WinLangs
var winSrvLangs = qelib.WinServerLangs

func iconFunc(osName string) string {
	switch {
	case strings.Contains(osName, "ubuntu-server"):
		return "ubuntu-server"
	case strings.Contains(osName, "ubuntu"):
		return "ubuntu"
	}
	return osName
}

func csvRows(cat []Entry, info map[string]InfoEntry) [][]string {
	hasZsync := false
	if _, err := exec.LookPath("zsync"); err == nil {
		hasZsync = true
	}
	var rows [][]string
	for _, e := range cat {
		inf := info[e.OS]
		fn := iconFunc(e.OS)
		png := "https://quickemu-project.github.io/quickemu-icons/png/" + fn + "/" + fn + "-quickemu-white-pinkbg.png"
		svg := "https://quickemu-project.github.io/quickemu-icons/svg/" + fn + "/" + fn + "-quickemu-white-pinkbg.svg"
		for _, rel := range e.Releases {
			dl := ""
			if hasZsync && strings.Contains(e.OS, "ubuntu") && strings.Contains(rel, "daily") {
				dl = "zsync"
			}
			if len(e.Editions) > 0 {
				for _, ed := range e.Editions {
					rows = append(rows, []string{inf.Name, e.OS, rel, ed, dl, png, svg})
				}
			} else if e.OS == "windows" {
				for _, l := range winLangs {
					rows = append(rows, []string{inf.Name, e.OS, rel, l, dl, png, svg})
				}
			} else if e.OS == "windows-server" {
				for _, l := range winSrvLangs {
					rows = append(rows, []string{inf.Name, e.OS, rel, l, dl, png, svg})
				}
			} else {
				rows = append(rows, []string{inf.Name, e.OS, rel, "", dl, png, svg})
			}
		}
	}
	// sort -s -t',' -k2,2: stable sort by column 2 (OS).
	slices.SortStableFunc(rows, func(a, b []string) int { return strings.Compare(a[1], b[1]) })
	return rows
}

// errSpecifyOS: error_specify_os parity — message, supported-OS list,
// and the help hint.
func errSpecifyOS() {
	fmt.Fprintln(os.Stdout, "ERROR! You must specify an operating system.")
	fmt.Fprintln(os.Stdout, "- Supported Operating Systems:")
	var names []string
	for _, e := range loadCatalog() {
		names = append(names, e.OS)
	}
	fmt.Fprintln(os.Stdout, strings.Join(names, " "))
	fmt.Fprintln(os.Stdout, "\nTo see all possible arguments, use:\n   quickget -h  or  quickget --help")
}

// wrapLines: `fmt -w 80` approximation — space-separated words wrapped
// at width.
func wrapLines(words []string, width int) string {
	var b strings.Builder
	col := 0
	for _, w := range words {
		if col > 0 && col+1+len(w) > width {
			b.WriteByte('\n')
			col = 0
		}
		if col > 0 {
			b.WriteByte(' ')
			col++
		}
		b.WriteString(w)
		col += len(w)
	}
	return b.String()
}

func globDir(dir string) []string {
	var out []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		return out
	}
	for _, e := range entries {
		if !e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out
}

// wrapCols: fmt -w 80 — greedy word wrap.
func wrapCols(s string, w int) string {
	var lines []string
	line := ""
	for _, word := range strings.Fields(s) {
		if line == "" {
			line = word
		} else if len(line)+1+len(word) <= w {
			line += " " + word
		} else {
			lines = append(lines, line)
			line = word
		}
	}
	return strings.Join(append(lines, line), "\n")
}
