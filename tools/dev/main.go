// Command qedev is the repository task runner. It replaces the former
// tools/*.sh scripts with a single Go binary:
//
//	qedev build                  build entrypoints into bin/ and tool binaries in-place
//	qedev check                  vet + test (-race when gcc exists) + build, all modules
//	qedev test                   go test -race -cover per module
//	qedev specs                  run the Go spec suite (tools/qespec)
//	qedev metrics chaoss         CHAOSS snapshot from git history
//	qedev metrics record [k=v..] append a record to docs/quality/history.jsonl
//	qedev metrics trend          report trends; exits 1 on regression
//	qedev probe                  nightly quickget --check against upstream sources
//	qedev check-catalog          validate bin/quickget --list-json emits valid JSON
//	qedev sbom <version>         emit a CycloneDX 1.5 SBOM of shipped binaries
//	qedev bench                  hyperfine startup benchmarks (on-demand)
//
// Run it via `go run ./tools/dev <cmd>` or the compiled binary.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// entries maps cli modules to their public bin/ name.
var entries = map[string]string{
	"quickemu-cli": "quickemu",
	"quickget-cli": "quickget",
	"quickgetc":    "quickgetc",
	"quickreport":  "quickreport",
}

// noBin are modules whose binaries never ship to bin/.
var noBin = map[string]bool{"dev": true, "qespec": true}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: qedev build|check|test|specs|metrics ...")
		os.Exit(2)
	}
	var rc int
	switch os.Args[1] {
	case "build":
		rc = cmdBuild()
	case "check":
		rc = cmdCheck()
	case "test":
		rc = cmdTest()
	case "specs":
		rc = cmdSpecs()
	case "metrics":
		rc = cmdMetrics(os.Args[2:])
	case "probe":
		rc = cmdProbe()
	case "check-catalog":
		rc = cmdCheckCatalog()
	case "sbom":
		rc = cmdSbom(os.Args[2:])
	case "bench":
		rc = cmdBench()
	case "release-notes":
		rc = cmdReleaseNotes()
	default:
		fmt.Fprintf(os.Stderr, "qedev: unknown command %q\n", os.Args[1])
		rc = 2
	}
	os.Exit(rc)
}

// root walks up from CWD to the directory holding go.work (repo root).
func root() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	for {
		if st, err := os.Stat(filepath.Join(dir, "go.work")); err == nil && !st.IsDir() {
			return dir
		}
		up := filepath.Dir(dir)
		if up == dir {
			wd, _ := os.Getwd()
			return wd
		}
		dir = up
	}
}

// goTool resolves the go binary, falling back to common install paths.
func goTool() string {
	if p, err := exec.LookPath("go"); err == nil {
		return p
	}
	home, _ := os.UserHomeDir()
	for _, p := range []string{
		filepath.Join(home, "go", "bin", "go"),
		"/usr/local/go/bin/go",
	} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "go"
}

// modules lists dirs under tools/ containing a go.mod.
func modules(r string) []string {
	ents, err := os.ReadDir(filepath.Join(r, "tools"))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		d := filepath.Join(r, "tools", e.Name())
		if st, err := os.Stat(filepath.Join(d, "go.mod")); err == nil && !st.IsDir() {
			out = append(out, d)
		}
	}
	sort.Strings(out)
	return out
}

// isMain reports whether the module's root package is a main package.
func isMain(dir string) bool {
	cmd := exec.Command(goTool(), "list", "-f", "{{.Name}}", ".")
	cmd.Dir = dir
	b, err := cmd.Output()
	return err == nil && strings.TrimSpace(string(b)) == "main"
}

func run(dir, name string, env []string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}

func cmdBuild() int {
	r := root()
	rc := 0
	for _, d := range modules(r) {
		name := filepath.Base(d)
		if !isMain(d) {
			continue
		}
		out := name // in-place dev-tree binary
		if pub, ok := entries[name]; ok && !noBin[name] {
			out = filepath.Join(r, "bin", pub)
		}
		if noBin[name] {
			continue // dev/test helpers stay out of bin/ and tools in-place
		}
		if err := run(d, goTool(), nil, "build", "-o", out, "."); err != nil {
			fmt.Fprintf(os.Stderr, "FAIL %s\n", name)
			rc = 1
			continue
		}
		fmt.Printf("built %s -> %s\n", name, out)
	}
	return rc
}

