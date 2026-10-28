package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// actIgnoreMSRsAlways: ignore_msrs_always — persist kvm ignore_msrs=Y
// via /etc/modprobe.d and rebuild initramfs (update-initramfs or
// mkinitcpio). Requires sudo, same as Bash.
func actIgnoreMSRsAlways() int {
	if !fileExists("/etc/modprobe.d") {
		fmt.Fprintln(os.Stderr, "ERROR! /etc/modprobe.d was not found, I don't know how to configure this system.")
		return 1
	}
	if b, err := os.ReadFile("/etc/modprobe.d/kvm-quickemu.conf"); err == nil && strings.Contains(string(b), "ignore_msrs=Y") {
		return 0 // already enabled (initramfs rebuilt previously)
	}
	sh := execCommand("sudo", "tee", "/etc/modprobe.d/kvm-quickemu.conf")
	sh.Stdin = strings.NewReader("options kvm ignore_msrs=Y\n")
	sh.Stdout, sh.Stderr = os.Stdout, os.Stderr
	if err := sh.Run(); err != nil {
		return 1
	}
	switch {
	case toolExists("update-initramfs"):
		return rc(execCommand("sudo", "update-initramfs", "-k", "all", "-u").Run())
	case toolExists("mkinitcpio"):
		return rc(execCommand("sudo", "mkinitcpio", "-P").Run())
	default:
		fmt.Fprintln(os.Stderr, "ERROR! User does not have update-initramfs or mkinitcpio installed, please find out respective utility to update/regenrate intiramfs.")
		fmt.Fprintln(os.Stderr, "Run respective command for rebuilding initramfs: 'sudo update-initramfs -k all -u'(debian) or 'sudo mkinitcpio -P'(arch)")
		return 1
	}
}

func toolExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// vmVerifyInvariants: lifecycle.sh parity — a state claims only what's
// true: running/paused need pidfile+runtime.json; stopped/archived
// must not have a stale pidfile.
func vmVerifyInvariants() bool {
	st, _ := state()
	dir, name := vmdir(), vmname()
	pf := filepath.Join(dir, name+".pid")
	rt := filepath.Join(dir, name+".runtime.json")
	switch st {
	case "running", "paused":
		if !fileExists(pf) {
			fmt.Fprintf(os.Stderr, "INVARIANT: %s without pidfile\n", st)
			return false
		}
		if !fileExists(rt) {
			fmt.Fprintf(os.Stderr, "INVARIANT: %s without runtime.json\n", st)
			return false
		}
	case "stopped", "archived":
		if fileExists(pf) {
			fmt.Fprintf(os.Stderr, "INVARIANT: %s with stale pidfile\n", st)
			return false
		}
	}
	return true
}

