package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/alesanfe/quickemu/tools/qelib"
)

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

func autostartCreate(name, confPath string) int {
	unitDir := filepath.Join(qelib.XDGConfig(), "systemd", "user")
	os.MkdirAll(unitDir, 0755)
	unit := filepath.Join(unitDir, "quickemu-"+name+".service")
	self, _ := filepath.EvalSymlinks(launcherPath())
	if self == "" {
		self = launcherPath()
	}
	abs, _ := filepath.Abs(confPath)
	unitBody := "[Unit]\nDescription=Quickemu VM " + name + "\nAfter=default.target\n\n" +
		"[Service]\nExecStart=" + self + " --vm " + abs + "\nRestart=on-failure\n\n" +
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
	dir := filepath.Join(qelib.XDGData(), "applications")
	os.MkdirAll(dir, 0755)
	file := filepath.Join(dir, name+".desktop")
	body := "[Desktop Entry]\nVersion=1.0\nType=Application\nTerminal=false\n" +
		"Exec=" + filepath.Base(launcherPath()) + " --vm " + conf + " " + opts + "\n" +
		"Path=" + filepath.Dir(conf) + "\nName=" + name + "\nIcon=qemu\n"
	if err := os.WriteFile(file, []byte(body), 0644); err != nil {
		die("SHORTCUT_FAILED", err.Error())
	}
	info(" - " + file + " created.")
	okMap(map[string]any{"shortcut": file})
	return 0
}

func shortcutDelete(name string) {
	file := filepath.Join(qelib.XDGData(), "applications", name+".desktop")
	if _, err := os.Stat(file); err == nil {
		os.Remove(file)
		info(" - Deleted " + file)
	}
}