func cmdCheck() int {
	r := root()
	rc := 0
	gcc := ""
	if _, err := exec.LookPath("gcc"); err == nil {
		gcc = "-race"
	}
	for _, d := range modules(r) {
		name := filepath.Base(d)
		fmt.Printf("=== tools/%s\n", name)
		env := []string{"GOWORK=off"}
		if err := run(d, goTool(), env, "vet", "./..."); err != nil {
			rc = 1
			continue
		}
		test := []string{"test"}
		if gcc != "" {
			test = append(test, gcc)
		}
		test = append(test, "-coverprofile=coverage.out", "./...")
		if err := run(d, goTool(), env, test...); err != nil {
			rc = 1
			continue
		}
		tmp := filepath.Join(os.TempDir(), name)
		if isMain(d) {
			if err := run(d, goTool(), env, "build", "-o", tmp, "."); err != nil {
				rc = 1
				continue
			}
		}
		for _, extra := range []string{"govulncheck", "staticcheck"} {
			if p, err := exec.LookPath(extra); err == nil {
				if err := run(d, p, env, "./..."); err != nil {
					rc = 1
				}
			}
		}
	}
	return rc
}

func cmdTest() int {
	r := root()
	rc := 0
	for _, d := range modules(r) {
		name := filepath.Base(d)
		fmt.Printf("== %s\n", name)
		if err := run(d, goTool(), nil, "test", "-race", "-cover", "./..."); err != nil {
			rc = 1
		}
	}
	return rc
}

// cmdSpecs runs the Go spec suite; kept a separate command so CI/docs
// have a stable name for the former ShellSpec runner.
func cmdSpecs() int {
	r := root()
	d := filepath.Join(r, "tools", "qespec")
	if _, err := os.Stat(filepath.Join(d, "go.mod")); err != nil {
		fmt.Fprintln(os.Stderr, "qedev: tools/qespec not found")
		return 1
	}
	return errRc(run(d, goTool(), nil, "test", "-timeout", "15m", "-v", "./..."))
}

func errRc(err error) int {
	if err != nil {
		return 1
	}
	return 0
}

// ---- probes / sbom / bench (former probes/, scripts/, tools/ scripts) ----

// probeOSes mirrors probes/check-sources.sh: live upstream checks for
// OSes whose download sources churn most. Nightly CI, not PR-blocking.
var probeOSes = []string{
	"blackarch", "blissos", "kaos", "midnightbsd", "nobara",
	"openmandriva", "openmediavault", "opnsense", "q4os",
	"systemrescue", "truenas-scale", "vyos",
}

func cmdProbe() int {
	r := root()
	fail := 0
	for _, name := range probeOSes {
		cmd := exec.Command(filepath.Join(r, "bin", "quickget"), "--check", name)
		if err := cmd.Run(); err != nil {
			fmt.Printf("FAIL %s\n", name)
			fail = 1
		}
	}
	return fail
}

// cmdCheckCatalog replaces tools/check-catalog: --list-json must emit
// a non-empty JSON array (no python3 needed anymore).
func cmdCheckCatalog() int {
	r := root()
	out, err := exec.Command(filepath.Join(r, "bin", "quickget"), "--list-json").Output()
	if err != nil {
		fmt.Fprintln(os.Stderr, "check-catalog: quickget --list-json failed")
		return 1
	}
	var entries []any
	if err := json.Unmarshal(out, &entries); err != nil {
		fmt.Fprintf(os.Stderr, "check-catalog: invalid JSON: %v\n", err)
		return 1
	}
	if len(entries) == 0 {
		fmt.Fprintln(os.Stderr, "check-catalog: empty catalog")
		return 1
	}
	fmt.Printf("%d catalog entries OK\n", len(entries))
	return 0
}

