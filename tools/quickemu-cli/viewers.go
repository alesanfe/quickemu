package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// startViewer: start_viewer parity — attach spicy/remote-viewer when
// display=spice. Unix socket (path in .spice or default <vm>.sock for
// local access) or TCP (port in .spice / spice_port for remote).
func startViewer(get func(string) string) {
	viewer := get("viewer")
	if viewer == "" {
		viewer = "spicy"
	}
	if viewer == "none" || get("display") != "spice" {
		return
	}
	var args []string
	spiceInfo := ""
	if b, err := os.ReadFile(filepath.Join(vmdir(), vmname()+".spice")); err == nil {
		spiceInfo = strings.TrimSpace(string(b))
	}
	switch {
	case strings.Contains(spiceInfo, "/"):
		if viewer == "spicy" {
			args = append(args, "--uri=spice+unix://"+spiceInfo)
		} else {
			args = append(args, "spice+unix://"+spiceInfo)
		}
	case spiceInfo != "":
		if viewer == "spicy" {
			args = append(args, "--port", spiceInfo)
		} else {
			args = append(args, "spice://localhost:"+spiceInfo)
		}
	default:
		access := get("access")
		if access == "" || access == "local" {
			sock := filepath.Join(vmdir(), vmname()+".sock")
			if viewer == "spicy" {
				args = append(args, "--uri=spice+unix://"+sock)
			} else {
				args = append(args, "spice+unix://"+sock)
			}
		} else {
			port := get("spice_port")
			if viewer == "spicy" {
				args = append(args, "--port", port)
			} else {
				args = append(args, "spice://localhost:"+port)
			}
		}
	}
	args = append(args, "--title", vmname())
	if get("guest_os") != "macos" {
		if pub := get("public_dir"); pub != "" {
			args = append(args, "--spice-shared-dir", pub)
		}
	}
	if get("fullscreen") == "true" || get("fullscreen") == "on" {
		args = append(args, "--full-screen")
	}
	if vea := get("viewer_extra_args"); vea != "" {
		args = append(args, strings.Fields(vea)...)
	}
	// start_viewer echo (stdout) — Bash prints the intent line
	// unconditionally before spawning, so a missing viewer binary still
	// shows what would have run.
	fmt.Printf(" - Viewer:   %s %s >/dev/null 2>&1 &\n", viewer, strings.Join(args, " "))
	bin, err := exec.LookPath(viewer)
	if err != nil {
		return
	}
	// detached spawn — the viewer outlives the CLI; a spawn failure
	// warns but leaves the (still running) VM headless.
	if err := execCommand(bin, args...).Start(); err != nil {
		fmt.Fprintf(os.Stderr, "WARNING! Could not start viewer (%s) Err: %v\n", viewer, err)
	}
}

// actSpiceConsole: spice-console parity target — attach the configured
// SPICE viewer to an already-running VM (start_viewer on attach).
func actSpiceConsole() int {
	if !vmRunning() {
		fmt.Fprintf(os.Stderr, "ERROR! %s is not running — no SPICE session to attach to.\n", vmname())
		return 1
	}
	conf := confKV()
	get := func(k string) string {
		if v := confOpts[k]; v != "" {
			return v
		}
		return conf[k]
	}
	if get("display") != "spice" && get("display") != "spice-app" {
		if outFmt == "json" {
			emitFail("vm.spice-console", "bad_display",
				"VM display is '"+get("display")+"' — spice-console needs display=spice.")
		} else {
			fmt.Fprintln(os.Stderr, "ERROR! VM display is '"+get("display")+"' — spice-console needs display=spice.")
		}
		return 1
	}
	startViewer(get)
	if outFmt == "json" {
		emitOK("vm.spice-console", map[string]any{"vm": vmname()})
	} else {
		fmt.Println(" - Viewer:   attached to " + vmname())
	}
	return 0
}

