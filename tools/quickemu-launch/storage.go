// tools/quickemu-launch/storage.go: disk/storage + fileshare arg building (virtiofs, extra_disks)
//
// Error text in this file is the Bash user-facing contract: it is
// printed verbatim to stdout, so it keeps its punctuation.
//
//lint:file-ignore ST1005 Bash parity error strings keep punctuation.
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/alesanfe/quickemu/tools/qelib"
)

// prepareStorage: configure_storage — create a missing disk_img via
// qemu-img (preallocation whitelist + per-format create options),
// write-lock probe on existing images, and the DISK_USED heuristic:
// an effectively-empty disk keeps booting the installer media while a
// used one drops iso/img so the guest boots from disk.
func prepareStorage(c Conf, dir string) error {
	d := c.get("disk_img", "")
	if d == "" {
		return nil
	}
	p := joinConf(dir, d)
	dfmt := c.get("disk_format", "qcow2")
	size := c.get("disk_size", "16G")
	pre := c.get("preallocation", "off")
	if _, err := os.Stat(p); err != nil {
		if c["_run"] != "1" {
			fmt.Printf("             Would create %s (%s, %s)\n", p, dfmt, size)
			return nil
		}
		os.MkdirAll(filepath.Dir(p), 0o755)
		switch pre {
		case "off", "metadata", "falloc", "full":
		default:
			return fmt.Errorf("ERROR! %s is an unsupported disk preallocation option.", pre)
		}
		opts := ""
		switch dfmt {
		case "qcow2":
			opts = "lazy_refcounts=on,preallocation=" + pre + ",nocow=on"
		case "raw":
			opts = "preallocation=" + pre
		}
		qi, err := exec.LookPath("qemu-img")
		if err != nil {
			return fmt.Errorf("ERROR! qemu-img not found — cannot create %s", p)
		}
		argv := []string{"create", "-q", "-f", dfmt, "-o", opts, p, size}
		cctx, ccancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer ccancel()
		if err := exec.CommandContext(cctx, qi, argv...).Run(); err != nil {
			return fmt.Errorf("ERROR! Failed to create %s using %s format.", p, dfmt)
		}
		if c.get("iso", "") == "" && c.get("img", "") == "" {
			return fmt.Errorf("ERROR! You haven't specified a .iso or .img image to boot from.")
		}
		fmt.Printf("             Just created, booting from %s%s\n",
			c.get("iso", ""), c.get("img", ""))
		return nil // DISK_USED=no — installer media stays attached.
	}
	// Existing image: the launch path implies the VM is stopped, so
	// probe the write lock (qemu-img info) — a held lock means another
	// process has the disk open.
	qi, err := exec.LookPath("qemu-img")
	if err != nil {
		return fmt.Errorf("ERROR! qemu-img not found — cannot inspect %s", p)
	}
	ictx, icancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer icancel()
	if err := exec.CommandContext(ictx, qi, "info", p).Run(); err != nil {
		return fmt.Errorf("             Failed to get \"write\" lock. Is another process using the disk?")
	}
	if pre == "off" {
		used := true
		// A qcow2 overlay is physically small but fully populated via
		// its backing file — treat as used regardless of size.
		out, _ := exec.CommandContext(ictx, qi, "info", "--output=json", p).Output()
		if !strings.Contains(string(out), `"backing-filename"`) {
			if fi, err := os.Stat(p); err == nil && fi.Size() <= 197632*8 {
				used = false
			}
		}
		if !used {
			fmt.Printf("             Looks unused, booting from %s%s\n",
				c.get("iso", ""), c.get("img", ""))
			if c.get("iso", "") == "" && c.get("img", "") == "" {
				return fmt.Errorf("ERROR! You haven't specified a .iso or .img image to boot from.")
			}
			return nil
		}
	}
	// DISK_USED=yes: a used disk means the OS is installed — detach
	// installer media so the guest boots from disk (kolibrios keeps
	// its floppy/iso attached on every boot).
	if c.get("guest_os", "") != "kolibrios" {
		c["iso"] = ""
		c["img"] = ""
	}
	// STATUS_QUO: -snapshot means writes are ephemeral — announce it
	// once the installer media has settled.
	if confOn(c, "snapshot") && c.get("iso", "") == "" && c.get("img", "") == "" {
		fmt.Printf("             Existing disk state will be preserved, no writes will be committed.\n")
	}
	return nil
}