// qeDoctor: doctor.sh parity — diagnoses stale/corrupt runtime state;
// fix==true repairs what's safe (never touches disks or the conf).
// Text mode prints " - <finding>" lines; --format json emits a
// vm.doctor envelope. Runs before the capability report, like
// doctor_vm does.
func qeDoctor(fix bool) {
	vmVerifyInvariants()
	findings := []string{}
	dir, name := vmdir(), vmname()

	rt := filepath.Join(dir, name+".runtime.json")
	if b, err := os.ReadFile(rt); err == nil && !strings.Contains(string(b), `"schemaVersion"`) {
		findings = append(findings, "runtime-corrupt:"+rt)
		if fix {
			os.Remove(rt)
			findings = append(findings, "fixed:removed-corrupt-runtime")
		}
	}

	pf := filepath.Join(dir, name+".pid")
	live := false
	if b, err := os.ReadFile(pf); err == nil {
		p := strings.TrimSpace(string(b))
		if n, err := strconv.Atoi(p); err == nil && alive(n) {
			live = true
		} else if p != "" {
			findings = append(findings, "stale-pidfile:"+p)
			if fix {
				os.Remove(pf)
				os.Remove(filepath.Join(dir, name+".paused"))
				findings = append(findings, "fixed:removed-stale-pid")
			}
		}
	}

	lockPaths := []string{filepath.Join(dir, name+".lock")}
	if ents, _ := filepath.Glob(filepath.Join(dir, name+".lock.shared", "*")); len(ents) > 0 {
		lockPaths = append(lockPaths, ents...)
	}
	for _, l := range lockPaths {
		st, err := os.Stat(l)
		if err != nil || !st.IsDir() {
			continue
		}
		lp := ""
		if b, err := os.ReadFile(filepath.Join(l, "pid")); err == nil {
			lp = strings.TrimSpace(string(b))
		} else {
			lp = filepath.Base(l)
		}
		if n, err := strconv.Atoi(lp); err == nil && !alive(n) {
			findings = append(findings, "stale-lock:"+l)
			if fix {
				os.RemoveAll(l)
				findings = append(findings, "fixed:reclaimed-lock")
			}
		}
	}

	var socks []string
	for _, pat := range []string{name + "*.socket", name + ".monitor", name + ".qmp"} {
		if m, _ := filepath.Glob(filepath.Join(dir, pat)); len(m) > 0 {
			socks = append(socks, m...)
		}
	}
	for _, s := range socks {
		st, err := os.Stat(s)
		if err != nil || st.Mode()&os.ModeSocket == 0 {
			continue
		}
		if !live {
			findings = append(findings, "stale-socket:"+s)
			if fix {
				os.Remove(s)
				findings = append(findings, "fixed:removed-socket")
			}
		}
	}

	ev := filepath.Join(dir, name+".events.jsonl")
	if b, err := os.ReadFile(ev); err == nil && len(b) > 0 {
		lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
		if !strings.Contains(lines[len(lines)-1], "}") {
			findings = append(findings, "event-truncated:"+ev)
			if fix {
				rest := strings.Join(lines[:len(lines)-1], "\n")
				if rest != "" {
					rest += "\n"
				}
				if os.WriteFile(ev, []byte(rest), 0644) == nil {
					findings = append(findings, "fixed:dropped-truncated-event")
				}
			}
		}
	}

	for _, op := range func() []string {
		m, _ := filepath.Glob(filepath.Join(dir, "ops", "*.json"))
		return m
	}() {
		b, err := os.ReadFile(op)
		if err != nil {
			continue
		}
		var o struct {
			State     string `json:"state"`
			WorkerPid int    `json:"workerPid"`
		}
		if json.Unmarshal(b, &o) != nil ||
			(o.State != "queued" && o.State != "running") ||
			(o.WorkerPid > 0 && alive(o.WorkerPid)) {
			continue
		}
		findings = append(findings, "orphan-operation:"+op)
		if fix {
			s := stateRe.ReplaceAllString(string(b), `"state":"interrupted"`)
			if os.WriteFile(op, []byte(s), 0644) == nil {
				findings = append(findings, "fixed:marked-interrupted")
			}
		}
	}

	if outFmt == "json" {
		emitOK("vm.doctor", map[string]any{"vm": name, "fix": fix, "findings": findings})
	} else {
		if len(findings) == 0 {
			fmt.Println(" - " + name + ": healthy.")
		}
		for _, f := range findings {
			fmt.Println(" - " + f)
		}
	}
}

// actDoctor: doctor_vm parity — config vs host capabilities report.
func actDoctor() int {
	qeDoctor(doctorFix)
	conf := confKV()
	issues := 0
	fmt.Println("Checking " + vmname() + " against host capabilities:")
	if fileExists("/dev/kvm") {
		fmt.Println(" o KVM:  /dev/kvm available")
	} else {
		fmt.Println(" x KVM:  /dev/kvm not writable, VM will use slow TCG emulation")
		issues++
	}
	if _, err := exec.LookPath("qemu-system-x86_64"); err != nil {
		fmt.Println(" x QEMU: qemu-system-x86_64 not executable")
		issues++
	}
	if d := conf["disk_img"]; d != "" && !fileExists(d) {
		fmt.Printf(" x Disk: %s missing (will be created, %s)\n", d, conf["disk_size"])
	}
	if i := conf["iso"]; i != "" && !fileExists(i) {
		fmt.Println(" x ISO:  " + i + " missing")
		issues++
	}
	if strings.Contains(conf["boot"], "efi") {
		found := false
		for _, g := range []string{"OVMF_CODE*.fd", "AAVMF*.fd"} {
			if m, _ := filepath.Glob(filepath.Join(vmdir(), g)); len(m) > 0 {
				found = true
			}
		}
		if found {
			fmt.Println(" o EFI:  firmware files present")
		} else {
			fmt.Println(" x EFI:  no OVMF/AAVMF firmware found")
			issues++
		}
	}
	if ram := conf["ram"]; ram != "" {
		if want := memMB(ram); want > 0 {
			if free := hostFreeMemMB(); free > 0 && want*1024 > free*1024 {
				fmt.Printf(" x RAM:  VM wants %s but only %dM available\n", ram, free)
				issues++
			} else {
				fmt.Printf(" o RAM:  %s requested (%dM free)\n", ram, free)
			}
		}
	}
	if conf["tpm"] == "on" {
		if _, err := exec.LookPath("swtpm"); err != nil {
			fmt.Println(" x TPM:  tpm=on but swtpm not installed")
			issues++
		}
	}
	switch conf["network"] {
	case "", "none", "restrict", "passt", "slirp":
	default:
		if _, err := os.Stat("/sys/class/net/" + conf["network"]); err == nil {
			fmt.Println(" o NET:  bridge " + conf["network"] + " up")
		} else {
			fmt.Println(" x NET:  bridge interface " + conf["network"] + " does not exist")
			issues++
		}
	}
	if issues == 0 {
		fmt.Println(" - No problems detected.")
	} else {
		fmt.Printf(" - %d issue(s) found.\n", issues)
	}
	return 0
}