// cmdSbom replaces scripts/gen-sbom.sh: CycloneDX 1.5 over the shipped
// binaries (bin/ entries + in-place tool binaries).
func cmdSbom(args []string) int {
	r := root()
	ver := "unknown"
	if len(args) > 0 {
		ver = args[0]
	}
	var files []string
	add := func(dir string) {
		ents, _ := os.ReadDir(filepath.Join(r, dir))
		for _, e := range ents {
			if e.IsDir() {
				continue
			}
			if strings.HasSuffix(e.Name(), ".md") || strings.HasSuffix(e.Name(), ".out") {
				continue
			}
			p := filepath.Join(dir, e.Name())
			if f, err := os.Open(filepath.Join(r, p)); err == nil {
				head := make([]byte, 4)
				if n, _ := f.Read(head); n >= 4 && string(head[:4]) == "\x7fELF" {
					files = append(files, filepath.ToSlash(p))
				}
				f.Close()
			}
		}
	}
	add("bin")
	for _, d := range modules(r) {
		name := filepath.Base(d)
		p := filepath.Join("tools", name, name)
		if f, err := os.Open(filepath.Join(r, p)); err == nil {
			head := make([]byte, 4)
			if n, _ := f.Read(head); n >= 4 && string(head[:4]) == "\x7fELF" {
				files = append(files, filepath.ToSlash(p))
			}
			f.Close()
		}
	}
	sort.Strings(files)
	type comp struct {
		Type string `json:"type"`
		Name string `json:"name"`
	}
	comps := make([]comp, 0, len(files))
	for _, f := range files {
		comps = append(comps, comp{"file", f})
	}
	bom := map[string]any{
		"bomFormat": "CycloneDX", "specVersion": "1.5", "version": 1,
		"metadata": map[string]any{
			"timestamp": time.Now().UTC().Format("2006-01-02T15:04:05Z"),
			"tools":     map[string]string{"vendor": "quickemu", "name": "qedev sbom"},
			"component": map[string]string{"type": "application", "name": "quickemu", "version": ver},
		},
		"components": comps,
	}
	b, _ := json.MarshalIndent(bom, "", "  ")
	fmt.Println(string(b))
	return 0
}

// cmdBench replaces tools/benchmark: hyperfine startup benchmarks.
func cmdBench() int {
	r := root()
	hf, err := exec.LookPath("hyperfine")
	if err != nil {
		fmt.Fprintln(os.Stderr, "hyperfine not installed")
		return 1
	}
	q := filepath.Join(r, "bin", "quickemu")
	return errRc(run(r, hf, nil,
		q+" --capabilities",
		q+" --list-vms --format json",
		q+" --help",
		"--warmup", "3", "--export-json", "benchmark.json"))
}

// cmdReleaseNotes replaces tools/release-notes: commits since the last tag.
func cmdReleaseNotes() int {
	r := root()
	base := git("describe", "--tags", "--abbrev=0")
	if base == "" {
		base = git("rev-list", "--max-parents=0", "HEAD")
	}
	out := git("log", "--pretty=format:- %s", base+"..HEAD")
	if out != "" {
		fmt.Println(out)
	}
	_ = r
	return 0
}

// ---- metrics ----

func cmdMetrics(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: qedev metrics chaoss|record|trend [k=v...]")
		return 2
	}
	switch args[0] {
	case "chaoss":
		return metricChaoss()
	case "record":
		return metricRecord(args[1:])
	case "trend":
		return metricTrend()
	}
	return 2
}

func git(args ...string) string {
	out, err := exec.Command("git", args...).Output()
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
	out := filepath.Join(r, "docs", "quality", "history.jsonl")
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
	filepath.Walk(r, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() &&
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
	out, err := exec.Command(filepath.Join(r, "bin", "quickget"), "--list").Output()
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
		cmd := exec.Command(goTool(), "test", "-cover", "./...")
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

var growMetrics = map[string]bool{
	"coverage_bash": true, "coverage_go": true,
	"catalog_os": true, "spec_files": true,
}

func metricTrend() int {
	r := root()
	f, err := os.Open(filepath.Join(r, "docs", "quality", "history.jsonl"))
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
	sort.Strings(keys)
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
