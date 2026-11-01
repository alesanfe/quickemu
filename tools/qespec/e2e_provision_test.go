package qespec

// e2e_provision_test.go — end-to-end coverage of the quickprovision
// pipeline over the principal VM families.
//
// Two layers:
//
//  TestProvisionPlanMatrix — ALWAYS ON and fully offline: every matrix
//  entry renders a profile, runs `quickprovision plan` and asserts the
//  resolved provider plus the step sequence the run will execute.
//  This validates the e2e contract without launching anything.
//
//  TestProvisionRunE2E — INERT unless explicitly enabled. It downloads
//  multi-GB install media, needs /dev/kvm (TCG installs are impractically
//  slow) and runs for tens of minutes per OS. Gate it on the Ubuntu/KVM
//  host:
//
//      QESPEC_PROVISION_E2E=1                     master switch
//      QESPEC_PROVISION_ONLY=ubuntu-server,nixos  optional subset
//
//  Left prepared but never run in CI/WSL — every test skips itself
//  when the gates are absent.

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// provisionCase describes one VM family to install end-to-end. The
// profile is generated (not the shipped testdata) so the matrix also
// exercises inferProvider — os/release/edition are the only inputs a
// user would give.
type provisionCase struct {
	name        string // test + profile name
	os          string
	release     string
	edition     string
	wantProvide string   // provider inferProvider must resolve
	wantSteps   []string // steps that MUST appear (subset, in order)
	interactive bool     // visual launches end at launch — no ssh/verify
	needsFw     string   // "", "efi" — windows needs efi+swtpm
	minRAM      string   // guest memory floor for the installer
	minDisk     string   // installer needs real headroom
	installMin  int      // install timeout, minutes
	sshMin      int      // ssh-wait timeout, minutes
	notes       string   // documented risk, kept with the case
}

