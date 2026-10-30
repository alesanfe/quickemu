// quickget-cli: the `quickget` entry point in Go — replaces
// bin/quickget + lib/quickget/cli/*. Parses the same flag surface and
// dispatches to the Go backend tools (quickget-catalog for catalog
// queries, quickget-vm for fetch/config). Zero Bash delegation.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/alesanfe/quickemu/tools/qelib"
)

var (
	arch = hostArch()
	// MINIMUM_TRUST env seeds the trust gate (Bash ${MINIMUM_TRUST:-E}
	// semantics: unset/invalid env is permissive, --minimum-trust wins
	// and is validated).
	minTrust          = strings.ToUpper(os.Getenv("MINIMUM_TRUST"))
	insecure          = false
	cloudInit         = false
	unattended        = false
	disableUnattended = false
	checkAllArc       = false
)

func actListCSV() int {
	fmt.Println("Display Name,OS,Release,Option,Downloader,PNG,SVG")
	for _, r := range csvRows(loadCatalog(), loadInfo()) {
		fmt.Println(strings.Join(r, ","))
	}
	return 0
}

func actList() int {
	// list_supported = list_csv cols 2,3,4 — the CSV header becomes
	// "OS Release Option".
	fmt.Println("OS Release Option")
	for _, r := range csvRows(loadCatalog(), loadInfo()) {
		fmt.Println(r[1], r[2], r[3])
	}
	return 0
}

func actListJSON() int {
	// list_json = list_csv through jq — object keys keep the CSV
	// column order (a Go map would marshal alphabetically).
	keys := []string{"Display Name", "OS", "Release", "Option", "Downloader", "PNG", "SVG"}
	var b strings.Builder
	b.WriteString("[\n")
	for i, r := range csvRows(loadCatalog(), loadInfo()) {
		if i > 0 {
			b.WriteString(",\n")
		}
		b.WriteString("  {")
		for j, k := range keys {
			if j > 0 {
				b.WriteString(", ")
			}
			kb, _ := json.Marshal(k)
			vb, _ := json.Marshal(r[j])
			b.WriteString(string(kb) + ": " + string(vb))
		}
		b.WriteString("}")
	}
	b.WriteString("\n]\n")
	fmt.Print(b.String())
	return 0
}

func actShow(osName string) int {
	info := loadInfo()
	inf, ok := info[osName]
	if !ok {
		fmt.Fprintf(os.Stdout, "ERROR! You must specify a valid OS.\n")
		return 1
	}
	fmt.Println()
	fmt.Println(inf.Name)
	fmt.Println(" - Credentials:\t" + inf.Credentials)
	fmt.Println(" - Website:\t" + inf.Homepage)
	fmt.Println(" - Description:\t" + inf.Info)
	return 0
}

// actUpdate: catalog_update_check parity — newest upstream release vs
// local <os>-* VM directories in the cwd.
func actUpdate(osName string) int {
	out, err := tool("quickget-catalog", "releases", osName)
	if err != nil || strings.TrimSpace(out) == "" {
		fmt.Fprintf(os.Stdout, "ERROR! You must specify a valid OS.\n")
		return 1
	}
	latest := strings.Fields(out)[0]
	fmt.Println("Latest " + osName + " release upstream: " + latest)
	dirs, _ := filepath.Glob(osName + "-*")
	var local []string
	for _, d := range dirs {
		if st, err := os.Stat(d); err == nil && st.IsDir() {
			local = append(local, d)
		}
	}
	slices.Sort(local)
	if len(local) == 0 {
		fmt.Println("No local " + osName + " VM directories found.")
		return 0
	}
	fmt.Println("Local VM dirs:")
	for _, d := range local {
		if d == osName+"-"+latest {
			fmt.Println("  " + d + "  (up to date)")
		} else {
			fmt.Println("  " + d + "  (older release available)")
		}
	}
	return 0
}

