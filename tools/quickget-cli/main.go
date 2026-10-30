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
	"runtime"
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

// windows language tables (languages_windows / languages_windows-server
// from lib/quickget/catalog/data.sh).
var winLangs = []string{
	"Arabic", "Brazilian Portuguese", "Bulgarian", "Chinese (Simplified)",
	"Chinese (Traditional)", "Croatian", "Czech", "Danish", "Dutch",
	"English (United States)", "English International", "Estonian",
	"Finnish", "French", "French Canadian", "German", "Greek", "Hebrew",
	"Hungarian", "Italian", "Japanese", "Korean", "Latvian", "Lithuanian",
	"Norwegian", "Polish", "Portuguese", "Romanian", "Russian",
	"Serbian Latin", "Slovak", "Slovenian", "Spanish", "Spanish (Mexico)",
	"Swedish", "Thai", "Turkish", "Ukrainian",
}
var winSrvLangs = []string{
	"English (United States)", "Chinese (Simplified)", "French", "German",
	"Italian", "Japanese", "Russian", "Spanish",
}

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

// combinations returns release—edition pairs to iterate for --url/--check
// (test_all parity: no release → all releases; editions expanded when the
// OS has them and none was given; windows → language list).
func combinations(e *Entry, release, edition string) [][2]string {
	rels := e.Releases
	if release != "" {
		rels = []string{release}
	}
	editionsFor := func(rel string) []string {
		switch {
		case edition != "":
			return []string{edition}
		case len(e.Editions) > 0:
			return e.Editions
		}
		// Editions that depend on the release (editions_<os>() uses
		// ${RELEASE}, e.g. blackarch) come from a live catalog scrape.
		if out, err := tool("quickget-catalog", "editions", e.OS, rel); err == nil && strings.TrimSpace(out) != "" {
			return strings.Fields(out)
		}
		switch e.OS {
		case "windows":
			return winLangs
		case "windows-server":
			return winSrvLangs
		default:
			return []string{""}
		}
	}
	var out [][2]string
	for _, r := range rels {
		for _, ed := range editionsFor(r) {
			out = append(out, [2]string{r, ed})
		}
	}
	return out
}

func actURL(osName, release, edition, arch string) int {
	cat := loadCatalog()
	e := findEntry(cat, osName)
	if e == nil {
		fmt.Fprintf(os.Stdout, "ERROR! You must specify a valid OS.\n")
		return 1
	}
	rc := 0
	// With a release arg, Bash's bottom flow applies: arch-skip for
	// non-edition OSes, trust gate + warn, edition required, release
	// validated. test_all (no release) iterates freely.
	if release != "" {
		if rc := releaseScopedPrecheck(osName, release, edition, arch, e); rc >= 0 {
			return rc
		}
	}
	for _, c := range combinations(e, release, edition) {
		out, err := tool("quickget-catalog", "url", osName, c[0], c[1], arch)
		if err != nil {
			fmt.Fprint(os.Stderr, out+"\n")
			rc = 1
			continue
		}
		label := osName + "-" + c[0]
		if c[1] != "" {
			label += "-" + c[1]
		}
		fmt.Printf("%-36s %s\n", label+":", strings.Fields(out)[0])
	}
	return rc
}

// releaseScopedPrecheck runs the pre-resolution gates Bash applies in
// its bottom flow for ops with an explicit release (test/show/download).
// Returns an exit code when the op should terminate, -1 to continue.
func releaseScopedPrecheck(osName, release, edition, arch string, e *Entry) int {
	if len(e.Editions) == 0 && !archSupport(e, arch) {
		// catalog_check_os_arch → test/show ops SKIP instead of dying.
		label := osName + "-" + release
		fmt.Printf("%s: %-35s (not available for %s)\n", "SKIP", label, arch)
		return 0
	}
	lvl := trustLevel(osName, release, edition, arch)
	if !trustOK(lvl, minTrust) {
		fmt.Fprintf(os.Stderr, "ERROR! %s source trust level %s is below --minimum-trust %s\n", osName, lvl, minTrust)
		return 1
	}
	switch lvl {
	case "D":
		fmt.Fprintf(os.Stderr, "warning: %s source integrity not verifiable (trust D: https only)\n", osName)
	case "E":
		fmt.Fprintf(os.Stderr, "warning: %s source unverifiable (trust E: community mirror)\n", osName)
	}
	if !validateRelease(osName, release) {
		return 1
	}
	if len(e.Editions) > 0 && edition == "" {
		specifyReleaseInfo(osName)
		if out, err := tool("quickget-catalog", "editions", osName); err == nil {
			fmt.Println(" - Editions:\t" + out)
		}
		fmt.Fprintln(os.Stdout, "\nERROR! You must specify an edition.")
		return 1
	}
	return -1
}

