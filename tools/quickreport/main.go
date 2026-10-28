// quickreport — system diagnostics tool in Go. Replaces bin/quickreport
// + lib/quickreport/*: host probes, human report, JSON envelope, and
// the qr_redact scrubbing filter. Zero Bash delegation.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
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

// probeQuickemu: resolved quickemu path (sibling of argv[0], the dev-tree
// bin/ next to the tools dir, else PATH).
func probeQuickemu() string {
	if exe, err := os.Executable(); err == nil {
		dir := exe[:strings.LastIndex(exe, string(os.PathSeparator))+1]
		for _, sib := range []string{dir + "quickemu", dir + "../../bin/quickemu"} {
			if st, err := os.Stat(sib); err == nil && st.Mode()&0111 != 0 {
				return sib
			}
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
	// /proc/meminfo MemTotal — same source `free` reads, no subprocess.
	// free --giga reports SI GB (1e9 B), so: kib * 1024 / 1e9.
	if b, err := os.ReadFile("/proc/meminfo"); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(l, "MemTotal:") {
				if f := strings.Fields(l); len(f) >= 2 {
					if kb, err := strconv.ParseInt(f[1], 10, 64); err == nil {
						return strconv.FormatInt(kb*1024/1_000_000_000, 10)
					}
				}
			}
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
	return pciGPUNames()
}

// probeQemuArch: arm64 → aarch64, else uname -m.
func probeQemuArch() string {
	if runtime.GOARCH == "arm64" {
		return "aarch64"
	}
	return runtime.GOARCH
}

// probeCPUInfo: /proc/cpuinfo content / machdep brand string. lscpu
// itself formats cpuinfo — reading the source avoids the subprocess.
func probeCPUInfo() string {
	if runtime.GOOS == "darwin" {
		return cmdOut("sysctl", "-n", "machdep.cpu.brand_string")
	}
	if b, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		return strings.TrimSpace(string(b))
	}
	return ""
}

// uname: `uname -s -r -m` fields without spawning uname — the kernel
// info comes from the uname(2) syscall (x/sys/unix).
func uname(flags string) string {
	var u unix.Utsname
	if unix.Uname(&u) != nil {
		return ""
	}
	m := map[byte]string{
		's': unix.ByteSliceToString(u.Sysname[:]),
		'r': unix.ByteSliceToString(u.Release[:]),
		'v': unix.ByteSliceToString(u.Version[:]),
		'm': unix.ByteSliceToString(u.Machine[:]),
		'n': unix.ByteSliceToString(u.Nodename[:]),
	}
	var out []string
	for _, f := range strings.Fields(flags) {
		for _, c := range []byte(strings.TrimPrefix(f, "-")) {
			if v, ok := m[c]; ok && v != "" {
				out = append(out, v)
			}
		}
	}
	return strings.Join(out, " ")
}

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
	_, hasCurl := exec.LookPath("curl")
	b, _ := json.Marshal(map[string]any{
		"schemaVersion": 1, "command": "host.report", "success": true,
		"data": map[string]any{
			"os": probeOSName(), "kernel": uname("-sr"), "arch": uname("-m"),
			"quickemu": probeQuickemu(), "qemu": firstLine(qver),
			"curl": hasCurl == nil},
		"warnings": []any{}, "errors": []any{},
		"meta": map[string]any{"redacted": true}})
	fmt.Println(qrRedact(string(b)))
	return 0
}

func firstLine(s string) string {
	return strings.SplitN(s, "\n", 2)[0]
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
