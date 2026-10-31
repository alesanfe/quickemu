// corpus_test.go: QE-111 round-trip corpus — real-world-shaped .conf
// files covering windows/macos/linux/BSD guests, quirks (arrays,
// quoting, exports, comments, unknown keys) and hostile lines that
// must warn but never migrate or execute.
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// roundTrip runs conf -> toml -> conf and returns the parsed docs
// plus the generated TOML text.
func roundTrip(t *testing.T, conf string) (before, after confDoc, tomlText string) {
	t.Helper()
	before = parseConf([]byte(conf))
	tomlText = confToTOML(before)
	// The generated TOML must itself be schema-clean.
	if ds := validateTOMLDoc("roundtrip.toml", []byte(tomlText)); hasErrors(ds) {
		t.Fatalf("generated TOML has schema errors: %v\n%s", ds, tomlText)
	}
	back, warns := tomlToConf([]byte(tomlText))
	for _, w := range warns {
		t.Logf("to-conf warning: %s", w)
	}
	after = parseConf([]byte(back))
	return before, after, tomlText
}

// checkRoundTrip asserts semantic equality of the flattened kv view.
func checkRoundTrip(t *testing.T, name, conf string) confDoc {
	t.Helper()
	before, after, tomlText := roundTrip(t, conf)
	bm, am := before.kv(), after.kv()
	if len(bm) != len(am) {
		t.Fatalf("%s: key count %d -> %d\nTOML:\n%s", name, len(bm), len(am), tomlText)
	}
	for k, v := range bm {
		if am[k] != v {
			t.Fatalf("%s: key %q: %q -> %q\nTOML:\n%s", name, k, v, am[k], tomlText)
		}
	}
	return before
}

