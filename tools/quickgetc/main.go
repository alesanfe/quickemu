// quickgetc: the data-driven downloader — bin/quickgetc parity.
// Downloads OS images described by the daily quickget_data.json
// catalog and writes .conf files. Pure-Go HTTP + decompression.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const version = "0.1.0"

var (
	arch      = ""
	operation = "" // "", download, config, show, test
)

func hostArch() string {
	if runtime.GOARCH == "arm64" {
		return "aarch64"
	}
	return "x86_64"
}

func setArch(a string) string {
	switch a {
	case "x86_64", "amd64":
		return "x86_64"
	case "aarch64", "arm64", "armv8":
		return "aarch64"
	case "riscv64", "riscv":
		return "riscv64"
	}
	fmt.Fprintf(os.Stderr, "ERROR! %s is not a supported architecture.\n - Supported architectures: x86_64 aarch64 riscv64\n", a)
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
		fmt.Fprintf(os.Stderr, "ERROR! %s is not a supported OS.\n\n%s\n", osName, supportedOSNames(cat))
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

// createVM: handle_sources + make_vm_config — download each image
// type's first source, decompress archives, then write the conf.
func createVM(e *OSEntry, entry *Release, vmPath string) int {
	images := map[string]string{}
	for _, t := range imageTypes {
		srcs := entry.sourcesFor(t)
		if len(srcs) == 0 {
			continue
		}
		s := srcs[0]
		switch {
		case s.Web != nil:
			final := handleWebSource(t, s, vmPath)
			if final == "" {
				return 1
			}
			images[t] = final
		case s.FileName != "":
			images[t] = vmPath + "/" + s.FileName
		case s.Custom != "":
			fmt.Fprintln(os.Stderr, "ERROR! Custom source not implemented")
			return 1
		default:
			fmt.Fprintf(os.Stderr, "ERROR! No source found for %s\n", t)
			return 1
		}
	}
	if operation == "download" {
		return 0
	}
	return makeVMConfig(e, entry, vmPath, images)
}

// handleWebSource: download + verify + decompress a web source.
func handleWebSource(imageType string, s Source, vmPath string) string {
	url := s.Web.URL
	fileName := s.Web.FileName
	if fileName == "" {
		fileName = filepath.Base(url)
	}
	if webGet(url, vmPath, fileName, nil) != 0 {
		return ""
	}
	if s.Web.Checksum != "" {
		if !checkHash(filepath.Join(vmPath, fileName), s.Web.Checksum) {
			return ""
		}
	}
	final := filepath.Join(vmPath, fileName)
	if operation == "download" {
		final = fileName
		vmPath = "."
	}
	return extractArchive(s.Web.ArchiveFormat, final, vmPath, imageType)
}

// makeVMConfig: write <vmpath>.conf with guest_os/disk_img/arch_vm/
// ram/image keys.
func makeVMConfig(e *OSEntry, entry *Release, vmPath string, images map[string]string) int {
	conf := vmPath + ".conf"
	if _, err := os.Stat(conf); err == nil {
		return 0
	}
	guest := entry.GuestOS
	if guest == "" {
		guest = e.GuestOS
	}
	if guest == "" {
		guest = "linux"
	}
	qe, _ := exec.LookPath("quickemu")
	if qe == "" {
		qe = "quickemu"
	}
	var b strings.Builder
	b.WriteString("#!" + qe + " --vm\n")
	b.WriteString("guest_os=\"" + guest + "\"\n")
	b.WriteString("disk_img=\"" + vmPath + "/disk.qcow2\"\n")
	b.WriteString("arch_vm=\"" + entry.Arch + "\"\n")
	fmt.Println("Making " + conf)
	for _, t := range imageTypes {
		if v, ok := images[t]; ok {
			b.WriteString(t + "='" + v + "'\n")
		}
	}
	if entry.RAM > 0 {
		fmt.Fprintf(&b, "ram=%d\n", entry.RAM/1024/1024/1024)
	}
	if err := os.WriteFile(conf, []byte(b.String()), 0755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

// createConfig: --create-config <vm-path> <path|url> — simple image
// move + minimal conf (the quickgetc variant, not quickget's).
func createConfig(vmPath, input string) int {
	if vmPath == "" || input == "" {
		fmt.Fprintln(os.Stderr, "ERROR! --create-config requires a VM name and a path or URL to an image.")
		return 1
	}
	if err := os.Mkdir(vmPath, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR! Could not create directory: %s. Please verify that it does not already exist.\n", vmPath)
		return 1
	}
	if strings.HasPrefix(input, "http://") || strings.HasPrefix(input, "https://") {
		input = webRedirect(input)
		low := strings.ToLower(input)
		if !strings.HasSuffix(low, ".iso") && !strings.HasSuffix(low, ".img") && !strings.HasSuffix(low, ".qcow2") {
			fmt.Fprintln(os.Stderr, "ERROR! Only ISO, IMG and QCOW2 file types are supported for --create-config")
			return 1
		}
		if webGet(input, vmPath, filepath.Base(input), nil) != 0 {
			return 1
		}
		input = filepath.Join(vmPath, filepath.Base(input))
	}
	if _, err := os.Stat(input); err != nil {
		fmt.Fprintln(os.Stderr, "ERROR! The input must be a valid URL or path to an ISO, IMG, or QCOW2 file.")
		return 1
	}
	low := strings.ToLower(input)
	imageKey := ""
	switch {
	case strings.HasSuffix(low, ".iso"):
		imageKey = "iso"
	case strings.HasSuffix(low, ".img"):
		imageKey = "img"
	case strings.HasSuffix(low, ".qcow2"):
		imageKey = "disk_img"
	default:
		fmt.Fprintln(os.Stderr, "ERROR! Only ISO, IMG and QCOW2 file types are supported for --create-config")
		return 1
	}
	base := filepath.Base(input)
	if filepath.Dir(input) != vmPath {
		os.Rename(input, filepath.Join(vmPath, base))
	}
	guest := "linux"
	switch {
	case strings.Contains(low, "windows"), strings.Contains(low, "win"):
		guest = "windows"
	case strings.Contains(low, "macos"):
		guest = "macos"
	case strings.Contains(low, "bsd"):
		guest = "freebsd"
	}
	fmt.Printf("Creating %s.conf for %s (guest_os=%s).\n", vmPath, base, guest)
	qe, _ := exec.LookPath("quickemu")
	if qe == "" {
		qe = "quickemu"
	}
	conf := "#!" + qe + " --vm\n" +
		"guest_os=\"" + guest + "\"\n" +
		imageKey + "=\"" + vmPath + "/" + base + "\"\n"
	if err := os.WriteFile(vmPath+".conf", []byte(conf), 0755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("\nTo start your virtual machine run:\n    quickemu --vm %s.conf\n", vmPath)
	return 0
}

func openHomepage(cat []OSEntry, osName string) int {
	e := findOSEntry(cat, osName)
	if e == nil || e.Homepage == "" {
		pretty := osName
		if e != nil {
			pretty = e.PrettyName
		}
		fmt.Fprintf(os.Stderr, "ERROR! No homepage found for %s\n", pretty)
		return 1
	}
	for _, b := range []string{"xdg-open", "sensible-browser", "x-www-browser", "gnome-open"} {
		if bin, err := exec.LookPath(b); err == nil {
			exec.Command(bin, e.Homepage).Start()
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
	b, _ := json.Marshal(rows)
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
				fmt.Fprintln(os.Stderr, "ERROR! You must specify an architecture.\n - Supported architectures: x86_64 aarch64 riscv64")
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
				fmt.Fprintln(os.Stderr, "ERROR! You must specify an OS.")
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
				fmt.Fprintf(os.Stderr, "ERROR! %s is not a supported OS.\n", args[i+1])
				os.Exit(1)
			}
			showOSInfo(e)
			listReleasesDisplay(e)
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
				fmt.Fprintln(os.Stderr, "ERROR! Not supported argument")
				fmt.Fprintln(os.Stderr, "To see all possible arguments, use:\n   quickgetc -h  or  quickgetc --help")
				os.Exit(1)
			}
			pos = append(pos, a)
		}
		i++
	}
	if operation == "config" {
		if len(pos) < 2 {
			fmt.Fprintln(os.Stderr, "ERROR! --create-config requires a VM name and a path or URL to an image.")
			os.Exit(1)
		}
		os.Exit(createConfig(pos[0], pos[1]))
	}
	if needCatalog {
		cat = populateCache()
	}
	if len(pos) == 0 {
		fmt.Fprintln(os.Stderr, "ERROR! You must specify an operating system.\n- Supported Operating Systems:")
		fmt.Fprintln(os.Stderr, supportedOSNames(cat))
		fmt.Fprintln(os.Stderr, "\nTo see all possible arguments, use:\n   quickgetc -h  or  quickgetc --help")
		os.Exit(1)
	}
	osName := strings.ToLower(pos[0])
	e := findOSEntry(cat, osName)
	if e == nil {
		fmt.Fprintf(os.Stderr, "ERROR! %s is not a supported OS.\n\n%s\n", osName, supportedOSNames(cat))
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
		fmt.Fprintf(os.Stderr, "ERROR! %s %s is not a supported release.\n\n", e.PrettyName, rel)
		listReleasesDisplay(e)
		os.Exit(1)
	}
	vmPath := osName + "-" + rel
	edition := ""
	if releaseRequiresEdition(relEntries) {
		if len(pos) < 3 {
			showOSInfo(e)
			fmt.Fprintf(os.Stderr, " - Editions:\t%s\n", strings.Join(listEditions(e, rel), " "))
			fmt.Fprintln(os.Stderr, "\nERROR! You must specify an edition.")
			os.Exit(1)
		}
		edition = pos[2]
		vmPath += "-" + edition
	} else if len(pos) > 2 {
		fmt.Fprintf(os.Stderr, "ERROR! %s %s does not require an edition.\n", e.PrettyName, rel)
		os.Exit(1)
	}
	entry, edEntries, err := findEditionEntry(relEntries, edition, arch, hostArch())
	if err != nil {
		if arch != "" {
			var archs []string
			for _, r := range edEntries {
				archs = append(archs, r.Arch)
			}
			fmt.Fprintf(os.Stderr, "ERROR! %s is not a supported architecture for %s %s.\n - Supported architectures: %s\n",
				arch, e.PrettyName, rel, strings.Join(archs, " "))
		} else {
			var eds []string
			for _, r := range relEntries {
				eds = append(eds, r.Edition)
			}
			fmt.Fprintf(os.Stderr, "ERROR! %s is not a supported %s %s edition.\n - Supported editions: %s\n",
				edition, e.PrettyName, rel, strings.Join(eds, " "))
		}
		os.Exit(1)
	}
	if arch != "" {
		vmPath += "-" + arch
	}
	os.Exit(createVM(e, entry, vmPath))
}
