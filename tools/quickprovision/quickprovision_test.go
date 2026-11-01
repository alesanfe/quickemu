// quickprovision_test.go — offline coverage of the quickprovision engine:
// conf helpers, profile loading/defaults, providers (user-data
// emission + conf edits), seed ISO build, ISO bootfile extraction,
// keypair/shell quoting, wait loops, the event emitter, and the
// CLI dispatch surface. Steps needing a live VM/SSH server
// (launch/provision/verify/run) are integration-only by design.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alesanfe/quickemu/tools/qelib"
)

// --- helpers.go ---------------------------------------------------------

func TestTrimLine(t *testing.T) {
	if trimLine("  x \n") != "x" {
		t.Fatal("trim")
	}
}

func TestPidAlive(t *testing.T) {
	if pidAlive(0) || pidAlive(-1) {
		t.Fatal("non-positive pid must be dead")
	}
	if !pidAlive(os.Getpid()) {
		t.Fatal("self pid must be alive")
	}
}

func TestByteReader(t *testing.T) {
	r := NewBytesReader([]byte("abc"))
	b, err := io.ReadAll(r)
	if err != nil || string(b) != "abc" {
		t.Fatalf("read %q %v", b, err)
	}
	if n, err := r.Read(make([]byte, 4)); n != 0 || err != io.EOF {
		t.Fatal("exhausted reader must EOF")
	}
	if _, err := NewBytesReader(nil).Read(make([]byte, 4)); err != io.EOF {
		t.Fatal("empty reader must EOF")
	}
}

func TestConfSetGetDrop(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "vm.conf")
	os.WriteFile(conf, []byte("guest_os=\"linux\"\nram=\"2G\"\niso=\"a.iso\"\n"), 0o600)

	if confGet(conf, "ram") != "2G" || confGet(conf, "missing") != "" {
		t.Fatal("confGet")
	}
	if confGet(filepath.Join(dir, "none.conf"), "x") != "" {
		t.Fatal("confGet unreadable must be empty")
	}

	if err := confSet(conf, map[string]string{
		"ram": "4G", "boot": "legacy", "iso": ""}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(conf)
	s := string(b)
	if !strings.Contains(s, `ram="4G"`) || !strings.Contains(s, `boot="legacy"`) {
		t.Fatalf("confSet %s", s)
	}
	// iso="" means delete — and the original iso line is replaced, not dup'd.
	if strings.Contains(s, "iso=") {
		t.Fatalf("iso must be dropped: %s", s)
	}
	if strings.Count(s, `ram=`) != 1 {
		t.Fatalf("ram duplicated: %s", s)
	}
	if confGet(conf, "iso") != "" || confGet(conf, "boot") != "legacy" {
		t.Fatal("post-set read")
	}

	if err := confDrop(conf, "boot", "guest_os"); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(conf)
	if strings.Contains(string(b), "boot=") || strings.Contains(string(b), "guest_os=") {
		t.Fatalf("confDrop %s", b)
	}
	if confSet(filepath.Join(dir, "none.conf"), map[string]string{"a": "b"}) == nil {
		t.Fatal("missing conf must fail")
	}
	if confDrop(filepath.Join(dir, "none.conf"), "a") == nil {
		t.Fatal("missing conf must fail")
	}
}

func TestDeriveVMDir(t *testing.T) {
	dir := t.TempDir()
	// disk_img points into a subdir → vmdir is where the disk lives.
	vmSub := filepath.Join(dir, "myvm")
	os.MkdirAll(vmSub, 0o755)
	conf := filepath.Join(dir, "myvm.conf")
	os.WriteFile(conf, []byte("disk_img=\"myvm/disk.qcow2\"\n"), 0o600)
	if got := deriveVMDir(conf); got != vmSub {
		t.Fatalf("disk_img dir %s want %s", got, vmSub)
	}
	// absolute disk path wins.
	conf2 := filepath.Join(dir, "abs.conf")
	os.WriteFile(conf2, []byte("disk_img=\"/elsewhere/d.qcow2\"\n"), 0o600)
	if got := deriveVMDir(conf2); got != "/elsewhere" {
		t.Fatalf("abs %s", got)
	}
	// no disk keys: existing <confdir>/<name> dir is the vmdir.
	conf3 := filepath.Join(dir, "myvm2.conf")
	os.MkdirAll(filepath.Join(dir, "myvm2"), 0o755)
	os.WriteFile(conf3, []byte("guest_os=\"linux\"\n"), 0o600)
	if got := deriveVMDir(conf3); got != filepath.Join(dir, "myvm2") {
		t.Fatalf("sibling dir %s", got)
	}
	// nothing at all → the conf's own dir.
	conf4 := filepath.Join(dir, "lone.conf")
	os.WriteFile(conf4, []byte("guest_os=\"linux\"\n"), 0o600)
	if got := deriveVMDir(conf4); got != dir {
		t.Fatalf("confdir %s", got)
	}
}

func TestResolveConfPath(t *testing.T) {
	if resolveConfPath("/d/v.conf", "/abs/x") != "/abs/x" {
		t.Fatal("abs passthrough")
	}
	if got := resolveConfPath("/d/v.conf", "sub/f.iso"); got != filepath.Join("/d", "sub", "f.iso") {
		t.Fatalf("rel %s", got)
	}
}