var corpus = map[string]string{
	// Windows 11 as quickget emits it: shebang, tpm+secureboot tweak.
	"windows11": "#!/usr/bin/env quickemu --vm\n" +
		"guest_os=\"windows\"\n" +
		"disk_img=\"windows-11/disk.qcow2\"\n" +
		"iso=\"windows-11/Win11_23H2_English_x64.iso\"\n" +
		"disk_size=\"64G\"\n" +
		"tpm=\"on\"\nsecureboot=\"off\"\n",
	// Windows 10 with SPICE display and port forwards.
	"windows10": "guest_os=\"windows\"\n" +
		"disk_img=\"windows-10/disk.qcow2\"\n" +
		"iso=\"windows-10/win10.iso\"\n" +
		"disk_size=\"64G\"\n" +
		"display=\"spice\"\n" +
		"port_forwards=(\"3389:3389\" \"8080:80\")\n",
	// macOS Sonoma: macos_release quirk + img media + gl.
	"macos": "guest_os=\"macos\"\n" +
		"img=\"macos-sonoma/RecoveryImage.img\"\n" +
		"disk_img=\"macos-sonoma/disk.qcow2\"\n" +
		"disk_size=\"128G\"\n" +
		"macos_release=\"sonoma\"\n" +
		"cpu_cores=\"4\"\nram=\"8G\"\ngl=\"on\"\n",
	// Ubuntu 24.04 LTS desktop.
	"ubuntu": "guest_os=\"linux\"\n" +
		"disk_img=\"ubuntu-24.04/disk.qcow2\"\n" +
		"iso=\"ubuntu-24.04/ubuntu-24.04-desktop-amd64.iso\"\n" +
		"ssh_port=\"22220\"\n" +
		"public_dir=\"~/Public\"\n",
	// Arch Linux: tpm+secureboot+disk tweaks.
	"archlinux": "guest_os=\"linux\"\n" +
		"disk_img=\"archlinux-latest/disk.qcow2\"\n" +
		"iso=\"archlinux-latest/archlinux-x86_64.iso\"\n" +
		"secureboot=\"on\"\ntpm=\"on\"\ndisk_size=\"32G\"\n",
	// OpenBSD: legacy boot quirk.
	"openbsd": "guest_os=\"openbsd\"\n" +
		"disk_img=\"openbsd-7_5/disk.qcow2\"\n" +
		"iso=\"openbsd-7_5/install75.iso\"\n" +
		"boot=\"legacy\"\n",
	// FreeDOS: legacy boot, tiny RAM/disk.
	"freedos": "guest_os=\"freedos\"\n" +
		"disk_img=\"freedos-1_3/disk.qcow2\"\n" +
		"iso=\"freedos-1_3/FD13LIVE.iso\"\n" +
		"boot=\"legacy\"\ndisk_size=\"4G\"\nram=\"256M\"\n",
	// ARM64 guest (aarch64 arch key).
	"aarch64": "guest_os=\"linux\"\n" +
		"arch=\"aarch64\"\n" +
		"disk_img=\"ubuntu-24.04/disk.qcow2\"\n" +
		"iso=\"ubuntu-24.04/ubuntu-24.04-live-server-arm64.iso\"\n",
	// Networking heavy: bridge + MAC + forwards + usb passthrough.
	"netquirk": "guest_os=\"linux\"\n" +
		"disk_img=\"srv/disk.qcow2\"\n" +
		"network=\"bridge\"\nbridge=\"br0\"\n" +
		"macaddr=\"52:54:00:aa:bb:cc\"\n" +
		"port_forwards=(\"2222:22\" \"443:443\" \"8080:80\")\n" +
		"usb_devices=(\"046d:c077\" \"1050:0407\")\n",
	// Direct-interface networking + VNC display + listener.
	"vncnet": "guest_os=\"linux\"\n" +
		"disk_img=\"vm/disk.qcow2\"\n" +
		"network=\"enp3s0\"\n" +
		"display=\"vnc\"\nvnc_listen=\"0.0.0.0\"\n" +
		"width=\"1920\"\nheight=\"1080\"\n",
	// Kernel boot conf: kernel+initrd+boot_args+seed.
	"kernelboot": "guest_os=\"linux\"\n" +
		"disk_img=\"kvm/disk.qcow2\"\n" +
		"kernel=\"kvm/vmlinuz\"\ninitrd=\"kvm/initrd.img\"\n" +
		"boot_args=\"console=ttyS0 root=/dev/vda1\"\n" +
		"seed=\"kvm/seed.iso\"\n",
	// Quoting/whitespace chaos: single quotes, bare values, spaces.
	"quoting": "guest_os='linux'\n" +
		"disk_img=vm/disk.qcow2\n" +
		"ram = '4G'\n" +
		"boot_args=\"quiet splash loglevel=3\"\n" +
		"extra_args=\"-device virtio-rng-pci\"\n",
	// Unknown keys must land in [extra] verbatim.
	"extrakeys": "guest_os=\"linux\"\n" +
		"disk_img=\"x/disk.qcow2\"\n" +
		"my_custom_hook=\"/usr/local/bin/hook\"\n" +
		"zz_legacy=\"1\"\n",
	// Comment blocks travel with their keys.
	"comments": "# Primary disk layout\nguest_os=\"linux\"\n" +
		"# Installer media\niso=\"x.iso\"\n" +
		"disk_img=\"d.qcow2\" # inline note\n" +
		"# trailing comment with no key\n",
	// Foreign construct: must warn, must not migrate, must not exec.
	"nonmigratable": "guest_os=\"linux\"\n" +
		"disk_img=\"x/d.qcow2\"\n" +
		"if [ -f /etc/os-release ]; then\n" +
		"source /etc/os-release\n" +
		"echo $((1 + 2))\n",
	// Empty-ish conf: only comments.
	"onlycomments": "# nothing here\n# but comments\n",
}

func TestCorpusRoundTrip(t *testing.T) {
	for name, conf := range corpus {
		before := checkRoundTrip(t, name, conf)
		if name == "nonmigratable" && len(before.warnings) < 3 {
			t.Fatalf("nonmigratable: expected >=3 warnings, got %v", before.warnings)
		}
	}
}

