// quickgetc: the data-driven downloader — bin/quickgetc parity.
// Downloads OS images described by the daily quickget_data.json
// catalog and writes .conf files. Pure-Go HTTP + decompression.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/alesanfe/quickemu/tools/qelib"
)

const version = "0.1.0"

var (
	arch      = ""
	operation = "" // "", download, config, show, test
)

func hostArch() string { return qelib.HostArch() }

func setArch(a string) string {
	switch a {
	case "x86_64", "amd64":
		return "x86_64"
	case "aarch64", "arm64", "armv8":
		return "aarch64"
	case "riscv64", "riscv":
		return "riscv64"
	}
	fmt.Printf("ERROR! %s is not a supported architecture.\n - Supported architectures: x86_64 aarch64 riscv64\n", a)
	os.Exit(1)
	return ""
}

func supportedOSNames(cat []OSEntry) string {
	var n []string
	for _, e := range cat {
		n = append(n, e.Name)
	}
	return strings.Join(n, " ")
}

func testResult(osName, rel, ed, url, result string) {
	s := osName + "-" + rel
	if ed != "" {
		s += "-" + ed
	}
	if result != "" {
		fmt.Printf("%s: %-35s %s\n", result, s, url)
	} else {
		fmt.Printf("%-36s %s\n", s+":", url)
	}
}

// testAll: iterate every release entry and report each source URL
// (show prints it, test probes it).
func testAll(cat []OSEntry, osName string) int {
	e := findOSEntry(cat, osName)
	if e == nil {
		fmt.Printf("ERROR! %s is not a supported OS.\n\n%s\n", osName, supportedOSNames(cat))
		return 1
	}
	for _, r := range e.Releases {
		for _, u := range allSourceURLs(r) {
			if operation == "show" {
				testResult(osName, r.Release, r.Edition, u, "")
			} else {
				res := "FAIL"
				if webCheck(u, nil) {
					res = "PASS"
				}
				testResult(osName, r.Release, r.Edition, u, res)
			}
		}
	}
	return 0
}

func showOSInfo(e *OSEntry) {
	fmt.Println("\n" + e.PrettyName)
	if e.Homepage != "" {
		fmt.Println(" - Website: " + e.Homepage)
	}
	if e.Description != "" {
		fmt.Println(" - Description: " + e.Description)
	}
}

// confSafe: values written inside quoted .conf lines must not carry
// quote characters or control bytes — a foreign catalog field with
// " or \n would inject extra conf entries (iso='/owned').
func confSafe(s string) bool {
	return !strings.ContainsAny(s, "\"'\n\r\x00")
}

// createVM: handle_sources + make_vm_config — download each image
// type's first source, decompress archives, then write the conf.

func openHomepage(cat []OSEntry, osName string) int {
	e := findOSEntry(cat, osName)
	if e == nil {
		fmt.Printf("ERROR! %s is not a supported OS.\n\n%s\n", osName, supportedOSNames(cat))
		return 1
	}
	if e.Homepage == "" {
		fmt.Printf("ERROR! No homepage found for %s\n", e.PrettyName)
		return 1
	}
	for _, b := range []string{"xdg-open", "sensible-browser", "x-www-browser", "gnome-open"} {
		if bin, err := exec.LookPath(b); err == nil {
			execCommand(bin, e.Homepage).Start()
			return 0
		}
	}
	return 1
}

func usage() {
	fmt.Println(`quickgetc — data-driven OS image downloader (Go)

Usage:
  quickgetc <os> <release> [edition]
  quickgetc --list | --list-csv | --list-json
  quickgetc --show [os] | --url [os] | --check [os]
  quickgetc --download <os> <release> [edition]
  quickgetc --create-config <vm-path> <path|url>
  quickgetc --open-homepage <os>
  quickgetc --arch <arch> ...
  quickgetc --version | --help`)
}

func listJSON(cat []OSEntry) {
	type row struct {
		DisplayName string `json:"Display Name"`
		OS          string `json:"OS"`
		Release     string `json:"Release"`
		Option      string `json:"Option"`
		Downloader  string `json:"Downloader"`
		PNG         string `json:"PNG"`
		SVG         string `json:"SVG"`
	}
	var rows []row
	for _, e := range cat {
		for _, r := range e.Releases {
			rows = append(rows, row{
				DisplayName: e.PrettyName, OS: e.Name, Release: r.Release,
				Option: r.Edition, Downloader: "",
				PNG: "https://quickemu-project.github.io/quickemu-icons/png/" + e.Name + "/" + e.Name + "-quickemu-white-pinkbg.png",
				SVG: "https://quickemu-project.github.io/quickemu-icons/svg/" + e.Name + "/" + e.Name + "-quickemu-white-pinkbg.svg",
			})
		}
	}
	// jq pretty-print parity: two-space indent, "key": value.
	b, _ := json.MarshalIndent(rows, "", "  ")
	fmt.Println(string(b))
}

