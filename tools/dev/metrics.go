// tools/dev/metrics.go: metrics record/trend/chaoss/structure + helpers (history.jsonl series)
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

func git(args ...string) string {
	out, err := execCommand("git", args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func metricChaoss() int {
	commits30 := lines(git("log", "--since=30 days ago", "--oneline"))
	commits90 := lines(git("log", "--since=90 days ago", "--oneline"))
	authors := uniq(git("log", "--since=90 days ago", "--format=%an"))
	topPct := 0
	if commits90 > 0 {
		counts := map[string]int{}
		for _, a := range strings.Split(git("log", "--since=90 days ago", "--format=%an"), "\n") {
			if a != "" {
				counts[a]++
			}
		}
		top := 0
		for _, c := range counts {
			if c > top {
				top = c
			}
		}
		topPct = top * 100 / commits90
	}
	last := git("log", "-1", "--format=%ci")
	if i := strings.IndexByte(last, ' '); i > 0 {
		last = last[:i]
	}
	out := map[string]any{
		"tool":      "qedev metrics chaoss",
		"timestamp": time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		"metrics": map[string]any{
			"commits_30d":          commits30,
			"commits_90d":          commits90,
			"active_authors_90d":   authors,
			"top_author_share_pct": topPct,
			"last_commit":          last,
			"release_tags":         lines(git("tag")),
		},
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	fmt.Println(string(b))
	return 0
}

func metricRecord(kvs []string) int {
	r := root()
	rec := map[string]string{
		"timestamp": time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		"commit":    or(git("rev-parse", "--short", "HEAD"), "unknown"),
		"branch":    or(git("rev-parse", "--abbrev-ref", "HEAD"), "unknown"),
	}
	rec["spec_files"] = strconv.Itoa(countSpecs(r))
	rec["catalog_os"] = strconv.Itoa(countCatalogOS(r))
	rec["coverage_go"] = goCoverage(r)
	for _, kv := range kvs {
		if i := strings.IndexByte(kv, '='); i > 0 {
			rec[kv[:i]] = kv[i+1:]
		}
	}
	// QEDEV_HISTORY overrides the series path — tests must not append
	// fake records (stubbed git data) to the real history.jsonl.
	out := filepath.Join(r, "docs", "quality", "history.jsonl")
	if p := os.Getenv("QEDEV_HISTORY"); p != "" {
		out = p
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return 1
	}
	f, err := os.OpenFile(out, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return 1
	}
	defer f.Close()
	b, _ := json.Marshal(rec)
	f.Write(append(b, '\n'))
	fmt.Println(string(b))
	return 0
}

func countSpecs(r string) int {
	n := 0
	filepath.WalkDir(r, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() &&
			(strings.HasSuffix(p, "_spec.sh") || strings.HasSuffix(p, "_test.go")) &&
			!strings.Contains(p, string(filepath.Separator)+"vendor"+string(filepath.Separator)) &&
			!strings.Contains(p, "shellspec-") {
			n++
		}
		return nil
	})
	return n
}

func countCatalogOS(r string) int {
	out, err := execCommand(filepath.Join(r, "bin", "quickget"), "--list").Output()
	if err != nil {
		return 0
	}
	set := map[string]bool{}
	for _, w := range strings.Fields(string(out)) {
		set[w] = true
	}
	return len(set)
}

// goCoverage averages `go test -cover` percentages across modules.
func goCoverage(r string) string {
	var sum, n float64
	for _, d := range modules(r) {
		cmd := execCommand(goTool(), "test", "-cover", "./...")
		cmd.Dir = d
		out, _ := cmd.Output()
		for _, l := range strings.Split(string(out), "\n") {
			if i := strings.Index(l, "coverage: "); i >= 0 {
				s := strings.TrimSuffix(strings.TrimSpace(l[i+10:]), "% of statements")
				if v, err := strconv.ParseFloat(s, 64); err == nil {
					sum, n = sum+v, n+1
				}
			}
		}
	}
	if n == 0 {
		return "0%"
	}
	return fmt.Sprintf("%.0f%%", sum/n)
}

// metricStructure walks the tree and reports structural counts:
// prod/test LOC, test funcs, exec sites, seams, deps, oversized
// files, and modules missing from go.work. Read-only; reproducible.
func metricStructure() int {
	r := root()
	sep := string(filepath.Separator)
	skip := func(rel string) bool {
		return rel == "vendor" || strings.HasPrefix(rel, "vendor"+sep) ||
			strings.Contains(rel, sep+".git"+sep) ||
			strings.Contains(rel, sep+"node_modules"+sep)
	}
	locP, locT, tests, execCalls, seamVars := 0, 0, 0, 0, 0
	deps := map[string]int{}
	big := []string{}
	mods := []string{}
	filepath.WalkDir(r, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(r, p)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if d.Name() == "vendor" || d.Name() == ".git" || d.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if skip(rel) {
			return nil
		}
		switch {
		case d.Name() == "go.mod":
			mods = append(mods, filepath.Dir(rel))
			b, e := os.ReadFile(p)
			if e == nil {
				inReq := false
				for _, l := range strings.Split(string(b), "\n") {
					l = strings.TrimSpace(l)
					if l == "require (" {
						inReq = true
						continue
					}
					if l == ")" {
						inReq = false
					}
					if strings.HasPrefix(l, "require ") && !strings.Contains(l, "(") {
						deps[strings.Fields(l)[1]]++
					} else if inReq && l != "" {
						deps[strings.Fields(l)[0]]++
					}
				}
			}
		case strings.HasSuffix(d.Name(), ".go"):
			b, e := os.ReadFile(p)
			if e != nil {
				return nil
			}
			n := bytes.Count(b, []byte("\n")) + 1
			if strings.HasSuffix(d.Name(), "_test.go") {
				locT += n
				tests += bytes.Count(b, []byte("\nfunc Test"))
			} else {
				locP += n
				execCalls += bytes.Count(b, []byte("exec.Command("))
				seamVars += bytes.Count(b, []byte("var execCommand"))
				if n > 500 {
					big = append(big, fmt.Sprintf("%s:%d", rel, n))
				}
			}
		}
		return nil
	})
	// Workspace drift: module dirs not listed in go.work's use blocks.
	missing := []string{}
	if wb, err := os.ReadFile(filepath.Join(r, "go.work")); err == nil {
		for _, m := range mods {
			if m != "." && !strings.Contains(string(wb), "./"+filepath.ToSlash(m)) {
				missing = append(missing, filepath.ToSlash(m))
			}
		}
	}
	ratio := 0.0
	if locP > 0 {
		ratio = float64(locT) / float64(locP)
	}
	// Advisory thresholds — warn but don't fail (per backlog: not
	// blocking until the series matures).
	warns := []string{}
	if ratio < 0.5 {
		warns = append(warns, fmt.Sprintf("test/product ratio %.2f below 0.50", ratio))
	}
	if execCalls > 10 {
		warns = append(warns, fmt.Sprintf("%d direct exec.Command sites (seam coverage should keep this low)", execCalls))
	}
	if len(missing) > 0 {
		warns = append(warns, fmt.Sprintf("%d modules outside go.work", len(missing)))
	}
	for _, f := range big {
		if n, _ := strconv.Atoi(f[strings.LastIndex(f, ":")+1:]); n > 1500 {
			warns = append(warns, f+" exceeds 1500 LOC — candidate for domain split")
		}
	}
	out := map[string]any{
		"tool":      "qedev metrics structure",
		"timestamp": time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		"warnings":  warns,
		"metrics": map[string]any{
			"loc_product":            locP,
			"loc_test":               locT,
			"test_product_ratio":     fmt.Sprintf("%.2f", ratio),
			"test_funcs":             tests,
			"exec_command_sites":     execCalls,
			"exec_command_seam_vars": seamVars,
			"go_modules":             len(mods),
			"modules_outside_work":   missing,
			"dep_require_refs":       len(deps),
			"files_over_500_loc":     big,
		},
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	fmt.Println(string(b))
	return 0
}

var growMetrics = map[string]bool{
	"coverage_bash": true, "coverage_go": true,
	"catalog_os": true, "spec_files": true,
}

func metricTrend() int {
	r := root()
	path := filepath.Join(r, "docs", "quality", "history.jsonl")
	if p := os.Getenv("QEDEV_HISTORY"); p != "" {
		path = p
	}
	f, err := os.Open(path)
	if err != nil {
		fmt.Println("no history yet")
		return 0
	}
	defer f.Close()
	series := map[string][]float64{}
	var rows int
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		if l == "" {
			continue
		}
		rows++
		var rec map[string]any
		if json.Unmarshal([]byte(l), &rec) != nil {
			continue
		}
		for k, v := range rec {
			if k == "timestamp" || k == "commit" || k == "branch" || k == "run" {
				continue
			}
			if f := num(v); f != nil {
				series[k] = append(series[k], *f)
			}
		}
	}
	if rows < 2 {
		fmt.Println("need >=2 records for trend")
		return 0
	}
	regress := 0
	keys := make([]string, 0, len(series))
	for k := range series {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		vs := series[k]
		first, last := vs[0], vs[len(vs)-1]
		delta := last - first
		mark := byte('=')
		if delta > 0 {
			mark = '+'
		} else if delta < 0 {
			mark = '-'
			if growMetrics[k] {
				regress = 1
			}
		}
		fmt.Printf("%c %s: %g -> %g (%+g)\n", mark, k, first, last, delta)
	}
	return regress
}

// num coerces "42", "42.5%" and json numbers to float; nil otherwise.
func num(v any) *float64 {
	switch t := v.(type) {
	case float64:
		return &t
	case string:
		s := strings.TrimSuffix(t, "%")
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return &f
		}
	}
	return nil
}

func lines(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

func uniq(s string) int {
	set := map[string]bool{}
	for _, l := range strings.Split(s, "\n") {
		if l != "" {
			set[l] = true
		}
	}
	return len(set)
}

func or(a, b string) string {
	if a == "" {
		return b
	}
	return a
}