func TestCorpusExtrasAndComments(t *testing.T) {
	_, _, tomlText := roundTrip(t, corpus["extrakeys"])
	if !strings.Contains(tomlText, "[extra]") ||
		!strings.Contains(tomlText, `my_custom_hook = "/usr/local/bin/hook"`) {
		t.Fatalf("extras not preserved:\n%s", tomlText)
	}
	_, _, tomlText = roundTrip(t, corpus["comments"])
	for _, want := range []string{
		"# Primary disk layout", "# Installer media",
		"disk_img = \"d.qcow2\"", "# inline note"} {
		if !strings.Contains(tomlText, want) {
			t.Fatalf("missing %q in\n%s", want, tomlText)
		}
	}
	// The shebang is a comment in TOML too — survives verbatim.
	if !strings.Contains(roundTripTOML(t, corpus["windows11"]), "#!/usr/bin/env quickemu --vm") {
		t.Fatal("shebang comment lost")
	}
}

func roundTripTOML(t *testing.T, conf string) string {
	t.Helper()
	_, _, tt := roundTrip(t, conf)
	return tt
}

func TestCorpusNeverExecutes(t *testing.T) {
	dir := t.TempDir()
	pwned := filepath.Join(dir, "pwned")
	conf := "guest_os=\"linux\"\n$(touch " + pwned + ")\n`touch " + pwned + "`\n"
	_, _, tomlText := roundTrip(t, conf)
	if _, err := os.Stat(pwned); err == nil {
		t.Fatal("EXEC-LEAK: conf parser executed a payload")
	}
	if !strings.Contains(tomlText, "guest_os") {
		t.Fatalf("lost valid keys around hostile lines:\n%s", tomlText)
	}
}

// TestRuntimeNotRoundTripped: [runtime] values never come back in the
// emitted .conf (QE-110 zone separation).
func TestRuntimeNotRoundTripped(t *testing.T) {
	tomlText := "schema_version = 1\n" +
		"guest_os = \"linux\"\n" +
		"disk_img = \"d.qcow2\"\n\n" +
		"[runtime]\npid = 4242\nspice_port = 5931\n\n" +
		"[extra]\nlegacy_hook = \"x\"\n"
	back, warns := tomlToConf([]byte(tomlText))
	for _, w := range warns {
		t.Logf("warn: %s", w)
	}
	if strings.Contains(back, "pid") || strings.Contains(back, "5931") {
		t.Fatalf("runtime leaked into conf:\n%s", back)
	}
	if !strings.Contains(back, `legacy_hook="x"`) {
		t.Fatalf("[extra] key lost:\n%s", back)
	}
}

// TestToConfValueKinds: TOML-native scalars and arrays flatten into
// conf values; non-representable values warn instead of corrupting.
func TestToConfValueKinds(t *testing.T) {
	src := "schema_version = 1\n" +
		"cpu_cores = 4\n" +
		"width = 1920\n" +
		"[extra]\n" +
		"ratio = 1.5\n" +
		"flag = true\n" +
		"ports = [5900, 5901]\n" +
		"blobs = [[1,2],[3,4]]\n"
	back, warns := tomlToConf([]byte(src))
	for _, want := range []string{`cpu_cores="4"`, `width="1920"`,
		`ratio="1.5"`, `flag="true"`, `ports=("5900" "5901")`} {
		if !strings.Contains(back, want) {
			t.Fatalf("missing %s in:\n%s", want, back)
		}
	}
	if len(warns) == 0 || !strings.Contains(warns[0]+warns[len(warns)-1]+strings.Join(warns, " "), "non-scalar") {
		t.Fatalf("nested array should warn: %v", warns)
	}
	// Malformed lines warn rather than corrupt.
	back, warns = tomlToConf([]byte("schema_version = 1\n[unclosed\ngarbage line\nk = [\n"))
	if len(warns) == 0 {
		t.Fatalf("expected warnings, got %q", back)
	}
}

// TestFromConfArrayPreserved: bash arrays become TOML arrays and
// return as bash arrays — the flat-string form survives too.
func TestFromConfArrayPreserved(t *testing.T) {
	_, _, tt := roundTrip(t, corpus["netquirk"])
	if !strings.Contains(tt, `port_forwards = ["2222:22", "443:443", "8080:80"]`) {
		t.Fatalf("array not emitted:\n%s", tt)
	}
	back, _ := tomlToConf([]byte(tt))
	if !strings.Contains(back, `port_forwards=("2222:22" "443:443" "8080:80")`) {
		t.Fatalf("array not restored:\n%s", back)
	}
}
