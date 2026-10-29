package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/alesanfe/quickemu/tools/qelib"
)

// hasCtl: raw \n/\r/etc. in a generated ini-style file (systemd unit,
// .desktop entry) injects extra keys — refuse them outright.
func hasCtl(s string) bool {
	return strings.IndexFunc(s, unicode.IsControl) >= 0
}

// unitQuote: systemd Exec* argument quoting — double quotes with \ and "
// backslash-escaped. Paths with spaces stay a single argument.
func unitQuote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

// deskQuote: desktop-entry Exec quoting per the spec — wrap in double
// quotes and backslash-escape " ` $ \.
func deskQuote(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "`", "\\`", "$", `\$`)
	return `"` + r.Replace(s) + `"`
}

// launcherPath: the `quickemu` CLI the unit/shortcut should invoke.
// QE_LAUNCHER is set by quickemu-cli (its own argv0); otherwise prefer
// a `quickemu` on PATH, then our own binary as last resort.
func launcherPath() string {
	if l := os.Getenv("QE_LAUNCHER"); l != "" {
		return l
	}
	if l, err := exec.LookPath("quickemu"); err == nil {
		return l
	}
	self, _ := os.Executable()
	return self
}

// rcFail: die semantics as a return code — emits the error envelope /
// "ERROR!" line but lets the caller keep control (and stay testable).
func rcFail(code, msg string) int {
	if outputJSON {
		qelib.Emit(opCmd, "quickemu-vm/"+vmVersion(), false, nil,
			[]qelib.QEError{{Code: code, Message: msg, Severity: "error", Retryable: false}})
	} else {
		fmt.Fprintln(os.Stderr, "ERROR! "+msg)
	}
	return 1
}

func autostartCreate(name, confPath string) int {
	if !validVMName(name) {
		return rcFail("INVALID_ARGS", "autostart needs a valid vm name")
	}
	unitDir := filepath.Join(qelib.XDGConfig(), "systemd", "user")
	os.MkdirAll(unitDir, 0755)
	unit := filepath.Join(unitDir, "quickemu-"+name+".service")
	self, _ := filepath.EvalSymlinks(launcherPath())
	if self == "" {
		self = launcherPath()
	}
	abs, _ := filepath.Abs(confPath)
	if hasCtl(self) || hasCtl(abs) {
		return rcFail("INVALID_ARGS", "paths with control characters cannot go into a unit file")
	}
	unitBody := "[Unit]\nDescription=Quickemu VM " + name + "\nAfter=default.target\n\n" +
		"[Service]\nExecStart=" + unitQuote(self) + " --vm " + unitQuote(abs) + "\nRestart=on-failure\n\n" +
		"[Install]\nWantedBy=default.target\n"
	if err := os.WriteFile(unit, []byte(unitBody), 0644); err != nil {
		die("AUTOSTART_FAILED", err.Error())
	}
	if s, err := exec.LookPath("systemctl"); err == nil {
		if err := execCommand(s, "--user", "daemon-reload").Run(); err != nil {
			fmt.Fprintf(os.Stderr, "WARNING: daemon-reload: %v\n", err)
		}
		if err := execCommand(s, "--user", "enable", "quickemu-"+name+".service").Run(); err != nil {
			fmt.Fprintf(os.Stderr, "WARNING: autostart not enabled: %v\n", err)
		}
	}
	info(" - Autostart unit written: " + unit)
	info("             Start now with: systemctl --user start quickemu-" + name + ".service")
	okMap(map[string]any{"unit": unit})
	return 0
}

// shortcutCreate/Delete: .desktop file management.
func shortcutCreate(name, conf, opts string) int {
	if !validVMName(name) || hasCtl(conf) || hasCtl(opts) {
		return rcFail("INVALID_ARGS", "shortcut needs a valid name, conf path and options")
	}
	dir := filepath.Join(qelib.XDGData(), "applications")
	os.MkdirAll(dir, 0755)
	file := filepath.Join(dir, name+".desktop")
	exec := deskQuote(filepath.Base(launcherPath())) + " --vm " + deskQuote(conf)
	if opts != "" {
		exec += " " + opts
	}
	body := "[Desktop Entry]\nVersion=1.0\nType=Application\nTerminal=false\n" +
		"Exec=" + exec + "\n" +
		"Path=" + filepath.Dir(conf) + "\nName=" + name + "\nIcon=qemu\n"
	if err := os.WriteFile(file, []byte(body), 0644); err != nil {
		die("SHORTCUT_FAILED", err.Error())
	}
	info(" - " + file + " created.")
	okMap(map[string]any{"shortcut": file})
	return 0
}

func shortcutDelete(name string) {
	if !validVMName(name) {
		return
	}
	file := filepath.Join(qelib.XDGData(), "applications", name+".desktop")
	if _, err := os.Stat(file); err == nil {
		os.Remove(file)
		info(" - Deleted " + file)
	}
}