func main() {
	args := os.Args[1:]
	var cat []OSEntry
	needCatalog := true
	var pos []string
	i := 0
	for i < len(args) {
		a := args[i]
		switch a {
		case "--arch", "-arch", "-a":
			if i+1 >= len(args) {
				fmt.Println("ERROR! You must specify an architecture.\n - Supported architectures: x86_64 aarch64 riscv64")
				os.Exit(1)
			}
			arch = setArch(args[i+1])
			i += 2
			continue
		case "--download", "-download":
			operation = "download"
		case "--create-config", "-create-config":
			// create_config <vm-path> <input>: positional args.
			if i+2 >= len(args)+1 && len(pos) < 1 {
				// consume remaining positionals below
			}
			operation = "config"
		case "--open-homepage", "-open-homepage":
			cat = populateCache()
			needCatalog = false
			if i+1 >= len(args) {
				fmt.Println("ERROR! You must specify an OS.")
				os.Exit(1)
			}
			os.Exit(openHomepage(cat, args[i+1]))
		case "--show", "-show":
			cat = populateCache()
			needCatalog = false
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				for _, e := range cat {
					showOSInfo(&e)
				}
				return
			}
			e := findOSEntry(cat, strings.ToLower(args[i+1]))
			if e == nil {
				fmt.Printf("ERROR! %s is not a supported OS.\n", args[i+1])
				os.Exit(1)
			}
			showOSInfo(e)
			return
		case "--version", "-version", "-V":
			fmt.Println(version)
			return
		case "--help", "-help", "--h", "-h":
			usage()
			return
		case "--url", "-url", "--check", "-check":
			if strings.Contains(a, "check") {
				operation = "test"
			} else {
				operation = "show"
			}
			cat = populateCache()
			needCatalog = false
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				for _, e := range cat {
					testAll(cat, e.Name)
				}
				return
			}
			os.Exit(testAll(cat, strings.ToLower(args[i+1])))
		case "--list-csv", "-list-csv", "list", "list_csv":
			listCSV(populateCache())
			return
		case "--list-json", "-list-json", "list_json":
			listJSON(populateCache())
			return
		case "--list", "-list":
			cat = populateCache()
			for _, e := range cat {
				for _, r := range e.Releases {
					fmt.Printf("%s %s %s\n", e.Name, r.Release, r.Edition)
				}
			}
			return
		default:
			if strings.HasPrefix(a, "-") {
				fmt.Println("ERROR! Not supported argument")
				fmt.Println("To see all possible arguments, use:\n   quickgetc -h  or  quickgetc --help")
				os.Exit(1)
			}
			pos = append(pos, a)
		}
		i++
	}
	if operation == "config" {
		if len(pos) < 2 {
			fmt.Println("ERROR! --create-config requires a VM name and a path or URL to an image.")
			os.Exit(1)
		}
		os.Exit(createConfig(pos[0], pos[1]))
	}
	if needCatalog {
		cat = populateCache()
	}
	if len(pos) == 0 {
		fmt.Println("ERROR! You must specify an operating system.\n- Supported Operating Systems:")
		fmt.Println(supportedOSNames(cat))
		fmt.Println("\nTo see all possible arguments, use:\n   quickgetc -h  or  quickgetc --help")
		os.Exit(1)
	}
	osName := strings.ToLower(pos[0])
	e := findOSEntry(cat, osName)
	if e == nil {
		fmt.Printf("ERROR! %s is not a supported OS.\n\n%s\n", osName, supportedOSNames(cat))
		os.Exit(1)
	}
	if len(pos) < 2 {
		showOSInfo(e)
		listReleasesDisplay(e)
		os.Exit(1)
	}
	rel := pos[1]
	relEntries := findReleaseEntries(e, rel)
	if len(relEntries) == 0 {
		fmt.Printf("ERROR! %s %s is not a supported release.\n\n", e.PrettyName, rel)
		listReleasesDisplay(e)
		os.Exit(1)
	}
	vmPath := osName + "-" + rel
	edition := ""
	if releaseRequiresEdition(relEntries) {
		if len(pos) < 3 {
			showOSInfo(e)
			fmt.Printf(" - Editions:\t%s\n", strings.Join(listEditions(e, rel), " "))
			fmt.Println("\nERROR! You must specify an edition.")
			os.Exit(1)
		}
		edition = pos[2]
		vmPath += "-" + edition
	} else if len(pos) > 2 {
		fmt.Printf("ERROR! %s %s does not require an edition.\n", e.PrettyName, rel)
		os.Exit(1)
	}
	entry, edEntries, err := findEditionEntry(relEntries, edition, arch, hostArch())
	if err != nil {
		// Edition empties error first (Bash checks EDITION_ENTRIES
		// before the arch preference), arch rejection second.
		if len(edEntries) == 0 {
			var eds []string
			seen := map[string]bool{}
			for _, r := range relEntries {
				if !seen[r.Edition] {
					seen[r.Edition] = true
					eds = append(eds, r.Edition)
				}
			}
			fmt.Printf("ERROR! %s is not a supported %s %s edition.\n - Supported editions: %s\n",
				edition, e.PrettyName, rel, strings.Join(eds, " "))
		} else if arch != "" {
			var archs []string
			for _, r := range edEntries {
				archs = append(archs, r.Arch)
			}
			suffix := ""
			if edition != "" {
				suffix = " " + edition
			}
			fmt.Printf("ERROR! %s is not a supported architecture for %s %s%s.\n - Supported architectures: %s\n",
				arch, e.PrettyName, rel, suffix, strings.Join(archs, " "))
		}
		os.Exit(1)
	}
	if arch != "" {
		vmPath += "-" + arch
	}
	os.Exit(createVM(e, entry, vmPath))
}