// storageEcho: configure_storage echoes — printed during the configure
// phase (before Display/Sound/PORTS), while the drive args they
// describe are emitted much later by bootMediaArgs/storageArgs.
func storageEcho(c Conf, dir string) {
	if d := c.get("disk_img", ""); d != "" {
		fmt.Printf(" - Disk:     %s (%s)\n", joinConf(dir, d), c.get("disk_size", "16G"))
	}
	// Boot ISO / Recovery are an if/elif in Bash — Recovery only
	// echoes when there is no ISO.
	if d := c.get("iso", ""); d != "" {
		if _, err := os.Stat(joinConf(dir, d)); err == nil {
			fmt.Printf(" - Boot ISO: %s\n", joinConf(dir, d))
		}
	} else if d := c.get("img", ""); d != "" {
		if _, err := os.Stat(joinConf(dir, d)); err == nil {
			fmt.Printf(" - Recovery: %s\n", joinConf(dir, d))
		}
	}
	if d := c.get("fixed_iso", ""); d != "" {
		if _, err := os.Stat(joinConf(dir, d)); err == nil {
			fmt.Printf(" - CD-ROM:   %s\n", joinConf(dir, d))
		}
	}
}

func storageArgs(c Conf, dir string, t OsTuning) []string {
	var a []string
	osn := c.get("guest_os", "")
	mac := osn == "macos"
	dfmt := c.get("disk_format", "qcow2")
	// STATUS_QUO: Bash appends "-snapshot" right after each writable
	// disk drive (SystemDisk, ExtraDisk, if=ide) — not to cdrom,
	// pflash, BootLoader, RecoveryImage, BootDisk or seed drives.
	sq := ""
	if confOn(c, "snapshot") {
		sq = "-snapshot"
	}
	if mac {
		// vm.sh macOS emit: AHCI bus + OpenCore/ESP bootloader at
		// bootindex 0, RecoveryImage on ahci.1, SystemDisk last.
		vmdir := c["_vmdir"]
		bl := filepath.Join(vmdir, "OpenCore.qcow2")
		if _, err := os.Stat(bl); err != nil {
			bl = filepath.Join(vmdir, "ESP.qcow2")
		}
		a = append(a, "-device", "ahci,id=ahci",
			"-device", "ide-hd,bus=ahci.0,drive=BootLoader,bootindex=0",
			"-drive", "id=BootLoader,if=none,format=qcow2,file="+qemuOptEsc(bl))
		if d := c.get("img", ""); d != "" {
			a = append(a, "-device", "ide-hd,bus=ahci.1,drive=RecoveryImage",
				"-drive", "id=RecoveryImage,if=none,format=raw,file="+qemuOptEsc(joinConf(dir, d)))
		}
	}
	if d := c.get("disk_img", ""); d != "" {
		dev := "virtio-blk-pci"
		if t.MacDiskDev != "" {
			dev = t.MacDiskDev
		}
		ioth := iothreadCount(c)
		// -drive and its spec must be separate argv elements: Bash's
		// SHELL_ARGS relied on shell word-splitting, exec.Command does not.
		sys := "id=SystemDisk,if=none,format=" + dfmt + ",file=" +
			qemuOptEsc(joinConf(dir, d)) + ",discard=unmap,detect-zeroes=unmap,cache=writeback,aio=threads"
		// Per-guest SystemDisk emit (vm.sh): ahci/ide variants, then
		// the virtio default; aarch64 gets bootindex=2 so the installer
		// CD-ROM (bootindex=1) wins during installation.
		sysDevice := func(devspec string) {
			a = append(a, "-device", devspec, "-drive", sys)
			if sq != "" {
				a = append(a, sq)
			}
		}
		switch {
		case mac:
			sysDevice(iothreadAttach(dev+",drive=SystemDisk", 0, ioth))
		case osn == "kolibrios":
			a = append(a, "-device", "ahci,id=ahci",
				"-device", "ide-hd,bus=ahci.0,drive=SystemDisk", "-drive", sys)
			if sq != "" {
				a = append(a, sq)
			}
		case osn == "batocera":
			if img := c.get("img", ""); img != "" {
				a = append(a, "-device", iothreadAttach("virtio-blk-pci,drive=BootDisk", 0, ioth),
					"-drive", "id=BootDisk,if=none,format=raw,file="+qemuOptEsc(joinConf(dir, img)))
			}
			sysDevice(iothreadAttach("virtio-blk-pci,drive=SystemDisk", 0, ioth))
		case osn == "reactos" || osn == "templeos":
			// ATA/IDE only — no -device, a plain if=ide disk.
			a = append(a, "-drive", "if=ide,index=0,media=disk,file="+qemuOptEsc(joinConf(dir, d)))
			if sq != "" {
				a = append(a, sq)
			}
		case osn == "windows-server":
			sysDevice("ide-hd,drive=SystemDisk")
		case c.get("arch", "x86_64") == "aarch64":
			sysDevice(iothreadAttach("virtio-blk-pci,drive=SystemDisk,bootindex=2", 0, ioth))
		default:
			sysDevice(iothreadAttach(dev+",drive=SystemDisk", 0, ioth))
		}
	}
	// extra_disks: optional :format suffix (default qcow2); missing
	// files warn and skip, never fatal.
	for i, ed := range confList(c, "extra_disks") {
		if ed == "" {
			continue
		}
		edu, ef := ed, "qcow2"
		if j := strings.LastIndex(ed, ":"); j >= 0 {
			edu, ef = ed[:j], ed[j+1:]
		}
		ep := joinConf(dir, edu)
		if _, err := os.Stat(ep); err != nil {
			fmt.Printf(" - WARNING! extra_disks[%d] '%s' not found; skipping.\n", i, edu)
			continue
		}
		id := fmt.Sprintf("ExtraDisk%d", i)
		a = append(a, "-device", iothreadAttach("virtio-blk-pci,drive="+id, i, iothreadCount(c)),
			"-drive", "id="+id+",if=none,format="+ef+",file="+qemuOptEsc(ep)+
				",discard=unmap,detect-zeroes=unmap,cache=writeback,aio=threads")
		if sq != "" {
			a = append(a, sq)
		}
	}
	if c.get("storage", "") == "nvme" {
		a = append(a, "-device", "nvme,drive=nvme0,serial=nvme0")
	}
	return a
}