func actOpenHomepage(osName string) int {
	info := loadInfo()
	inf, ok := info[osName]
	if !ok || inf.Homepage == "" {
		// open_homepage parity: os_info miss → error_specify_os.
		errSpecifyOS()
		return 1
	}
	for _, opener := range []string{"xdg-open", "sensible-browser", "x-www-browser", "gnome-open"} {
		if bin, err := exec.LookPath(opener); err == nil {
			if err := execCommand(bin, inf.Homepage).Start(); err == nil {
				return 0
			}
		}
	}
	fmt.Println(inf.Homepage)
	return 0
}

func actGet(osName, release, edition string, noConf bool) int {
	// Bash order: os_supported → release required → validate_release →
	// resolve edition → trust gate → download.
	if !validateOS(osName) {
		return 1
	}
	if release == "" {
		return specifyRelease(osName)
	}
	e := findEntry(loadCatalog(), osName)
	// catalog_check_os_arch parity: OSes without editions gate before
	// release validation; download ops die, test/show SKIP.
	if e != nil && len(e.Editions) == 0 && !archSupport(e, arch) {
		fmt.Fprintf(os.Stdout, "ERROR! %s is not available for %s architecture.\n", prettyName(osName), arch)
		return 1
	}
	lvl := trustLevel(osName, release, edition, arch)
	if !trustOK(lvl, minTrust) {
		fmt.Fprintf(os.Stderr, "ERROR! %s source trust level %s is below --minimum-trust %s\n", osName, lvl, minTrust)
		return 1
	}
	// trust_warn parity: unverifiable sources warn on every download
	// (before release/edition validation, like main.sh). The catalog
	// trust probe already ran get_<os>-equivalent resolution, so a
	// checksum source whose hash fails to resolve reports D here.
	switch lvl {
	case "D":
		fmt.Fprintf(os.Stderr, "warning: %s source integrity not verifiable (trust D: https only)\n", osName)
	case "E":
		fmt.Fprintf(os.Stderr, "warning: %s source unverifiable (trust E: community mirror)\n", osName)
	}
	if !validateRelease(osName, release) {
		return 1
	}
	if !resolveEdition(osName, edition) {
		return 1
	}
	// catalog_resolve_edition parity: edition OSes gate arch after the
	// edition resolves; handle_missing rejects the Fedora pre-40
	// Onyx/Sericea combinations.
	if e != nil && len(e.Editions) > 0 {
		if !archSupport(e, arch) {
			fmt.Fprintf(os.Stdout, "ERROR! %s %s is not available for %s architecture.\n", prettyName(osName), edition, arch)
			return 1
		}
		if osName == "fedora" {
			nrel := strings.TrimSuffix(release, "_Beta")
			if n, err := strconv.Atoi(nrel); err == nil && n < 40 && (edition == "Onyx" || edition == "Sericea") {
				fmt.Fprintln(os.Stdout, "ERROR! Unsupported combination")
				fmt.Fprintf(os.Stdout, "       Fedora %s %s is not available, please choose another Release or Edition\n", release, edition)
				return 1
			}
		}
	}
	args := []string{"get", osName}
	if release != "" {
		args = append(args, release)
	}
	if edition != "" {
		args = append(args, edition)
	}
	if arch != "" && arch != "amd64" {
		args = append(args, "--arch", arch)
	}
	if cloudInit {
		args = append(args, "--cloud-init")
	}
	if noConf {
		args = append(args, "--no-conf")
	}
	if unattended {
		args = append(args, "--unattended")
	}
	return runPassthru("quickget-vm", args...)
}

// actCreateConfig: --create-config <vm-path> <input> [--flags] —
// create_config parity: build a VM dir+conf from a local image or URL.
func actCreateConfig(vmPath, input string) int {
	args := []string{"customconf", vmPath, input}
	if unattended {
		args = append(args, "--unattended")
	}
	if disableUnattended {
		args = append(args, "--disable-unattended")
	}
	return runPassthru("quickget-vm", args...)
}

