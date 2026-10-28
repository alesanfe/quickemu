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
	bin, err := exec.LookPath(viewer)
	if err != nil {
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
	cmd := execCommand(bin, args...)
	// detached — viewer outlives the CLI process; a spawn failure
	// must not be silent (the VM still runs, just headless).
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "WARNING! could not start %s: %v\n", viewer, err)
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
		fmt.Fprintln(os.Stderr, "ERROR! VM display is '"+get("display")+"' — spice-console needs display=spice.")
		return 1
	}
	startViewer(get)
	fmt.Println(" - Viewer:   attached to " + vmname())
	return 0
}

// actVNC: vnc parity target — open a VNC client against a running VM
// configured with display=vnc. vnc_listen is host:display (display N
// maps to TCP 5900+N, same arithmetic as QEMU -vnc).
func actVNC() int {
	if !vmRunning() {
		fmt.Fprintf(os.Stderr, "ERROR! %s is not running — no VNC session to attach to.\n", vmname())
		return 1
	}
	conf := confKV()
	if conf["display"] != "vnc" {
		fmt.Fprintln(os.Stderr, "ERROR! VM display is '"+conf["display"]+"' — vnc needs display=vnc.")
		return 1
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
		fmt.Fprintf(os.Stderr, "ERROR! '%s' is not a valid VNC listen address (expected host:display).\n", listen)
		return 1
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
			fmt.Println(" - VNC:      attaching " + cand + " to " + host + ":" + port)
			c.Start()
			return 0
		}
	}
	fmt.Fprintln(os.Stderr, "ERROR! No VNC viewer found (remote-viewer, vncviewer, xtightvncviewer, gvncviewer).")
	fmt.Fprintln(os.Stderr, " - VNC:      VM listens on "+host+":"+port)
	return 1
}

// actPublicDir: open the VM's public_dir in the host file manager,
// creating it if needed — the 'publicdir' subcommand.
func actPublicDir() int {
	conf := confKV()
	pub := conf["public_dir"]
	if pub == "" {
		fmt.Fprintln(os.Stderr, "ERROR! public_dir is not set in "+vmname()+" config.")
		return 1
	}
	if !filepath.IsAbs(pub) {
		pub = filepath.Join(vmdir(), pub)
	}
	if err := os.MkdirAll(pub, 0755); err != nil {
		fmt.Fprintln(os.Stderr, "ERROR! Cannot create "+pub+": "+err.Error())
		return 1
	}
	opener := "xdg-open"
	if runtime.GOOS == "darwin" {
		opener = "open"
	}
	bin, err := exec.LookPath(opener)
	if err != nil {
		fmt.Println(" - Public:   " + pub) // no file manager — still give the path
		return 0
	}
	execCommand(bin, pub).Start()
	fmt.Println(" - Public:   " + pub)
	return 0
}

// actTelnet: interactive attach to a QEMU telnet chardev — used for
// serial-console (serial=telnet) and monitor-console (monitor=telnet).
func actTelnet(kind string) int {
	if !vmRunning() {
		fmt.Fprintf(os.Stderr, "ERROR! %s is not running.\n", vmname())
		return 1
	}
	conf := confKV()
	if conf[kind] != "telnet" {
		fmt.Fprintf(os.Stderr, "ERROR! %s is '%s' — attach needs %s=telnet.\n", kind, conf[kind], kind)
		return 1
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
	fmt.Println(" - " + strings.ToUpper(kind[:1]) + kind[1:] + ":   telnet " + host + " " + port)
	return telnetAttach(host, port)
}