// resolvePublic: fileshare_param_check parity — resolves PUBLIC early
// (called before arg building; the user netdev reads it for ,smb=).
func resolvePublic(c Conf) {
	pub := c.get("public_dir", "")
	if pub == "none" {
		return
	}
	if pub == "" {
		pub = xdgUserDir("PUBLICSHARE")
		if pub == "" {
			if h := os.Getenv("HOME"); h != "" {
				if _, err := os.Stat(filepath.Join(h, "Public")); err == nil {
					pub = filepath.Join(h, "Public")
				}
			}
		}
	}
	if pub == "" {
		return
	}
	if st, err := os.Stat(pub); err != nil || !st.IsDir() {
		fmt.Fprintf(os.Stderr, " - WARNING! Public directory does not exist: %s\n", pub)
		return
	}
	c["_public"] = pub
}

func fileshareArgs(c Conf, dir string) []string {
	osn := c.get("guest_os", "")
	pub := c["_public"]
	// configure_file_sharing echoes run whenever PUBLIC resolves —
	// even for windows guests (WebDAV hint still prints there).
	if pub != "" {
		switch osn {
		case "macos":
			if d := c.get("display", "none"); d == "none" || d == "spice" || d == "spice-app" {
				fmt.Printf(" - WebDAV:   On guest: build spice-webdavd (https://gitlab.gnome.org/GNOME/phodav/-/merge_requests/24)\n")
				fmt.Printf(" - WebDAV:   On guest: Finder -> Connect to Server -> http://localhost:9843/\n")
			}
		default:
			fmt.Printf(" - WebDAV:   On guest: dav://localhost:9843/\n")
		}
	}
	if pub == "" {
		return nil
	}
	// no 9p for windows (Samba via netdev instead); windows-server keeps it.
	var a []string
	if osn != "windows" {
		tag := "Public-" + strings.ToLower(os.Getenv("USER"))
		if len(tag) > 31 {
			tag = tag[:31]
		}
		// virtiofs preferred over 9p (vm_boot_fileshare): _virtiofsd is
		// set in main when the guest is linux, no ISO is attached, and
		// the daemon is running (--run) or merely resolvable (plan mode).
		if c["_virtiofsd"] == "1" && qemuHasVhostFS(c) {
			sock := qelib.SockBind(dotdir(c["_vmdir"]), qelib.SockVirtiofsd)
			fmt.Printf(" - virtiofs: On guest: sudo mount -t virtiofs %s ~/%s\n", tag, filepath.Base(pub))
			a = []string{
				"-object", "memory-backend-file,id=mem,size=" + qemuOptEsc(c.get("ram", "4G")) + ",mem-path=/dev/shm,share=on",
				"-numa", "node,memdev=mem",
				"-chardev", "socket,id=char0,path=" + qemuOptEsc(sock),
				"-device", "vhost-user-fs-pci,queue-size=1024,chardev=char0,tag=" + qemuOptEsc(tag),
			}
		} else {
			if c["_virtiofsd"] == "1" {
				// QEMU lacks the device — warn, stop the daemon, fall to 9p.
				fmt.Printf(" - WARNING! QEMU does not support vhost-user-fs-pci; falling back to 9p.\n")
				stopDaemon(c["_vmdir"], c["_vmname"], "virtiofsd")
				c["_virtiofsd"] = ""
			}
			switch osn {
			case "linux":
				fmt.Printf(" - 9P:       On guest: sudo mount -t 9p -o trans=virtio,version=9p2000.L,msize=512000 %s ~/%s\n", tag, filepath.Base(pub))
			case "macos":
				fmt.Printf(" - 9P:       On guest: sudo mount_9p %s\n", tag)
				if fi, err := os.Stat(pub); err == nil && fi.Mode().Perm() != 0o777 {
					fmt.Printf(" - 9P:       On host:  chmod 777 %s\n", pub)
					fmt.Printf("             Required for macOS integration 👆\n")
				}
			}
			a = []string{
				"-fsdev", "local,id=fsdev0,path=" + qemuOptEsc(pub) + ",security_model=mapped-xattr",
				"-device", "virtio-9p-pci,fsdev=fsdev0,mount_tag=" + qemuOptEsc(tag),
			}
		}
	}
	// QE-060/061: extra [[share]] entries + guest→host inboxes emit
	// their own 9p pairs (fsdev1+), independent of the primary share.
	a = append(a, extraShareArgs(c)...)
	// configure_file_sharing's smbd echo runs last for every guest —
	// the ,smb= option itself is already on the user netdev.
	if smbdPresent() {
		fmt.Printf(" - smbd:     On guest: smb://10.0.2.4/qemu\n")
	}
	return a
}

// qemuHasVhostFS: vm_boot_fileshare parity — probe `qemu -device help`
// for the quoted device name; an unresolvable binary keeps the
// virtiofs decision (QEMU will report its own error at spawn).
func qemuHasVhostFS(c Conf) bool {
	qemu, err := exec.LookPath("qemu-system-" + c.get("arch", "x86_64"))
	if err != nil {
		return true
	}
	out, _ := execCommand(qemu, "-device", "help").CombinedOutput()
	return strings.Contains(string(out), `"vhost-user-fs-pci"`)
}
