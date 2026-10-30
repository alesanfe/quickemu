// tools/quickget-cli/validate.go: os/release/edition validation, trust gating, --check/--url actions
package main

import (
	"fmt"
	"os"
	"strings"
)

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
			// get_* die() text is a stdout contract, not a diagnostic.
			fmt.Fprint(os.Stdout, out+"\n")
			rc = 1
			continue
		}
		label := osName + "-" + c[0]
		if c[1] != "" {
			label += "-" + c[1]
		}
		u := ""
		if f := strings.Fields(out); len(f) > 0 {
			u = f[0]
		}
		fmt.Printf("%-36s %s\n", label+":", u)
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
		// catalog_check_all_arch windows branch: for a supported arch
		// the SKIP carries the "(Windows not supported in check mode)"
		// reason; unsupported arches keep the catalog's own SKIP.
		if checkAllArc && release != "" &&
			(osName == "windows" || osName == "windows-server") && archSupport(e, arch) {
			label := osName + "-" + c[0]
			if c[1] != "" {
				label += "-" + c[1]
			}
			fmt.Printf("SKIP: %-35s (Windows not supported in check mode)\n", label)
			continue
		}
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
