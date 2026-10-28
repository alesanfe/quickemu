// catalog.go: the quickget_data.json model and the jq-equivalent
// selection helpers (find_os_entry/find_release_entries/
// find_edition_entry/list_*).
package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"slices"
	"strings"
)

// Source / Release / OSEntry mirror the upstream quickget_configs
// dataset schema (quickget_data.json).
type Source struct {
	Web *struct {
		URL           string `json:"url"`
		Checksum      string `json:"checksum"`
		ArchiveFormat string `json:"archive_format"`
		FileName      string `json:"file_name"`
	} `json:"web"`
	FileName string `json:"file_name"`
	Custom   string `json:"custom"`
}

type Release struct {
	Release  string   `json:"release"`
	Edition  string   `json:"edition"`
	Arch     string   `json:"arch"`
	GuestOS  string   `json:"guest_os"`
	RAM      int64    `json:"ram"`
	ISO      []Source `json:"iso"`
	IMG      []Source `json:"img"`
	FixedISO []Source `json:"fixed_iso"`
	Floppy   []Source `json:"floppy"`
}

type OSEntry struct {
	Name        string    `json:"name"`
	PrettyName  string    `json:"pretty_name"`
	Description string    `json:"description"`
	Homepage    string    `json:"homepage"`
	GuestOS     string    `json:"guest_os"`
	Releases    []Release `json:"releases"`
}

var imageTypes = []string{"iso", "img", "fixed_iso", "floppy"}

// sourcesFor returns the source slice for an image type key.
func (r *Release) sourcesFor(t string) []Source {
	switch t {
	case "iso":
		return r.ISO
	case "img":
		return r.IMG
	case "fixed_iso":
		return r.FixedISO
	case "floppy":
		return r.Floppy
	}
	return nil
}

// findOSEntry: name match + release/arch defaults (jq
// `.releases[] |= (. + {arch:(.arch//"x86_64"), release:(.release//"latest")})`).
func findOSEntry(cat []OSEntry, name string) *OSEntry {
	for i := range cat {
		if cat[i].Name == name {
			for j := range cat[i].Releases {
				if cat[i].Releases[j].Arch == "" {
					cat[i].Releases[j].Arch = "x86_64"
				}
				if cat[i].Releases[j].Release == "" {
					cat[i].Releases[j].Release = "latest"
				}
			}
			return &cat[i]
		}
	}
	return nil
}

// findReleaseEntries: all entries for a release.
func findReleaseEntries(e *OSEntry, rel string) []Release {
	var out []Release
	for _, r := range e.Releases {
		if r.Release == rel {
			out = append(out, r)
		}
	}
	return out
}

// releaseRequiresEdition: every entry in the release has a non-null
// edition (jq any(.edition==null)==false).
func releaseRequiresEdition(entries []Release) bool {
	for _, r := range entries {
		if r.Edition == "" {
			return false
		}
	}
	return true
}

// findEditionEntry: edition match (empty → null-edition entries),
// preferring the requested arch, then host arch, else first entry.
func findEditionEntry(entries []Release, edition, arch, hostArch string) (*Release, []Release, error) {
	var edEntries []Release
	for _, r := range entries {
		if r.Edition == edition {
			edEntries = append(edEntries, r)
		}
	}
	if len(edEntries) == 0 {
		return nil, edEntries, fmt.Errorf("edition not found")
	}
	pref := arch
	if pref == "" {
		pref = hostArch
	}
	for i := range edEntries {
		if edEntries[i].Arch == pref {
			return &edEntries[i], edEntries, nil
		}
	}
	if arch != "" {
		return nil, edEntries, fmt.Errorf("arch not supported")
	}
	return &edEntries[0], edEntries, nil
}

// listReleasesRaw: unique releases preserving order.
func listReleasesRaw(e *OSEntry) []string {
	seen := map[string]bool{}
	var out []string
	for _, r := range e.Releases {
		if !seen[r.Release] {
			seen[r.Release] = true
			out = append(out, r.Release)
		}
	}
	return out
}

// listEditions: unique editions for a release.
func listEditions(e *OSEntry, rel string) []string {
	seen := map[string]bool{}
	var out []string
	for _, r := range e.Releases {
		if r.Release == rel && !seen[r.Edition] {
			seen[r.Edition] = true
			out = append(out, r.Edition)
		}
	}
	return out
}

// listReleasesDisplay: the - Releases / - Editions block from
// error_specify_release and --show output.
func listReleasesDisplay(e *OSEntry) {
	rels := listReleasesRaw(e)
	eds := map[string][]string{}
	uniqueEd := false
	hasEd := false
	var first []string
	for i, r := range rels {
		eds[r] = listEditions(e, r)
		if i == 0 {
			first = eds[r]
		}
		if !equalStrs(eds[r], first) {
			uniqueEd = true
		}
		if len(eds[r]) > 0 && eds[r][0] != "" {
			hasEd = true
		}
	}
	if uniqueEd {
		fmt.Printf(" - %-15s | %s\n", "Releases", "Editions")
		for _, r := range rels {
			fmt.Printf("  %-16s | %s\n", r, strings.Join(eds[r], " "))
		}
		return
	}
	fmt.Println(" - Releases: " + strings.Join(rels, " "))
	if hasEd {
		fmt.Println(" - Editions: " + strings.Join(first, " "))
	}
}

func equalStrs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// listCSV: Display Name,OS,Release,Option,Downloader,PNG,SVG rows.
// encoding/csv quoting matches jq @csv (fields with commas/quotes get
// wrapped) — plain Printf would corrupt rows.
func listCSV(cat []OSEntry) {
	w := csv.NewWriter(os.Stdout)
	w.Write([]string{"Display Name", "OS", "Release", "Option", "Downloader", "PNG", "SVG"})
	for _, e := range cat {
		for _, r := range e.Releases {
			w.Write([]string{
				e.PrettyName, e.Name, r.Release, r.Edition, "",
				"https://quickemu-project.github.io/quickemu-icons/png/" + e.Name + "/" + e.Name + "-quickemu-white-pinkbg.png",
				"https://quickemu-project.github.io/quickemu-icons/svg/" + e.Name + "/" + e.Name + "-quickemu-white-pinkbg.svg",
			})
		}
	}
	w.Flush()
}

// allSourceURLs: unique urls anywhere inside a release entry (the
// jq `[.. | objects | .url? // empty]` walk).
func allSourceURLs(r Release) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range imageTypes {
		for _, s := range r.sourcesFor(t) {
			if s.Web != nil && s.Web.URL != "" && !seen[s.Web.URL] {
				seen[s.Web.URL] = true
				out = append(out, s.Web.URL)
			}
		}
	}
	slices.Sort(out)
	return out
}
