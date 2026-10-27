// quickreport — system diagnostics tool in Go. Replaces bin/quickreport
// + lib/quickreport/*: host probes, human report, JSON envelope, and
// the qr_redact scrubbing filter. Zero Bash delegation.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

func cmdOut(name string, args ...string) string {
	if bin, err := exec.LookPath(name); err == nil {
		if out, err := exec.Command(bin, args...).Output(); err == nil {
			return strings.TrimSpace(string(out))
		}
	}
	return ""
}

// probeOSName: macOS via sw_vers, else PRETTY_NAME from /etc/os-release.
func probeOSName() string {
	if runtime.GOOS == "darwin" {
		if name, ver := cmdOut("sw_vers", "-productName"), cmdOut("sw_vers", "-productVersion"); name != "" {
			return strings.TrimSpace(name + " " + ver)
		}
		return "macOS"
	}
	if b, err := os.ReadFile("/etc/os-release"); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(l, "PRETTY_NAME=") {
				return strings.Trim(strings.TrimPrefix(l, "PRETTY_NAME="), `"`)
			}
		}
	}
	return "Unknown OS"
}

// probeQuickemu: resolved quickemu path (sibling of argv[0], else PATH).
func probeQuickemu() string {
	if exe, err := os.Executable(); err == nil {
		sib := exe[:strings.LastIndex(exe, string(os.PathSeparator))+1] + "quickemu"
		if st, err := os.Stat(sib); err == nil && st.Mode()&0111 != 0 {
			return sib
		}
	}
	if p, err := exec.LookPath("quickemu"); err == nil {
		return p
	}
	return ""
}

// probeMemoryGB: installed RAM in GB.
func probeMemoryGB() string {
	if runtime.GOOS == "darwin" {
		if out := cmdOut("sysctl", "-n", "hw.memsize"); out != "" {
			if n, err := strconv.ParseInt(out, 10, 64); err == nil {
				return strconv.FormatInt(n/(1048576*1024), 10)
			}
		}
		return ""
	}
	// free --giga -h: first numeric token, strip trailing G.
	out := cmdOut("free", "--giga", "-h")
	for _, f := range strings.Fields(out) {
		if f[0] >= '0' && f[0] <= '9' {
			return strings.TrimSuffix(f, "G")
		}
	}
	return ""
}

// probeGPUs: GPU model names, one per line.
func probeGPUs() []string {
	var gpus []string
	if runtime.GOOS == "darwin" {
		for _, l := range strings.Split(cmdOut("system_profiler", "SPDisplaysDataType"), "\n") {
			if strings.Contains(l, "Chipset Model") {
				if i := strings.Index(l, ":"); i >= 0 {
					gpus = append(gpus, strings.TrimSpace(l[i+1:]))
				}
			}
		}
		return gpus
	}
	for _, l := range strings.Split(cmdOut("lspci"), "\n") {
		if strings.Contains(strings.ToLower(l), "vga") {
			parts := strings.SplitN(l, ":", 3)
			if len(parts) == 3 {
				gpus = append(gpus, strings.TrimSpace(parts[2]))
			}
		}
	}
	return gpus
}

// probeQemuArch: arm64 → aarch64, else uname -m.
func probeQemuArch() string {
	if runtime.GOARCH == "arm64" {
		return "aarch64"
	}
	return runtime.GOARCH
}

// probeCPUInfo: lscpu output / machdep brand string.
func probeCPUInfo() string {
	if runtime.GOOS == "darwin" {
		return cmdOut("sysctl", "-n", "machdep.cpu.brand_string")
	}
	return cmdOut("lscpu")
}

func uname(args string) string { return cmdOut("uname", strings.Fields(args)...) }

// qrRedact: scrub HOME/user/host/IP/MAC/credentialed URLs (qr_redact).
var (
	ipRE    = regexp.MustCompile(`([0-9]{1,3}\.){3}[0-9]{1,3}`)
	macRE   = regexp.MustCompile(`([0-9a-fA-F]{2}:){5}[0-9a-fA-F]{2}`)
	credsRE = regexp.MustCompile(`(https?://)[^:/@]+:[^@/]+@`)
)

func qrRedact(s string) string {
	home := os.Getenv("HOME")
	if home != "" {
		s = strings.ReplaceAll(s, home, "<home>")
	}
	if u := os.Getenv("USER"); u != "" {
		s = regexp.MustCompile(`\b`+regexp.QuoteMeta(u)+`\b`).ReplaceAllString(s, "<user>")
	}
	if h, err := os.Hostname(); err == nil && h != "" {
		s = strings.ReplaceAll(s, h, "<host>")
		if i := strings.Index(h, "."); i > 0 {
			s = strings.ReplaceAll(s, h[:i], "<host>")
		}
	}
	s = ipRE.ReplaceAllString(s, "<ip>")
	s = macRE.ReplaceAllString(s, "<mac>")
	return credsRE.ReplaceAllString(s, "${1}<creds>@")
}

const sep = "----------------------------------"