var _ = globDir // retained for future use
func usage() {
	// help_message parity: ASCII banner + flag reference + the
	// supported-OS list wrapped at 80 columns.
	curlVer := ""
	if out, err := execCommand("curl", "--version").Output(); err == nil {
		if f := strings.Fields(strings.SplitN(string(out), "\n", 2)[0]); len(f) > 1 {
			curlVer = f[1]
		}
	}
	fmt.Printf(`
             _      _              _
  __ _ _   _(_) ___| | ____ _  ___| |_
 / _`+"`"+` | | | | |/ __| |/ / _`+"`"+` |/ _ \ __|
| (_| | |_| | | (__|   < (_| |  __/ |_
 \__, |\__,_|_|\___|_|\_\__, |\___|\__|
    |_|                 |___/ v%s, using curl %s
--------------------------------------------------------------------------------
 Project - https://github.com/quickemu-project/quickemu
 Discord - https://wimpysworld.io/discord
--------------------------------------------------------------------------------

Usage:
  quickget <os> <release> [edition]
  quickget ubuntu 22.04

Advanced usage:
  quickget <arg> [path] <os> [release] [edition]
  quickget --download ubuntu 22.04

Arguments:
   --arch           <arch>                    : Set architecture (arm64, aarch64, amd64, x86_64)
   --download       <os> <release> [edition]  : Download image; no VM configuration
   --create-config  <os> [path/url] [flags]   : Create VM config for an OS image
   --insecure                                 : Skip SSL certificate verification (or set QUICKGET_INSECURE=1)
   --cloud-init                               : Generate a NoCloud seed.iso for the downloaded VM
   --unattended                               : Configure unattended install (ubuntu-server)
   --update        <os>                        : Show newest upstream release vs local VMs
   --open-homepage  <os>                      : Open homepage for the OS
   --show           [os]                      : Show OS information
   --version                                  : Show version
   --help                                     : Show this help message
------------------------------------ Flags -------------------------------------
--create-config:
  --disable-unattended                        : Force quickget not to set up an unattended installation
-------------------------- For testing & development ---------------------------
   --url            [os] [release] [edition]  : Show image URL(s)
   --check          [os] [release] [edition]  : Check image URL(s)
   --check-all-arch [os] [release] [edition]  : Check downloads for all architectures (amd64 and arm64)
   --list                                     : List all supported systems
   --list-csv                                 : List everything in csv format
   --list-json                                : List everything in json format
--------------------------------------------------------------------------------

Supported Operating Systems:

`, qelib.QEVersion, curlVer)
	var names []string
	for _, e := range loadCatalog() {
		names = append(names, e.OS)
	}
	fmt.Println(wrapCols(strings.Join(names, " "), 80))
}