func TestFindVMConf(t *testing.T) {
	dir := t.TempDir()
	if _, _, _, err := findVMConf(dir); err == nil {
		t.Fatal("empty dir must fail")
	}
	// flat layout: <dir>/os-rel.conf — vmdir falls back to confdir.
	conf := filepath.Join(dir, "cirros-0.6.conf")
	os.WriteFile(conf, []byte("guest_os=\"linux\"\n"), 0o600)
	cp, vd, vn, err := findVMConf(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cp != conf || vn != "cirros-0.6" || vd != dir {
		t.Fatalf("flat %s %s %s", cp, vd, vn)
	}
	// nested layout wins when newer: <dir>/<name>/<name>.conf.
	nested := filepath.Join(dir, "nested-1")
	os.MkdirAll(nested, 0o755)
	nc := filepath.Join(nested, "nested-1.conf")
	os.WriteFile(nc, []byte("disk_img=\"disk.qcow2\"\n"), 0o600)
	os.Chtimes(nc, time.Now(), time.Now().Add(time.Hour))
	cp, vd, vn, err = findVMConf(dir)
	if err != nil || cp != nc || vn != "nested-1" || vd != nested {
		t.Fatalf("nested %s %s %s %v", cp, vd, vn, err)
	}
}

// --- profile.go -----------------------------------------------------------

func writeProfile(t *testing.T, body string) string {
	p := filepath.Join(t.TempDir(), "p.toml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadProfileDefaults(t *testing.T) {
	p, err := loadProfile(writeProfile(t, `
[system]
os = "cirros"
release = "0.6"
`))
	if err != nil {
		t.Fatal(err)
	}
	if p.System.Arch != "amd64" || p.Profile.Name != "cirros-0.6" ||
		p.Identity.Username != "quickemu" || p.Machine.CPUs != 2 ||
		p.Machine.RAM != "4G" || p.Automation.InstallTimeoutMin != 45 ||
		p.Automation.SSHTimeoutMin != 15 {
		t.Fatalf("defaults %+v", p)
	}
	if p.Identity.Hostname != "cirros-0-6" {
		t.Fatalf("hostname %q", p.Identity.Hostname)
	}
}

func TestLoadProfileRequired(t *testing.T) {
	if _, err := loadProfile(writeProfile(t, `[system]
release = "1"`)); err == nil || !strings.Contains(err.Error(), "system.os") {
		t.Fatalf("os required %v", err)
	}
	if _, err := loadProfile(writeProfile(t, `[system]
os = "x"`)); err == nil || !strings.Contains(err.Error(), "release") {
		t.Fatalf("release required %v", err)
	}
	if _, err := loadProfile(filepath.Join(t.TempDir(), "none.toml")); err == nil {
		t.Fatal("missing file must fail")
	}
	if _, err := loadProfile(writeProfile(t, "not toml [[[")); err == nil {
		t.Fatal("bad toml must fail")
	}
}

func TestLoadProfileExplicit(t *testing.T) {
	p, err := loadProfile(writeProfile(t, `
[profile]
name = "My Run"
[system]
os = "ubuntu-server"
release = "24.04"
arch = "arm64"
[machine]
cpus = 4
ram = "8G"
ssh_port = 2222
boot = "efi"
[identity]
hostname = "h1"
username = "me"
password = "pw"
ssh_authorized_keys = ["ssh-ed25519 AAAA k1"]
allow_pw_auth = true
[locale]
lang = "en_US.UTF-8"
keyboard = "us"
timezone = "UTC"
[automation]
provider = "ubuntu-autoinstall"
install_timeout_min = 60
ssh_timeout_min = 30
[provision]
packages = ["htop"]
services = ["nginx"]
[[provision.files]]
source = "f.txt"
destination = "/etc/f.txt"
[[provision.commands]]
name = "c1"
run = "true"
as = "root"
[[verify]]
name = "v1"
run = "uname"
expect = "Linux"
[finalize]
shutdown = true
snapshot = "snap1"
`))
	if err != nil {
		t.Fatal(err)
	}
	if p.Profile.Name != "My Run" || p.System.Arch != "arm64" ||
		p.Machine.CPUs != 4 || p.Machine.SSHPort != 2222 ||
		p.Identity.Hostname != "h1" || p.Automation.Provider != "ubuntu-autoinstall" ||
		p.Automation.InstallTimeoutMin != 60 ||
		len(p.Provision.Packages) != 1 || len(p.Provision.Files) != 1 ||
		len(p.Provision.Commands) != 1 || len(p.Verify) != 1 ||
		!p.Finalize.Shutdown || p.Finalize.Snapshot != "snap1" {
		t.Fatalf("explicit %+v", p)
	}
}

func TestSafeHostname(t *testing.T) {
	cases := map[string]string{
		"My_VM Name": "my-vm-name",
		"..dot..":    "dot",
		"A_1 b":      "a-1-b",
		"--x--":      "x",
	}
	for in, want := range cases {
		if got := safeHostname(in); got != want {
			t.Fatalf("safeHostname(%q)=%q want %q", in, got, want)
		}
	}
}

func TestCatalogImageType(t *testing.T) {
	dir := t.TempDir()
	cat := filepath.Join(dir, "catalog", "data")
	os.MkdirAll(cat, 0o755)
	os.WriteFile(filepath.Join(cat, "catalog.json"), []byte(
		`{"entries":[{"os":"fakeos","image_type":"img"},{"os":"other","image_type":"iso"}]}`), 0o600)
	t.Setenv("QUICKEMU_DATA_DIR", dir)
	if catalogImageType("fakeos") != "img" {
		t.Fatal("catalog lookup")
	}
	if catalogImageType("absent") != "" {
		t.Fatal("absent os must be empty")
	}
	// provider inference rides on the same catalog.
	var p Profile
	p.System.OS = "ubuntu-server"
	if inferProvider(&p) != "ubuntu-autoinstall" {
		t.Fatal("ubuntu-server is always autoinstall")
	}
	p.System.OS = "fakeos-cloud"
	if inferProvider(&p) != "nocloud-image" {
		t.Fatal("*-cloud name → nocloud-image")
	}
	p.System.OS = "fakeos"
	if inferProvider(&p) != "visual" {
		t.Fatal("img type without cloud-init → visual (not nocloud)")
	}
	p.System.OS = "other"
	if inferProvider(&p) != "visual" {
		t.Fatal("iso type → visual")
	}
	t.Setenv("QUICKEMU_DATA_DIR", filepath.Join(dir, "empty"))
	p.System.OS = "x"
	if catalogImageType("fakeos") != "" || inferProvider(&p) != "visual" {
		t.Fatal("unreadable catalog → empty/visual")
	}
}

// --- provider.go ----------------------------------------------------------

func TestPickProvider(t *testing.T) {
	var p Profile
	p.Automation.Provider = "ubuntu-autoinstall"
	if prov, err := pickProvider(&p); err != nil || prov.id() != "ubuntu-autoinstall" {
		t.Fatal("autoinstall")
	}
	p.Automation.Provider = "nocloud-image"
	if prov, err := pickProvider(&p); err != nil || prov.id() != "nocloud-image" {
		t.Fatal("nocloud")
	}
	p.Automation.Provider = "manual"
	if _, err := pickProvider(&p); err == nil {
		t.Fatal("manual must fail")
	}
	p.Automation.Provider = "bogus"
	if _, err := pickProvider(&p); err == nil {
		t.Fatal("unknown must fail")
	}
}

func TestYamlQ(t *testing.T) {
	if yamlQ(`a"b\c`) != `"a\"b\\c"` {
		t.Fatal("yaml quoting")
	}
}

func TestCryptSaltPwHash(t *testing.T) {
	s := qelib.CryptSalt(8)
	if len(s) != 8 {
		t.Fatalf("salt len %d", len(s))
	}
	var p Profile
	p.Identity.Password = "secret"
	h, err := pwHash(&p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$6$") {
		t.Fatalf("sha512-crypt %s", h)
	}
	// empty password → random-secret locked hash, still valid crypt.
	p.Identity.Password = ""
	if h, err = pwHash(&p); err != nil || !strings.HasPrefix(h, "$6$") {
		t.Fatalf("empty pw %s %v", h, err)
	}
}

func testRunCtx(t *testing.T) (*runCtx, string) {
	dir := t.TempDir()
	kp, err := genKeypair(filepath.Join(dir, "art"))
	if err != nil {
		t.Fatal(err)
	}
	var p Profile
	p.Identity.Hostname = "h1"
	p.Identity.Username = "u1"
	p.Machine.RAM = "4G"
	p.Machine.CPUs = 2
	p.Automation.InstallTimeoutMin = 1
	p.Automation.SSHTimeoutMin = 1
	conf := filepath.Join(dir, "vm.conf")
	os.WriteFile(conf, []byte("guest_os=\"linux\"\n"), 0o600)
	r := &runCtx{p: &p, prov: &noCloudImage{}, kp: kp, confPath: conf,
		vmDir: dir, vmName: "vm", runID: "run-1", sshPort: 2222}
	return r, dir
}

func TestBaseMetaData(t *testing.T) {
	r, _ := testRunCtx(t)
	md := baseMetaData(r.p, r)
	for _, want := range []string{`"instance-id":"run-1"`, `"local-hostname":"h1"`, `"public-keys"`, "ssh-ed25519"} {
		if !strings.Contains(md, want) {
			t.Fatalf("meta-data missing %s:\n%s", want, md)
		}
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(md), &parsed); err != nil {
		t.Fatalf("meta-data must be JSON for cirros-init: %v", err)
	}
}

func TestAuthKeysYAML(t *testing.T) {
	r, _ := testRunCtx(t)
	r.p.Identity.SSHAuthorizedKeys = []string{"ssh-ed25519 PROFILE"}
	y := authKeysYAML(r.p, r.kp)
	if !strings.Contains(y, "ssh-ed25519 PROFILE") || !strings.Contains(y, "ssh-ed25519") {
		t.Fatalf("keys %q", y)
	}
	if strings.Count(y, "- ssh-") != 2 {
		t.Fatalf("two keys expected: %q", y)
	}
}

func TestCommonConfEdits(t *testing.T) {
	r, dir := testRunCtx(t)
	m := commonConfEdits(r)
	if m["display"] != "none" || m["monitor"] != "socket" ||
		m["serial"] != "socket" || m["ram"] != "4G" ||
		m["cpu_cores"] != "2" || m["port_forwards"] != "2222:22" ||
		m["seed"] != filepath.Join(dir, "seed.iso") {
		t.Fatalf("edits %v", m)
	}
	if _, ok := m["boot"]; ok {
		t.Fatal("empty boot must not be written")
	}
	r.p.Machine.Boot = "efi"
	if commonConfEdits(r)["boot"] != "efi" {
		t.Fatal("boot override")
	}
}

func TestProviderMeta(t *testing.T) {
	u := &ubuntuAutoinstall{}
	if u.id() != "ubuntu-autoinstall" || !u.needsSecondBoot() {
		t.Fatal("ubuntu provider meta")
	}
	if got := u.dropConfKeys(); len(got) != 6 || got[0] != "kernel" {
		t.Fatalf("drop keys %v", got)
	}
	var p Profile
	p.Identity.Username = "u"
	if u.sshUser(&p) != "u" {
		t.Fatal("sshUser")
	}
	n := &noCloudImage{}
	if n.id() != "nocloud-image" || n.needsSecondBoot() ||
		len(n.dropConfKeys()) != 1 || n.dropConfKeys()[0] != "seed" ||
		n.sshUser(&p) != "u" {
		t.Fatal("nocloud provider meta")
	}
}

// --- seed.go + iso.go -------------------------------------------------------

func TestBuildSeed(t *testing.T) {
	dir := t.TempDir()
	seed, err := buildSeed(dir, "#cloud-config\nhostname: h", "instance-id: i", "vend")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(seed) != "seed.iso" {
		t.Fatalf("seed %s", seed)
	}
	st, err := os.Stat(seed)
	if err != nil || st.Size() == 0 {
		t.Fatal("seed.iso not written")
	}
	for _, f := range []string{"user-data", "meta-data", "vendor-data"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Fatalf("%s missing", f)
		}
	}
	// rebuild over an existing seed must not fail (diskfs refuses overwrite).
	if _, err := buildSeed(dir, "x", "y", ""); err != nil {
		t.Fatalf("rebuild %v", err)
	}
	// vendor-data omitted on the second build — only user/meta required.
}

// makeTestISO builds a real ISO9660 image with the given files via
// the same diskfs path writeSeedISO uses.
func makeTestISO(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	mapped := map[string]string{}
	for inIso, content := range files {
		src := filepath.Join(dir, "src"+strings.ReplaceAll(inIso, "/", "_"))
		if err := os.WriteFile(src, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		mapped[inIso] = src
	}
	out := filepath.Join(dir, "test.iso")
	if err := writeSeedISO(out, mapped); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestWriteSeedISOMissingSrc(t *testing.T) {
	dir := t.TempDir()
	// A missing source file aborts the build with the read error.
	out := filepath.Join(dir, "s.iso")
	if err := writeSeedISO(out, map[string]string{
		"/user-data": filepath.Join(dir, "ghost")}); err == nil {
		t.Fatal("missing source must fail")
	}
}

func TestExtractBootfiles(t *testing.T) {
	iso := makeTestISO(t, map[string]string{
		"/casper/vmlinuz": "kernel-bytes",
		"/casper/initrd":  "initrd-bytes",
		"/casper/other":   "skip-me",
	})
	out := t.TempDir()
	got, err := extractBootfiles(iso, "casper", out, []string{"vmlinuz", "initrd"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("extracted %v", got)
	}
	b, _ := os.ReadFile(got["vmlinuz"])
	if string(b) != "kernel-bytes" {
		t.Fatalf("vmlinuz %q", b)
	}
	if _, err := os.Stat(filepath.Join(out, "other")); !os.IsNotExist(err) {
		t.Fatal("unwanted file must not be extracted")
	}
	// missing wanted file → error naming it.
	if _, err := extractBootfiles(iso, "casper", out, []string{"vmlinuz", "nope"}); err == nil ||
		!strings.Contains(err.Error(), "nope") {
		t.Fatalf("missing want %v", err)
	}
	// bad ISO path / non-ISO content.
	if _, err := extractBootfiles(iso+".none", "casper", out, nil); err == nil {
		t.Fatal("missing iso must fail")
	}
	junk := filepath.Join(t.TempDir(), "junk.iso")
	os.WriteFile(junk, []byte("not an iso"), 0o600)
	if _, err := extractBootfiles(junk, "casper", out, nil); err == nil {
		t.Fatal("bad image must fail")
	}
}

// --- ssh.go -----------------------------------------------------------------

func TestGenKeypair(t *testing.T) {
	k, err := genKeypair("")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(k.pub, "ssh-ed25519") || k.signer == nil ||
		k.privPath != "" {
		t.Fatalf("keypair %+v", k)
	}
	art := t.TempDir()
	k2, err := genKeypair(art)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"id_ed25519", "id_ed25519.pub"} {
		st, err := os.Stat(filepath.Join(art, f))
		if err != nil || st.Size() == 0 {
			t.Fatalf("%s missing", f)
		}
	}
	b, _ := os.ReadFile(k2.privPath)
	if !strings.Contains(string(b), "PRIVATE KEY") {
		t.Fatal("pem not written")
	}
	if k.pub == k2.pub {
		t.Fatal("keypairs must be unique")
	}
}

func TestShellQ(t *testing.T) {
	if shellQ("abc") != "'abc'" {
		t.Fatal("plain")
	}
	if shellQ("a'b") != `'a'\''b'` {
		t.Fatalf("quote %s", shellQ("a'b"))
	}
	if shellQ("") != "''" {
		t.Fatal("empty")
	}
}

func TestSSHDialRefused(t *testing.T) {
	k, _ := genKeypair("")
	port, _ := freePort()
	if _, err := sshDial(port, "u", k.signer); err == nil {
		t.Fatal("closed port must fail")
	}
}

// --- wait.go ------------------------------------------------------------------

func TestFreePort(t *testing.T) {
	p, err := freePort()
	if err != nil || p <= 0 {
		t.Fatalf("port %d %v", p, err)
	}
}

func TestWaitPIDGone(t *testing.T) {
	dir := t.TempDir()
	e := newEmitter("r", "", "", "", true)
	ctx := context.Background()
	// no pidfile → immediately gone.
	if err := waitPIDGone(ctx, e, dir, "vm", time.Second); err != nil {
		t.Fatal(err)
	}
	// dead pid → gone.
	os.WriteFile(filepath.Join(dir, "vm.pid"), []byte("99999999"), 0o600)
	if err := waitPIDGone(ctx, e, dir, "vm", time.Second); err != nil {
		t.Fatal(err)
	}
	// alive pid (self) → still running → times out.
	os.WriteFile(filepath.Join(dir, "vm.pid"), []byte(fmt.Sprint(os.Getpid())), 0o600)
	if err := waitPIDGone(ctx, e, dir, "vm", 100*time.Millisecond); err == nil {
		t.Fatal("alive pid must time out")
	}
	// cancel path.
	c2, cancel := context.WithCancel(ctx)
	cancel()
	if err := waitPIDGone(c2, e, dir, "vm", time.Minute); err != context.Canceled {
		t.Fatalf("cancel %v", err)
	}
}

func TestWaitSSHTimeoutAndCancel(t *testing.T) {
	e := newEmitter("r", "", "", "", true)
	k, _ := genKeypair("")
	port, _ := freePort() // closed port — dial refused instantly
	ctx := context.Background()
	if err := waitSSH(ctx, e, port, "u", k, 50*time.Millisecond); err == nil ||
		!strings.Contains(err.Error(), "not reachable") {
		t.Fatalf("timeout %v", err)
	}
	c2, cancel := context.WithCancel(ctx)
	cancel()
	if err := waitSSH(c2, e, port, "u", k, time.Minute); err != context.Canceled {
		t.Fatalf("cancel %v", err)
	}
}

func TestTailSerial(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "s.sock")
	log := filepath.Join(dir, "serial.log")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Skip("no unix sockets")
	}
	defer l.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e := newEmitter("r", "", "", "", true)
	done := make(chan struct{})
	go func() { tailSerial(ctx, sock, log, e, &serialTap{}); close(done) }()
	go func() {
		c, err := l.Accept()
		if err == nil {
			c.Write([]byte("serial-bytes"))
			c.Close()
		}
	}()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if b, _ := os.ReadFile(log); strings.Contains(string(b), "serial-bytes") {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("serial bytes never logged")
}

// --- events.go ------------------------------------------------------------------

func TestEmitter(t *testing.T) {
	dir := t.TempDir()
	art := filepath.Join(dir, "art")
	os.MkdirAll(art, 0o755)
	e := newEmitter("run-1", "vm1", dir, art, true)
	e.emit("run.started", map[string]any{"k": 1})
	e.progress("s", "msg")
	if err := e.step("s1", 1, 2, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := e.step("s2", 2, 2, func() error { return fmt.Errorf("boom") }); err == nil {
		t.Fatal("failed step must return error")
	}
	e.close()
	b, err := os.ReadFile(filepath.Join(art, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 6 {
		t.Fatalf("events %v", lines)
	}
	var first, last map[string]any
	json.Unmarshal([]byte(lines[0]), &first)
	json.Unmarshal([]byte(lines[5]), &last)
	if first["seq"].(float64) != 1 || first["event"] != "run.started" || first["run"] != "run-1" {
		t.Fatalf("rec %v", first)
	}
	if last["event"] != "step.failed" || last["seq"].(float64) != 6 {
		t.Fatalf("last %v", last)
	}
	// mirrored into the VM event log too.
	if b, _ := os.ReadFile(filepath.Join(dir, "vm1.events.jsonl")); !strings.Contains(string(b), "run.started") {
		t.Fatal("vm event log not mirrored")
	}
	// quiet emitter drops stdout, keeps artifact.
	e2 := newEmitter("r2", "vm2", dir, art, true)
	e2.emit("x", nil)
	e2.close()
}

func TestOpCancelled(t *testing.T) {
	dir := t.TempDir()
	if opCancelled(dir, "nope") {
		t.Fatal("missing entry must not cancel")
	}
	ops := filepath.Join(dir, "ops")
	os.MkdirAll(ops, 0o755)
	os.WriteFile(filepath.Join(ops, "o1.json"), []byte(`{"state":"running"}`), 0o600)
	if opCancelled(dir, "o1") {
		t.Fatal("running must not cancel")
	}
	for _, st := range []string{"cancelRequested", "cancelled"} {
		os.WriteFile(filepath.Join(ops, "o2.json"), []byte(`{"state":"`+st+`"}`), 0o600)
		if !opCancelled(dir, "o2") {
			t.Fatalf("%s must cancel", st)
		}
	}
	os.WriteFile(filepath.Join(ops, "o3.json"), []byte("not json"), 0o600)
	if opCancelled(dir, "o3") {
		t.Fatal("bad json must not cancel")
	}
}

func TestNewRunID(t *testing.T) {
	id := newRunID()
	if len(id) != 15+1+8 {
		t.Fatalf("run id %q", id)
	}
}

// --- runner.go pure surface ------------------------------------------------

func TestPlanSteps(t *testing.T) {
	if got := planSteps(&ubuntuAutoinstall{}); len(got) != 12 ||
		got[3] != "launch-install" || got[11] != "seal" {
		t.Fatalf("two-boot steps %v", got)
	}
	if got := planSteps(&noCloudImage{}); len(got) != 9 || got[3] != "launch" {
		t.Fatalf("one-boot steps %v", got)
	}
}

func TestPkgInstallCmd(t *testing.T) {
	for pm, want := range map[string]string{
		"apt":    "install -y",
		"apk":    "apk add",
		"dnf":    "dnf install -y",
		"zypper": "zypper --non-interactive install",
		"pacman": "pacman -S --noconfirm",
	} {
		cmd, err := pkgInstallCmd(pm, "htop", "sudo ")
		if err != nil || !strings.Contains(cmd, want) || !strings.Contains(cmd, "'htop'") {
			t.Fatalf("%s → %s %v", pm, cmd, err)
		}
	}
	cmd, _ := pkgInstallCmd("apt", "htop", "sudo ")
	if !strings.Contains(cmd, "DPkg::Lock::Timeout") {
		t.Fatalf("apt cmd should wait on the dpkg lock: %s", cmd)
	}
	// root login (alpine unattended): no privilege prefix.
	if cmd, _ = pkgInstallCmd("apk", "htop", ""); strings.HasPrefix(cmd, "sudo") {
		t.Fatalf("root priv must drop sudo: %s", cmd)
	}
	if _, err := pkgInstallCmd("yum", "x", "sudo "); err == nil {
		t.Fatal("unsupported pm must fail")
	}
}

func TestTailLines(t *testing.T) {
	if tailLines("a\nb\nc", 2) != "b\nc" {
		t.Fatal("tail")
	}
	if tailLines("a\nb", 5) != "a\nb" {
		t.Fatal("short")
	}
}

func TestMoveFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a")
	os.WriteFile(src, []byte("x"), 0o600)
	dst := filepath.Join(dir, "b")
	moveFile(src, dst)
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatal("src must be removed")
	}
	if b, _ := os.ReadFile(dst); string(b) != "x" {
		t.Fatal("content lost")
	}
	moveFile(src, dst) // missing src → silent no-op
}

func TestDispatchStepUnknown(t *testing.T) {
	r, _ := testRunCtx(t)
	if err := r.dispatch(context.Background(), "bogus"); err == nil ||
		!strings.Contains(err.Error(), "bogus") {
		t.Fatalf("unknown step %v", err)
	}
}

func TestFailRun(t *testing.T) {
	r, dir := testRunCtx(t)
	r.artDir = filepath.Join(dir, "art")
	os.MkdirAll(r.artDir, 0o755)
	r.e = newEmitter(r.runID, r.vmName, r.vmDir, r.artDir, true)
	defer r.e.close()
	if rc := r.failRun("X", fmt.Errorf("bad")); rc != 1 {
		t.Fatal("failRun must return 1")
	}
	b, _ := os.ReadFile(filepath.Join(r.artDir, "events.jsonl"))
	if !strings.Contains(string(b), "run.failed") {
		t.Fatal("run.failed not emitted")
	}
}

func TestSeal(t *testing.T) {
	r, dir := testRunCtx(t)
	r.artDir = filepath.Join(dir, "art")
	os.MkdirAll(r.artDir, 0o755)
	confSet(r.confPath, map[string]string{"disk_img": "disk.qcow2"})
	if err := r.seal(); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(r.artDir, "manifest.json"))
	var m map[string]any
	json.Unmarshal(b, &m)
	if m["runId"] != "run-1" || m["provider"] == nil || m["vm"] != "vm" {
		t.Fatalf("manifest %s", b)
	}
	// qcow2 declared but file absent → qemu-img attempt fails →
	// snapshotError recorded, snapshot stays empty (honest manifest).
	r2, dir2 := testRunCtx(t)
	r2.artDir = filepath.Join(dir2, "art")
	os.MkdirAll(r2.artDir, 0o755)
	confSet(r2.confPath, map[string]string{
		"disk_img": "d.qcow2", "disk_format": "qcow2"})
	r2.p.Finalize.Snapshot = "s1"
	if err := r2.seal(); err != nil {
		t.Fatal(err)
	}
	b2, _ := os.ReadFile(filepath.Join(r2.artDir, "manifest.json"))
	var m2 map[string]any
	json.Unmarshal(b2, &m2)
	if m2["snapshot"] != "" && m2["snapshotError"] == nil {
		t.Fatalf("qcow2 snap should error or succeed, got %s", b2)
	}
	// QFI-magic file named .img → sniffed as qcow2 → qemu-img attempt.
	r3, dir3 := testRunCtx(t)
	r3.artDir = filepath.Join(dir3, "art")
	os.MkdirAll(r3.artDir, 0o755)
	os.WriteFile(filepath.Join(dir3, "d.img"),
		[]byte{'Q', 'F', 'I', 0xfb}, 0o600)
	confSet(r3.confPath, map[string]string{"disk_img": "d.img"})
	r3.p.Finalize.Snapshot = "s1"
	if err := r3.seal(); err != nil {
		t.Fatal(err)
	}
	b3, _ := os.ReadFile(filepath.Join(r3.artDir, "manifest.json"))
	var m3 map[string]any
	json.Unmarshal(b3, &m3)
	if m3["snapshotError"] != nil &&
		strings.Contains(fmt.Sprint(m3["snapshotError"]), "cannot hold") {
		t.Fatalf("QFI .img should be treated as qcow2: %s", b3)
	}
	// declared raw disk: snapshot skipped with the reason recorded.
	r4, dir4 := testRunCtx(t)
	r4.artDir = filepath.Join(dir4, "art")
	os.MkdirAll(r4.artDir, 0o755)
	confSet(r4.confPath, map[string]string{
		"disk_img": "d.raw", "disk_format": "raw"})
	r4.p.Finalize.Snapshot = "s1"
	if err := r4.seal(); err != nil {
		t.Fatal(err)
	}
	b4, _ := os.ReadFile(filepath.Join(r4.artDir, "manifest.json"))
	var m4 map[string]any
	json.Unmarshal(b4, &m4)
	if m4["snapshot"] != "" ||
		!strings.Contains(fmt.Sprint(m4["snapshotError"]), "cannot hold") {
		t.Fatalf("raw snap should be skipped: %s", b4)
	}
}

func TestShutdownVMNoVM(t *testing.T) {
	// No VM: closed ssh port refused instantly, no pidfile → fast no-op.
	r, _ := testRunCtx(t)
	if r.kp == nil {
		t.Fatal("kp")
	}
	done := make(chan struct{})
	go func() { r.shutdownVM(); close(done) }()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("shutdownVM hung without a VM")
	}
}

// --- providers' prepare (real conf + seed ISO) ------------------------------

func TestNoCloudPrepare(t *testing.T) {
	r, dir := testRunCtx(t)
	n := &noCloudImage{}
	if err := n.prepare(r); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(r.confPath)
	s := string(b)
	for _, want := range []string{
		`display="none"`, `seed="`, `port_forwards="2222:22"`, `ram="4G"`} {
		if !strings.Contains(s, want) {
			t.Fatalf("conf missing %s:\n%s", want, s)
		}
	}
	ud, _ := os.ReadFile(filepath.Join(dir, "user-data"))
	for _, want := range []string{"#cloud-config", "hostname: h1", "name: u1",
		"ssh_authorized_keys", "passwd:",
		// portability: no /bin/bash (Alpine lacks it), sudo installed
		// via cloud-init so provision never races its absence.
		"shell: /bin/sh", "packages: [sudo]"} {
		if !strings.Contains(string(ud), want) {
			t.Fatalf("user-data missing %s:\n%s", want, ud)
		}
	}
	md, _ := os.ReadFile(filepath.Join(dir, "meta-data"))
	if !strings.Contains(string(md), `"instance-id":"run-1"`) {
		t.Fatalf("meta-data %s", md)
	}
	if st, _ := os.Stat(filepath.Join(dir, "seed.iso")); st == nil {
		t.Fatal("seed.iso missing")
	}
	// locale fields render when set.
	r2, dir2 := testRunCtx(t)
	r2.p.Locale.Timezone = "UTC"
	r2.p.Locale.Lang = "en_US.UTF-8"
	r2.p.Identity.AllowPwAuth = true
	if err := n.prepare(r2); err != nil {
		t.Fatal(err)
	}
	ud2, _ := os.ReadFile(filepath.Join(dir2, "user-data"))
	if !strings.Contains(string(ud2), "timezone: UTC") ||
		!strings.Contains(string(ud2), "ssh_pwauth: true") {
		t.Fatalf("locale/pwauth:\n%s", ud2)
	}
}

func TestUbuntuPrepare(t *testing.T) {
	r, dir := testRunCtx(t)
	// casper ISO fixture so extractBootfiles succeeds.
	iso := makeTestISO(t, map[string]string{
		"/casper/vmlinuz": "k", "/casper/initrd": "i"})
	confSet(r.confPath, map[string]string{"iso": iso})
	u := &ubuntuAutoinstall{}
	if err := u.prepare(r); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(r.confPath)
	s := string(b)
	for _, want := range []string{`kernel="`, `initrd="`, `boot_args="autoinstall`, `seed="`} {
		if !strings.Contains(s, want) {
			t.Fatalf("conf missing %s:\n%s", want, s)
		}
	}
	ud, _ := os.ReadFile(filepath.Join(dir, "user-data"))
	for _, want := range []string{"autoinstall:", "shutdown: poweroff",
		"username: u1", "install-server: true", "authorized-keys"} {
		if !strings.Contains(string(ud), want) {
			t.Fatalf("user-data missing %s:\n%s", want, ud)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "vmlinuz")); err != nil {
		t.Fatal("kernel not extracted")
	}
	// missing iso key → descriptive error.
	r2, _ := testRunCtx(t)
	if err := u.prepare(r2); err == nil || !strings.Contains(err.Error(), "iso=") {
		t.Fatalf("no-iso %v", err)
	}
	// locale + pwauth branches.
	r3, dir3 := testRunCtx(t)
	confSet(r3.confPath, map[string]string{"iso": iso})
	r3.p.Locale.Lang = "es_ES.UTF-8"
	r3.p.Locale.Keyboard = "es"
	r3.p.Locale.Timezone = "Europe/Madrid"
	r3.p.Identity.Password = "pw"
	if err := u.prepare(r3); err != nil {
		t.Fatal(err)
	}
	ud3, _ := os.ReadFile(filepath.Join(dir3, "user-data"))
	for _, want := range []string{"locale: es_ES.UTF-8", "layout: es",
		"timezone: Europe/Madrid", "allow-pw: true"} {
		if !strings.Contains(string(ud3), want) {
			t.Fatalf("user-data missing %s:\n%s", want, ud3)
		}
	}
}

// --- main.go dispatch --------------------------------------------------------

func TestFlagHelpers(t *testing.T) {
	args := []string{"--profile", "p.toml", "--keep"}
	if flag(args, "--profile", "d") != "p.toml" {
		t.Fatal("flag")
	}
	if flag(args, "--none", "d") != "d" {
		t.Fatal("default")
	}
	if flag([]string{"--profile"}, "--profile", "d") != "d" {
		t.Fatal("trailing flag")
	}
	if !hasFlag(args, "--keep") || hasFlag(args, "--nope") {
		t.Fatal("hasFlag")
	}
}

func TestDispatchMeta(t *testing.T) {
	if rc := dispatch([]string{"quickprovision"}); rc != 2 {
		t.Fatalf("no args rc=%d", rc)
	}
	if rc := dispatch([]string{"quickprovision", "--version"}); rc != 0 {
		t.Fatalf("version rc=%d", rc)
	}
	if rc := dispatch([]string{"quickprovision", "version"}); rc != 0 {
		t.Fatalf("version rc=%d", rc)
	}
	for _, h := range []string{"--help", "help", "-h"} {
		if rc := dispatch([]string{"quickprovision", h}); rc != 0 {
			t.Fatalf("%s rc=%d", h, rc)
		}
	}
	if rc := dispatch([]string{"quickprovision", "bogus"}); rc != 2 {
		t.Fatalf("unknown rc=%d", rc)
	}
}

// cmdPlan with a valid profile emits the v1 envelope (stdout) and
// returns 0 — no exit paths involved.
func TestCmdPlan(t *testing.T) {
	pf := writeProfile(t, "[system]\nos=\"cirros\"\nrelease=\"0.6\"\n")
	if rc := cmdPlan([]string{"--profile", pf}); rc != 0 {
		t.Fatalf("plan rc=%d", rc)
	}
	if rc := cmdPlan([]string{"--profile", pf, "--format", "json"}); rc != 0 {
		t.Fatalf("plan json rc=%d", rc)
	}
}

// Error paths that end in qelib.Fail → os.Exit(1) run via self-reexec.
func TestDispatchFailPaths(t *testing.T) {
	if os.Getenv("QE_QT_HELPER") != "" {
		switch os.Getenv("QE_QT_HELPER") {
		case "plan-noprofile":
			os.Exit(dispatch([]string{"quickprovision", "plan"}))
		case "plan-badprofile":
			os.Exit(dispatch([]string{"quickprovision", "plan", "--profile", "/none/x.toml"}))
		case "run-noprofile":
			os.Exit(dispatch([]string{"quickprovision", "run"}))
		case "run-badprofile":
			os.Exit(dispatch([]string{"quickprovision", "run", "--profile", "/none/x.toml"}))
		case "status-noargs":
			os.Exit(dispatch([]string{"quickprovision", "status"}))
		case "cancel-noargs":
			os.Exit(dispatch([]string{"quickprovision", "cancel"}))
		}
		return
	}
	for _, c := range []string{
		"plan-noprofile", "plan-badprofile", "run-noprofile",
		"run-badprofile", "status-noargs", "cancel-noargs"} {
		cmd := exec.Command(os.Args[0], "-test.run=TestDispatchFailPaths")
		cmd.Env = append(os.Environ(), "QE_QT_HELPER="+c)
		out, err := cmd.CombinedOutput()
		ee, ok := err.(*exec.ExitError)
		if !ok || ee.ExitCode() != 1 {
			t.Fatalf("%s rc=%v out=%s", c, err, out)
		}
		var env map[string]any
		if json.Unmarshal([]byte(strings.TrimSpace(string(out))), &env) != nil {
			t.Fatalf("%s out=%s", c, out)
		}
		if env["success"] != false {
			t.Fatalf("%s envelope %s", c, out)
		}
	}
}

// The shipped cloud-dev profiles must parse and resolve the nocloud
// provider — they are the E2E entry points for alpine/debian/fedora.
func TestLoadProfileCloudProfiles(t *testing.T) {
	for _, f := range []string{
		"alpine-cloud-dev.toml", "debian-cloud-dev.toml",
		"fedora-cloud-dev.toml", "ubuntu-cloud-dev.toml"} {
		p, err := loadProfile(filepath.Join("testdata", f))
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		if p.Automation.Provider != "nocloud-image" {
			t.Fatalf("%s provider=%q", f, p.Automation.Provider)
		}
		if len(p.Provision.Packages) == 0 || len(p.Verify) == 0 {
			t.Fatalf("%s provision/verify empty", f)
		}
	}
}