func actCheck(osName, release, edition, arch string) int {
	cat := loadCatalog()
	e := findEntry(cat, osName)
	if e == nil {
		fmt.Fprintf(os.Stdout, "ERROR! You must specify a valid OS.\n")
		return 1
	}
	rc := 0
	combos := combinations(e, release, edition)
	if release != "" && checkAllArc {
		// catalog_check_all_arch: validates the release, but runs no
		// trust gate and resolves exactly the release/edition given.
		if !validateRelease(osName, release) {
			return 1
		}
		combos = [][2]string{{release, edition}}
	} else if release != "" {
		if rc := releaseScopedPrecheck(osName, release, edition, arch, e); rc >= 0 {
			return rc
		}
	}
	for _, c := range combos {
		out, err := tool("quickget-catalog", "check", osName, c[0], c[1], arch)
		fmt.Println(out)
		if err != nil || strings.HasPrefix(out, "FAIL") {
			rc = 1
		}
	}
	return rc
}

// trustLevel maps catalog trust output A-E. Release/edition/arch feed
// the live resolution probe (trust_os ran get_<os> with the globals
// already set).
func trustLevel(osName, release, edition, arch string) string {
	out, err := tool("quickget-catalog", "trust", osName, release, edition, arch)
	if err != nil || out == "" {
		return "E"
	}
	return out
}

// archSupport: is_arch_supported parity — Entry.Arch empty means the
// Bash default (amd64 only).
func archSupport(e *Entry, arch string) bool {
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

func prettyName(osName string) string {
	if inf, ok := loadInfo()[osName]; ok {
		return inf.Name
	}
	return osName
}

func trustOK(level, min string) bool {
	if min == "" {
		return true
	}
	return level <= min // A<B<C<D<E lexicographically
}

// actGet is the default/download pipeline: quickget-vm get (or --no-conf
// for --download). VirtIO/unattended/seed extras run inside quickget-vm.
// validateOS: os_supported parity — exits the error contract
// ("ERROR! <os> is not a supported OS." + wrapped list) and returns
// false for unknown OS names.
func validateOS(osName string) bool {
	if findEntry(loadCatalog(), osName) != nil {
		return true
	}
	fmt.Fprintf(os.Stdout, "ERROR! %s is not a supported OS.\n\n", osName)
	var names []string
	for _, e := range loadCatalog() {
		names = append(names, e.OS)
	}
	fmt.Fprintln(os.Stdout, wrapLines(names, 80))
	return false
}

// specifyRelease: error_specify_release parity — OS info + releases +
// editions/archs then "ERROR! You must specify a release."
func specifyRelease(osName string) int {
	info := loadInfo()
	if inf, ok := info[osName]; ok {
		fmt.Println()
		fmt.Println(inf.Name)
		fmt.Println(" - Credentials:\t" + inf.Credentials)
		fmt.Println(" - Website:\t" + inf.Homepage)
		fmt.Println(" - Description:\t" + inf.Info)
	}
	if out, err := tool("quickget-catalog", "releases", osName); err == nil {
		fmt.Println(" - Releases:\t" + out)
	}
	if out, err := tool("quickget-catalog", "editions", osName); err == nil && strings.TrimSpace(out) != "" {
		fmt.Println(" - Editions:\t" + out)
	}
	if out, err := tool("quickget-catalog", "arch", osName); err == nil && strings.TrimSpace(out) != "" {
		fmt.Println(" - Archs:\t" + out)
	}
	fmt.Fprintln(os.Stdout, "\nERROR! You must specify a release.")
	return 1
}

// validateRelease: validate_release parity — the chosen release must
// appear in the catalog releases list.
func validateRelease(osName, release string) bool {
	out, err := tool("quickget-catalog", "releases", osName)
	if err != nil {
		return true // catalog can't answer — let the download path decide
	}
	for _, r := range strings.Fields(out) {
		if r == release {
			return true
		}
	}
	name := osName
	if inf, ok := loadInfo()[osName]; ok {
		name = inf.Name
	}
	fmt.Fprintf(os.Stdout, "ERROR! %s %s is not a supported release.\n\n - Supported releases: %s\n", name, release, out)
	return false
}

// resolveEdition: catalog_resolve_edition parity — OSes with an
// editions list require a valid edition argument.
func resolveEdition(osName, edition string) bool {
	out, err := tool("quickget-catalog", "editions", osName)
	if err != nil || strings.TrimSpace(out) == "" {
		return true // no editions for this OS
	}
	eds := strings.Fields(out)
	name := osName
	if inf, ok := loadInfo()[osName]; ok {
		name = inf.Name
	}
	if edition == "" {
		specifyReleaseInfo(osName)
		fmt.Println(" - Editions:\t" + out)
		fmt.Fprintln(os.Stdout, "\nERROR! You must specify an edition.")
		return false
	}
	for _, e := range eds {
		if e == edition {
			return true
		}
	}
	fmt.Fprintf(os.Stdout, "ERROR! %s is not a supported %s edition\n\n - Supported editions: %s\n", edition, name, out)
	return false
}

func specifyReleaseInfo(osName string) {
	info := loadInfo()
	if inf, ok := info[osName]; ok {
		fmt.Println()
		fmt.Println(inf.Name)
		fmt.Println(" - Credentials:\t" + inf.Credentials)
		fmt.Println(" - Website:\t" + inf.Homepage)
		fmt.Println(" - Description:\t" + inf.Info)
	}
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
