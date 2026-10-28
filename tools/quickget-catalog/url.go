// URL resolution for quickget-catalog: implements the get_<os> contract
// generically. Template-based OSes resolve purely in Go (URL + ISO + live
// checksum fetch); scraper OSes resolve live via scrape().
package main

import (
	"fmt"
	"github.com/alesanfe/quickemu/tools/qelib"
	"regexp"
	"strings"
)

// fetchString GETs a URL and returns the body (bounded). Mirrors web_pipe.
// A var so tests can serve fixture pages without network.
var fetchString = func(u string) (string, error) { return qelib.FetchString(u, 4<<20) }

// finalURL resolves the last redirect target (web_redirect parity).
var finalURL = func(u string) (string, error) { return qelib.FinalURL(u) }

// findLine greps the body for lines matching re and returns the first,
// extracting group `field` (1-indexed) or the whole match when field==0.
func findLine(body, re string, field int) string {
	r := regexp.MustCompile(re)
	for _, line := range strings.Split(body, "\n") {
		if m := r.FindStringSubmatch(line); m != nil {
			if field > 0 && len(m) > field {
				return m[field]
			}
			return strings.TrimSpace(m[0])
		}
	}
	return ""
}

// resolveURL produces "URL HASH" the way get_<os> does. Templates use
// %RELEASE%/%EDITION%/%ARCH% placeholders captured at dump time; hash_src
// is the checksum-file URL pattern.
func resolveURL(e *Entry, release, edition, arch string) (string, string) {
	if e.URL == "" {
		return "", ""
	}
	archTok := arch
	if e.QArch {
		// ${QEMU_ARCH} parity: amd64→x86_64, arm64→aarch64.
		switch arch {
		case "amd64":
			archTok = "x86_64"
		case "arm64":
			archTok = "aarch64"
		}
	}
	sub := func(s string) string {
		s = strings.ReplaceAll(s, "%RELEASE%", release)
		s = strings.ReplaceAll(s, "%EDITION%", edition)
		s = strings.ReplaceAll(s, "%ARCH%", archTok)
		return s
	}
	base := sub(e.URL)
	iso := ""
	if e.ISO != "" {
		iso = sub(e.ISO)
	}
	hash := ""
	if e.HashURL != "" {
		// %ISO% expands to the resolved ISO filename; ${URL}/${BASE} to
		// the resolved base (hash_url templates dumped with raw vars).
		src := strings.ReplaceAll(sub(e.HashURL), "%ISO%", iso)
		src = expandVars(src, base, iso, release, edition)
		if body, err := fetchString(src); err == nil {
			re := e.HashRE
			if re == "" {
				re = `(?i)` + regexp.QuoteMeta(iso) + `.*?([a-f0-9]{64})`
			}
			hash = findLine(body, re, 1)
		}
	}
	full := base
	if iso != "" {
		iso = expandVars(iso, base, iso, release, edition)
	}
	if iso != "" && !strings.HasSuffix(base, "/") {
		full = base + "/" + iso
	} else if iso != "" {
		full = base + iso
	}
	return full, hash
}

// expandVars resolves the residual ${VAR}/parameter-expansion tokens
// left in dumped templates that map to already-known values (Bash
// forms supported: ${RELEASE#prefix}, ${RELEASE:off:len},
// ${RELEASE//pat/}, ${ISO//old/new}, ${ED_UPPER}, ${VER_MAJ},
// ${VER_MIN}, ${edition}, ${ED}, ${URL}, ${BASE}, ${ISONAME}, ${ISO},
// ${README}).
func expandVars(s, base, iso, release, edition string) string {
	if !strings.Contains(s, "${") {
		return s
	}
	re := regexp.MustCompile(`\$\{([^}]+)\}`)
	return re.ReplaceAllStringFunc(s, func(tok string) string {
		inner := re.FindStringSubmatch(tok)[1]
		switch inner {
		case "URL", "BASE", "URLBASE":
			return base
		case "ISO", "ISONAME":
			return iso
		case "edition", "ED":
			return edition
		case "ED_UPPER":
			return strings.ToUpper(edition)
		case "README":
			return "README"
		case "VER_MAJ":
			if f := strings.SplitN(release, ".", 2); len(f) > 0 {
				return f[0]
			}
		case "VER_MIN":
			f := strings.SplitN(release, ".", 3)
			if len(f) > 1 {
				return f[1]
			}
		}
		// ${RELEASE#prefix} — strip prefix
		if strings.HasPrefix(inner, "RELEASE#") {
			return strings.TrimPrefix(release, inner[8:])
		}
		// ${RELEASE:off:len} — substring
		if strings.HasPrefix(inner, "RELEASE:") {
			parts := strings.Split(inner[8:], ":")
			off := 0
			if len(parts) > 0 {
				fmt.Sscanf(parts[0], "%d", &off)
			}
			if off < len(release) {
				if len(parts) > 1 {
					l := 0
					if _, err := fmt.Sscanf(parts[1], "%d", &l); err == nil && l >= 0 {
						if off+l <= len(release) {
							return release[off : off+l]
						}
						return release[off:]
					}
				}
				return release[off:]
			}
			return ""
		}
		// ${NAME//old/new} — replace-all (\ escapes are literal chars)
		if i := strings.Index(inner, "//"); i > 0 {
			name, rest := inner[:i], inner[i+2:]
			old, new_ := rest, ""
			if j := strings.Index(rest, "/"); j >= 0 {
				old, new_ = rest[:j], rest[j+1:]
			}
			old = strings.ReplaceAll(old, `\`, "")
			src := release
			if name == "ISO" {
				src = iso
			}
			return strings.ReplaceAll(src, old, new_)
		}
		return tok
	})
}