// actVNC: vnc parity target — open a VNC client against a running VM
// configured with display=vnc. vnc_listen is host:display (display N
// maps to TCP 5900+N, same arithmetic as QEMU -vnc).
func actVNC() int {
	fail := func(msg string) int {
		if outFmt == "json" {
			emitFail("vm.vnc", "vnc_failed", msg)
		} else {
			fmt.Fprintln(os.Stderr, "ERROR! "+msg)
		}
		return 1
	}
	if !vmRunning() {
		return fail(vmname() + " is not running — no VNC session to attach to.")
	}
	conf := confKV()
	if conf["display"] != "vnc" {
		return fail("VM display is '" + conf["display"] + "' — vnc needs display=vnc.")
	}
	listen := conf["vnc_listen"]
	if listen == "" {
		listen = "127.0.0.1:0" // bin/quickemu default
	}
	host, disp := listen, "0"
	if i := strings.LastIndex(listen, ":"); i >= 0 {
		host, disp = listen[:i], listen[i+1:]
	}
	if host == "" {
		host = "127.0.0.1"
	}
	d, err := strconv.Atoi(disp)
	if err != nil || d < 0 || d > 99 {
		return fail("'" + listen + "' is not a valid VNC listen address (expected host:display).")
	}
	port := strconv.Itoa(5900 + d)
	// remote-viewer understands vnc:// URIs; plain vncviewer wants host::port.
	for _, cand := range []string{"remote-viewer", "vncviewer", "xtightvncviewer", "gvncviewer"} {
		if bin, err := exec.LookPath(cand); err == nil {
			var c *exec.Cmd
			if cand == "remote-viewer" {
				c = execCommand(bin, "vnc://"+host+":"+port)
			} else {
				c = execCommand(bin, host+"::"+port)
			}
			if outFmt == "json" {
				emitOK("vm.vnc", map[string]any{
					"vm": vmname(), "viewer": cand, "host": host, "port": port})
			} else {
				fmt.Println(" - VNC:      attaching " + cand + " to " + host + ":" + port)
			}
			c.Start()
			return 0
		}
	}
	return fail("No VNC viewer found (remote-viewer, vncviewer, xtightvncviewer, gvncviewer); " +
		"VM listens on " + host + ":" + port)
}

// actPublicDir: open the VM's public_dir in the host file manager,
// creating it if needed — the 'publicdir' subcommand.
func actPublicDir() int {
	fail := func(msg string) int {
		if outFmt == "json" {
			emitFail("vm.publicdir", "publicdir_failed", msg)
		} else {
			fmt.Fprintln(os.Stderr, "ERROR! "+msg)
		}
		return 1
	}
	conf := confKV()
	pub := conf["public_dir"]
	if pub == "" {
		return fail("public_dir is not set in " + vmname() + " config.")
	}
	if !filepath.IsAbs(pub) {
		pub = filepath.Join(vmdir(), pub)
	}
	if err := os.MkdirAll(pub, 0755); err != nil {
		return fail("Cannot create " + pub + ": " + err.Error())
	}
	opener := "xdg-open"
	if runtime.GOOS == "darwin" {
		opener = "open"
	}
	if bin, err := exec.LookPath(opener); err == nil {
		execCommand(bin, pub).Start()
	}
	if outFmt == "json" {
		emitOK("vm.publicdir", map[string]any{"vm": vmname(), "path": pub})
	} else {
		fmt.Println(" - Public:   " + pub)
	}
	return 0
}

// actTelnet: interactive attach to a QEMU telnet chardev — used for
// serial-console (serial=telnet) and monitor-console (monitor=telnet).
func actTelnet(kind string) int {
	emit := "vm." + kind + "-console"
	fail := func(msg string) int {
		if outFmt == "json" {
			emitFail(emit, "attach_failed", msg)
		} else {
			fmt.Fprintf(os.Stderr, "ERROR! %s\n", msg)
		}
		return 1
	}
	if !vmRunning() {
		return fail(vmname() + " is not running.")
	}
	conf := confKV()
	if conf[kind] != "telnet" {
		return fail(fmt.Sprintf("%s is '%s' — attach needs %s=telnet.", kind, conf[kind], kind))
	}
	host := conf[kind+"_telnet_host"]
	if host == "" {
		host = "localhost"
	}
	port := conf[kind+"_telnet_port"]
	if port == "" {
		if kind == "monitor" {
			port = "4440"
		} else {
			port = "6660"
		}
	}
	if outFmt != "json" {
		fmt.Println(" - " + strings.ToUpper(kind[:1]) + kind[1:] + ":   telnet " + host + " " + port)
	}
	rc := telnetAttach(host, port)
	if outFmt == "json" {
		if rc != 0 {
			emitFail(emit, "attach_failed", "telnet attach exited "+strconv.Itoa(rc))
		} else {
			emitOK(emit, map[string]any{"vm": vmname(), "host": host, "port": port})
		}
	}
	return rc
}