var provisionMatrix = []provisionCase{
	{
		name: "ubuntu-server", os: "ubuntu-server", release: "24.04",
		wantProvide: "ubuntu-autoinstall",
		wantSteps:   []string{"acquire-image", "await-install", "wait-ssh", "verify", "seal"},
		installMin:  60, sshMin: 30,
		notes: "subiquity autoinstall via nocloud seed on extracted casper kernel",
	},
	{
		name: "ubuntu-desktop", os: "ubuntu", release: "24.04",
		wantProvide: "ubuntu-desktop-autoinstall",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		minRAM:      "6G", minDisk: "30G",
		installMin: 90, sshMin: 30,
		notes: "flutter installer ends with a serial marker, not poweroff",
	},
	{
		name: "debian-netinst", os: "debian", release: "latest", edition: "netinst",
		wantProvide: "debian-preseed",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  60, sshMin: 30,
		notes: "d-i preseed over slirp; netinst pulls packages from mirrors",
	},
	{
		name: "debian-cloud", os: "debian-cloud", release: "latest",
		wantProvide: "nocloud-image",
		wantSteps:   []string{"acquire-image", "prepare", "launch", "wait-ssh", "verify", "seal"},
		installMin:  30, sshMin: 20,
		notes: "genericcloud qcow2 — no install boot, cloud-init only",
	},
	{
		name: "ubuntu-cloud", os: "ubuntu-cloud", release: "24.04",
		wantProvide: "nocloud-image",
		wantSteps:   []string{"wait-ssh", "verify", "seal"},
		installMin:  30, sshMin: 20,
		notes: "kvm qcow2 — cloud-init datasource seed",
	},
	{
		name: "fedora-server", os: "fedora", release: "latest", edition: "Server",
		wantProvide: "anaconda-kickstart",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  90, sshMin: 30,
		notes: "inst.ks fetched over slirp on extracted pxeboot kernel",
	},
	{
		name: "fedora-cloud", os: "fedora-cloud", release: "latest",
		wantProvide: "nocloud-image",
		wantSteps:   []string{"wait-ssh", "verify", "seal"},
		installMin:  30, sshMin: 20,
		notes: "GenericCloud qcow2 — cloud-init",
	},
	{
		name: "rockylinux", os: "rockylinux", release: "latest", edition: "minimal",
		wantProvide: "anaconda-kickstart",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  90, sshMin: 30,
		notes: "anaconda; minimal edition still runs lorax pxeboot kernel",
	},
	{
		name: "alma-cloud", os: "alma-cloud", release: "latest",
		wantProvide: "nocloud-image",
		wantSteps:   []string{"wait-ssh", "verify", "seal"},
		installMin:  30, sshMin: 20,
		notes: "genericcloud qcow2 — cloud-init",
	},
	{
		name: "alpine", os: "alpine", release: "v3.22", edition: "virt",
		wantProvide: "alpine-unattended",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  45, sshMin: 20,
		minRAM: "1G", minDisk: "8G",
		notes: "unattended answers via kernel params; needs mkinitfs ≥3.19",
	},
	{
		name: "alpine-cloud", os: "alpine-cloud", release: "latest",
		wantProvide: "nocloud-image",
		wantSteps:   []string{"wait-ssh", "verify", "seal"},
		installMin:  20, sshMin: 15,
		notes: "alpine cloud image — cloud-init present",
	},
	{
		name: "kali", os: "kali", release: "current", edition: "netinst",
		wantProvide: "debian-preseed",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  90, sshMin: 30, minDisk: "30G",
		notes: "kali is d-i — rolling mirror keys in the preseed",
	},
	{
		name: "opensuse", os: "opensuse", release: "latest",
		wantProvide: "opensuse-autoyast",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  120, sshMin: 30, minDisk: "30G",
		notes: "autoyast= on cmdline; tumbleweed DVD is multi-GB",
	},
	{
		name: "archlinux", os: "archlinux", release: "latest",
		wantProvide: "arch-pacstrap",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  60, sshMin: 20,
		notes: "serial-driven pacstrap — archiso root shell",
	},
	{
		name: "nixos", os: "nixos", release: "latest", edition: "minimal",
		wantProvide: "nixos-serial",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  90, sshMin: 20, minDisk: "24G",
		notes: "serial-driven nixos-install with rendered configuration.nix",
	},
	{
		name: "guix", os: "guix", release: "latest",
		wantProvide: "guix-serial",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  120, sshMin: 30, minDisk: "24G",
		notes: "grub.cfg-parsed store paths; serial getty layout unverified on real ISO",
	},
	{
		name: "freebsd", os: "freebsd", release: "latest", edition: "disc1",
		wantProvide: "freebsd-shell",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  60, sshMin: 20,
		notes: "bsdinstall menu nav via serial escapes — menu order is a risk",
	},
	{
		name: "openbsd", os: "openbsd", release: "latest",
		wantProvide: "openbsd-autoinstall",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  60, sshMin: 20,
		notes: "graphics=off drops kernel to com0; (A)utoinstall answer path",
	},
	{
		name: "netbsd", os: "netbsd", release: "latest",
		wantProvide: "netbsd-sets",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  60, sshMin: 20, minRAM: "2G",
		notes: "boot-com companion ISO; sysinst lettered menu nav is a risk",
	},
	{
		name: "void", os: "void", release: "latest", edition: "base",
		wantProvide: "void-xbps",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  60, sshMin: 20,
		notes: "serial login root/voidlinux → xbps-install into /mnt",
	},
	{
		name: "gentoo", os: "gentoo", release: "latest",
		wantProvide: "gentoo-stage3",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  120, sshMin: 20, minDisk: "24G",
		notes: "stage3+getbinpkg — kernel/initrd name guesses are a risk",
	},
	{
		name: "flatcar", os: "flatcar", release: "latest",
		wantProvide: "flatcar-ignition",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  60, sshMin: 20,
		notes: "flatcar-install -i ignition.json over serial as core",
	},
	{
		name: "fedora-coreos", os: "fedora-coreos", release: "latest",
		wantProvide: "coreos-ignition",
		wantSteps:   []string{"launch", "wait-ssh", "verify", "seal"},
		installMin:  30, sshMin: 20,
		notes: "qcow2 + fw_cfg ignition — no install boot at all",
	},
	{
		name: "linuxmint", os: "linuxmint", release: "22.1", edition: "cinnamon",
		wantProvide: "ubiquity-preseed",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  90, sshMin: 30, minDisk: "25G",
		notes: "casper+automatic-ubiquity; initrd name variants are a risk",
	},
	{
		name: "windows", os: "windows", release: "11", needsFw: "efi",
		wantProvide: "windows-autounattend",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  120, sshMin: 45, minRAM: "6G", minDisk: "64G",
		notes: "autounattend.iso on a second cdrom; TPM2+EFI required",
	},
	{
		name: "windows-server", os: "windows-server", release: "2022", needsFw: "efi",
		wantProvide: "windows-autounattend",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  120, sshMin: 45, minRAM: "4G", minDisk: "64G",
		notes: "same unattend path; ssh comes from provisioned openssh",
	},
	{
		name: "proxmox-ve", os: "proxmox-ve", release: "latest", needsFw: "efi",
		wantProvide: "proxmox-autoinstall",
		wantSteps:   []string{"await-install", "wait-ssh", "verify", "seal"},
		installMin:  90, sshMin: 30, minRAM: "4G", minDisk: "32G",
		notes: "answer.toml on the fetched ISO via qemu-img embedding; webhook completion",
	},
	{
		name: "macos", os: "macos", release: "sonoma",
		wantProvide: "visual", interactive: true,
		wantSteps:  []string{"acquire-image", "prepare", "launch"},
		installMin: 10,
		notes:      "recoveryOS is interactive — run ends at launch by design",
	},
}