func quickReport() int {
	q := probeQuickemu()
	if q == "" {
		fmt.Println(sep + "\n        Quickemu missing!\n" + sep)
		return 1
	}
	version := cmdOut(q, "--version")
	fmt.Printf("%s\n        Quickemu %s\n%s\n", sep, version, sep)
	fmt.Printf("Distro:\t%s\n", probeOSName())
	fmt.Printf("Kernel:\t%s\n", uname("-s -r -m"))
	fmt.Printf("Memory:\t%sG\n", probeMemoryGB())
	gpus := probeGPUs()
	if len(gpus) <= 1 {
		fmt.Println("GPU:")
	} else {
		fmt.Println("GPUs:")
	}
	for _, g := range gpus {
		fmt.Println(" -" + g)
	}
	if v := cmdOut("curl", "--version"); v != "" {
		lines := strings.Split(v, "\n")
		fmt.Printf("%s\n            curl %s\n%s\n", sep, strings.Fields(lines[0])[1], sep)
		libs := ""
		if i := strings.Index(lines[0], ")"); i >= 0 {
			libs = lines[0][i+1:]
		}
		fmt.Printf("Libraries:%s\n", libs)
		if len(lines) > 2 {
			fmt.Printf("Protocols:%s\n", cutAfter(lines[2], ":"))
		}
		if len(lines) > 3 {
			fmt.Printf("Features: %s\n", cutAfter(lines[3], ":"))
		}
	} else {
		fmt.Println(sep + "\n            curl missing\n" + sep)
	}
	qarch := probeQemuArch()
	qemu := "qemu-system-" + qarch
	if _, err := exec.LookPath(qemu); err == nil {
		ver := ""
		if f := strings.Fields(cmdOut(qemu, "--version")); len(f) >= 4 {
			ver = f[3]
		}
		fmt.Printf("%s\n            QEMU %s\n%s\n", sep, ver, sep)
		fmt.Println(cmdOut(qemu, "-cpu", "help"))
	} else {
		fmt.Println(sep + "\n            QEMU missing\n" + sep)
	}
	fmt.Printf("%s\n               CPU\n%s\n", sep, sep)
	fmt.Println(probeCPUInfo())
	return 0
}

func cutAfter(s, sep string) string {
	if i := strings.Index(s, sep); i >= 0 {
		return s[i+len(sep):]
	}
	return s
}

func quickReportJSON() int {
	qver := ""
	qarch := probeQemuArch()
	if _, err := exec.LookPath("qemu-system-" + qarch); err == nil {
		qver = cmdOut("qemu-system-"+qarch, "--version")
	}
	hasCurl := "false"
	if _, err := exec.LookPath("curl"); err == nil {
		hasCurl = "true"
	}
	jq := func(s string) string {
		b, _ := jsonEscape(s)
		return string(b)
	}
	out := fmt.Sprintf(`{"schemaVersion":1,"command":"host.report","success":true,"data":{"os":%s,"kernel":%s,"arch":%s,"quickemu":%s,"qemu":%s,"curl":%s},"warnings":[],"errors":[],"meta":{"redacted":true}}`,
		jq(probeOSName()), jq(uname("-sr")), jq(uname("-m")),
		jq(probeQuickemu()), jq(firstLine(qver)), hasCurl)
	fmt.Println(qrRedact(out))
	return 0
}

func firstLine(s string) string {
	return strings.SplitN(s, "\n", 2)[0]
}

// jsonEscape renders s as a JSON string literal.
func jsonEscape(s string) ([]byte, error) {
	q := `"`
	for _, r := range s {
		switch r {
		case '"', '\\':
			q += `\` + string(r)
		case '\n':
			q += `\n`
		case '\t':
			q += `\t`
		default:
			if r < 0x20 {
				q += fmt.Sprintf(`\u%04x`, r)
			} else {
				q += string(r)
			}
		}
	}
	return []byte(q + `"`), nil
}

func main() {
	args := os.Args[1:]
	if len(args) >= 1 && (args[0] == "--format=json" ||
		(len(args) >= 2 && args[0] == "--format" && args[1] == "json")) {
		// quick_report_json | tee quickreport.json
		rc := captureStdout(func() { exitWith(quickReportJSON()) }, "quickreport.json")
		os.Exit(rc)
	}
	// quick_report | qr_redact | tee quickreport.txt
	rc := captureStdout(func() { exitWith(quickReport()) }, "quickreport.txt")
	os.Exit(rc)
}

// captureStdout runs fn with stdout redirected to a pipe, then prints
// the qr_redact'ed output and tees it to `file` when non-empty. The
// os.Exit inside fn is recovered so the report still flushes.
func captureStdout(fn func(), file string) (rc int) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	done := make(chan struct{})
	var buf strings.Builder
	go func() {
		b := make([]byte, 8192)
		for {
			n, err := r.Read(b)
			if n > 0 {
				buf.Write(b[:n])
			}
			if err != nil {
				close(done)
				return
			}
		}
	}()
	func() {
		defer func() {
			if e := recover(); e != nil {
				if c, ok := e.(exitCode); ok {
					rc = c.code
				}
			}
		}()
		fn()
	}()
	w.Close()
	os.Stdout = old
	<-done
	out := qrRedact(buf.String())
	fmt.Print(out)
	if file != "" {
		os.WriteFile(file, []byte(out), 0644)
	}
	return rc
}

type exitCode struct{ code int }

func exitWith(code int) { panic(exitCode{code}) }