func main() {
	args := os.Args[1:]
	var op string
	var pos []string
	i := 0
	// --arch may precede the operation (Bash parses it first).
	for i < len(args) {
		a := args[i]
		next := func() string {
			if i+1 < len(args) {
				i++
				return args[i]
			}
			return ""
		}
		switch a {
		case "--arch", "-arch":
			switch next() {
			case "arm64", "aarch64":
				arch = "arm64"
			case "amd64", "x86_64":
				arch = "amd64"
			default:
				fmt.Fprintln(os.Stdout, "ERROR! Unsupported architecture")
				os.Exit(1)
			}
		case "--minimum-trust", "-minimum-trust":
			minTrust = strings.ToUpper(next())
			switch minTrust {
			case "A", "B", "C", "D", "E":
			default:
				fmt.Fprintln(os.Stderr, "ERROR! invalid trust level '"+minTrust+"' (A-E)")
				os.Exit(1)
			}
		case "--insecure", "-insecure":
			insecure = true
		case "--cloud-init", "-cloud-init":
			cloudInit = true
		case "--unattended", "-unattended":
			unattended = true
		case "--disable-unattended", "-disable-unattended":
			disableUnattended = true
		case "--download", "-download":
			op = "download"
		case "--create-config", "-create-config":
			op = "config"
		case "--url", "-url":
			op = "url"
		case "--check", "-check":
			op = "check"
		case "--check-all-arch", "-check-all-arch":
			op = "check"
			checkAllArc = true
		case "--list", "-list":
			os.Exit(actList())
		case "--list-csv", "-list-csv", "list", "list_csv":
			os.Exit(actListCSV())
		case "--list-json", "-list-json", "list_json":
			os.Exit(actListJSON())
		case "--show", "-show":
			op = "show"
		case "--open-homepage", "-open-homepage":
			op = "homepage"
		case "--update", "-update":
			op = "update"
		case "--chunkcheck", "-chunkcheck", "chunkcheck":
			op = "chunkcheck"
		case "--version", "-version":
			fmt.Println(qelib.QEVersion)
			return
		case "--help", "-help", "--h", "-h":
			usage()
			return
		default:
			if strings.HasPrefix(a, "-") {
				// error_not_supported_argument: no arg echo, help hint.
				fmt.Fprintln(os.Stdout, "ERROR! Not supported argument")
				fmt.Fprintln(os.Stdout, "To see all possible arguments, use:")
				fmt.Fprintln(os.Stdout, "   quickget -h  or  quickget --help")
				os.Exit(1)
			}
			pos = append(pos, a)
		}
		i++
	}
	_ = insecure // consumed by quickget-vm via env when needed
	if insecure {
		os.Setenv("QUICKGET_INSECURE", "1")
	}
	var osName, release, edition string
	if len(pos) > 0 {
		osName = strings.ToLower(pos[0])
	}
	if len(pos) > 1 {
		release = pos[1]
	}
	if len(pos) > 2 {
		edition = strings.Join(pos[2:], " ")
	}
	// os_supported gate (main.sh:194): any positional OS is validated
	// before dispatch. --create-config positionals are vm-path+input
	// and chunkcheck's is a vmdir — neither is an OS, so exempt.
	if osName != "" && op != "config" && op != "homepage" && op != "chunkcheck" && !validateOS(osName) {
		os.Exit(1)
	}
	rc := 0
	switch op {
	case "show":
		if osName == "" {
			for _, e := range loadCatalog() {
				actShow(e.OS)
			}
			return
		}
		rc = actShow(osName)
	case "homepage":
		if osName == "" {
			fmt.Fprintln(os.Stdout, "ERROR! You must specify a valid OS.")
			os.Exit(1)
		}
		rc = actOpenHomepage(osName)
	case "url":
		if osName == "" {
			for _, e := range loadCatalog() {
				actURL(e.OS, "", "", arch)
			}
			return
		}
		rc = actURL(osName, release, edition, arch)
	case "check":
		archs := []string{arch}
		if checkAllArc {
			archs = []string{"amd64", "arm64"}
		}
		if osName == "" {
			for _, a := range archs {
				for _, e := range loadCatalog() {
					actCheck(e.OS, "", "", a)
				}
			}
			return
		}
		for _, a := range archs {
			if r := actCheck(osName, release, edition, a); r != 0 {
				rc = r
			}
		}
	case "download":
		if osName == "" || release == "" {
			fmt.Fprintln(os.Stdout, "ERROR! You must specify an OS and release.")
			os.Exit(1)
		}
		rc = actGet(osName, release, edition, true)
	case "config":
		// create_config <vm-path> <input>: positional args are the VM
		// dir name and the image path/URL.
		if osName == "" || release == "" {
			fmt.Fprintln(os.Stdout, "ERROR! --create-config needs <vm-path> <input>.")
			os.Exit(1)
		}
		rc = actCreateConfig(osName, release)
	case "update":
		rc = actUpdate(osName)
	case "chunkcheck":
		// Public surface for the libexec chunkcheck.py contract:
		// `quickget chunkcheck <vmdir>` delegates to quickget-vm.
		if len(pos) == 0 {
			fmt.Fprintln(os.Stdout, "ERROR! chunkcheck needs a <vmdir>.")
			os.Exit(1)
		}
		rc = runPassthru("quickget-vm", append([]string{"chunkcheck"}, pos...)...)
	default:
		// Positional default: download + create config (create_vm parity).
		if osName == "" {
			errSpecifyOS()
			os.Exit(1)
		}
		rc = actGet(osName, release, edition, false)
	}
	os.Exit(rc)
}