// ---------- always-on, offline: the matrix must resolve ----------

func TestProvisionPlanMatrix(t *testing.T) {
	bin := Bin(t, "quickprovision")
	for _, tc := range provisionMatrix {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			prof := writeProvisionProfile(t, tc)
			r := RunArgv(t, bin, nil, "plan", "--profile", prof)
			env := r.Envelope(t)
			if ok, _ := env["success"].(bool); !ok {
				t.Fatalf("plan failed: %s", r.Stdout)
			}
			data, _ := env["data"].(map[string]any)
			if got := data["provider"]; got != tc.wantProvide {
				t.Fatalf("provider=%v want %s", got, tc.wantProvide)
			}
			steps, _ := data["steps"].([]any)
			var ids []string
			for _, s := range steps {
				ids = append(ids, fmt.Sprint(s))
			}
			assertStepSubset(t, ids, tc.wantSteps)
			if tc.interactive && len(ids) != 4 {
				t.Fatalf("visual pipeline should be 4 steps, got %v", ids)
			}
		})
	}
}

// assertStepSubset requires want to appear in got in order.
func assertStepSubset(t *testing.T, got, want []string) {
	t.Helper()
	i := 0
	for _, g := range got {
		if i < len(want) && g == want[i] {
			i++
		}
	}
	if i != len(want) {
		t.Fatalf("steps %v missing ordered subset %v", got, want)
	}
}

// writeProvisionProfile renders the minimal profile a user would give —
// provider left unset so inference is exercised per case.
func writeProvisionProfile(t *testing.T, tc provisionCase) string {
	t.Helper()
	ram, disk := tc.minRAM, tc.minDisk
	if ram == "" {
		ram = "4G"
	}
	if disk == "" {
		disk = "20G"
	}
	install, ssh := tc.installMin, tc.sshMin
	if install == 0 {
		install = 90
	}
	if ssh == 0 {
		ssh = 30
	}
	var b strings.Builder
	fmt.Fprintf(&b, "[profile]\nname = %q\n", tc.name)
	fmt.Fprintf(&b, "[system]\nos = %q\n", tc.os)
	if tc.release != "" {
		fmt.Fprintf(&b, "release = %q\n", tc.release)
	}
	if tc.edition != "" {
		fmt.Fprintf(&b, "edition = %q\n", tc.edition)
	}
	fmt.Fprintf(&b, "arch = \"amd64\"\n")
	fmt.Fprintf(&b, "[machine]\ncpus = 2\nram = %q\ndisk_size = %q\n", ram, disk)
	if tc.needsFw == "efi" {
		b.WriteString("boot = \"efi\"\n")
	}
	fmt.Fprintf(&b, "[identity]\nhostname = %q\nusername = \"dev\"\n",
		strings.ReplaceAll(tc.name, "_", "-"))
	b.WriteString("[automation]\n")
	fmt.Fprintf(&b, "install_timeout_min = %d\nssh_timeout_min = %d\n", install, ssh)
	b.WriteString("[[verify]]\nname = \"ssh-alive\"\nrun = \"true\"\n")
	b.WriteString("[finalize]\nshutdown = true\nsnapshot = \"prepared\"\n")
	dir := t.TempDir()
	f := filepath.Join(dir, "profile.toml")
	if err := os.WriteFile(f, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return f
}

// ---------- gated: real downloads + KVM boots — prepared, not run ----

func provisionE2EEnabled(t *testing.T) {
	t.Helper()
	if os.Getenv("QESPEC_PROVISION_E2E") != "1" {
		t.Skip("provision e2e inert — set QESPEC_PROVISION_E2E=1 on a KVM host")
	}
	if _, err := os.Stat("/dev/kvm"); err != nil {
		t.Skip("/dev/kvm absent — TCG installs are impractically slow")
	}
	if f, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0); err != nil {
		f.Close()
		t.Skip("/dev/kvm not writable")
	} else {
		f.Close()
	}
	// WAN probe: downloads are multi-GB; fail fast instead of hanging.
	c, err := net.DialTimeout("tcp", "deb.debian.org:443", 5*time.Second)
	if err != nil {
		t.Skip("no outbound network")
	}
	c.Close()
}

func provisionOnly() map[string]bool {
	only := map[string]bool{}
	for _, s := range strings.Split(os.Getenv("QESPEC_PROVISION_ONLY"), ",") {
		if s = strings.TrimSpace(s); s != "" {
			only[s] = true
		}
	}
	return only
}

func TestProvisionRunE2E(t *testing.T) {
	provisionE2EEnabled(t)
	bin := Bin(t, "quickprovision")
	only := provisionOnly()
	for _, tc := range provisionMatrix {
		tc := tc
		if len(only) > 0 && !only[tc.name] {
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			work := t.TempDir()
			prof := writeProvisionProfile(t, tc)

			// 1. plan — the run must see the same provider the matrix asserts.
			r := RunArgv(t, bin, nil, "plan", "--profile", prof)
			data, _ := r.Envelope(t)["data"].(map[string]any)
			if data["provider"] != tc.wantProvide {
				t.Fatalf("provider=%v want %s", data["provider"], tc.wantProvide)
			}

			// 2. run — JSONL events on stdout; the runner is kill-safe
			// on timeout so a stuck install never orphans a QEMU.
			r = RunArgv(t, bin, []RunOpt{
				InDir(work),
				Timeout(time.Duration(tc.installMin+tc.sshMin+30) * time.Minute),
			}, "run", "--profile", prof, "--dir", work)
			evs := parseRunEvents(r.Stdout)

			// 3. the step sequence must contain the ordered contract.
			var done []string
			for _, e := range evs {
				if e["event"] == "step.completed" {
					done = append(done, eventStep(e))
				}
			}
			assertStepSubset(t, done, tc.wantSteps)
			if tc.interactive {
				// visual launches complete the run at launch — no ssh.
				if evs[len(evs)-1]["event"] != "run.completed" {
					t.Fatalf("visual run ended on %v", evs[len(evs)-1])
				}
				return
			}

			// 4. verify results all passed; run completed.
			for _, e := range evs {
				if e["event"] != "verify.result" {
					continue
				}
				if pass, _ := eventData(e)["pass"].(bool); !pass {
					t.Errorf("verify %v failed: %v", eventData(e)["name"], e)
				}
			}
			if last := evs[len(evs)-1]; last["event"] != "run.completed" {
				lines := strings.Split(strings.TrimRight(r.Stdout, "\n"), "\n")
				if len(lines) > 8 {
					lines = lines[len(lines)-8:]
				}
				t.Fatalf("run ended on %v — tail: %s", last, strings.Join(lines, "\n"))
			}

			// 5. the sealed snapshot must be visible to quickemu.
			confs, _ := filepath.Glob(filepath.Join(work, "*", "*.conf"))
			if len(confs) == 0 {
				t.Fatalf("no VM conf under %s", work)
			}
			Run(t, "quickemu", nil, "--vm", confs[0], "--snapshot-list").
				Contains(t, "prepared")

			// 6. cleanup unless --keep-debug is wanted post-mortem.
			if os.Getenv("QESPEC_PROVISION_KEEP") != "1" {
				Run(t, "quickemu", nil, "--vm", confs[0], "--delete-vm", "--trash")
			}
		})
	}
}

// parseRunEvents decodes the run's JSONL stream (one event per line).
// Records look like {"seq":n,"event":"step.completed","data":{…}}.
func parseRunEvents(out string) []map[string]any {
	var evs []map[string]any
	for _, l := range strings.Split(out, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || !strings.HasPrefix(l, "{") {
			continue
		}
		var e map[string]any
		if json.Unmarshal([]byte(l), &e) == nil {
			evs = append(evs, e)
		}
	}
	return evs
}

// eventData digs the data map out of a parsed event record.
func eventData(e map[string]any) map[string]any {
	d, _ := e["data"].(map[string]any)
	return d
}

// eventStep is the step id of a step.* event ("" for anything else).
func eventStep(e map[string]any) string {
	s, _ := eventData(e)["step"].(string)
	return s
}
