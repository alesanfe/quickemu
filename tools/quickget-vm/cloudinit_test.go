// cloudinit_test.go — QE-040 cloud-init coverage: merge contract
// (user wins, ssh_authorized_keys union, users merge by name), minimal
// YAML validation, secret redaction, --regen-seed diff, flag parsing.
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alesanfe/quickemu/tools/qelib/qetest"
	"gopkg.in/yaml.v3"
)

func TestParseSeedFlags(t *testing.T) {
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "k.pub")
	os.WriteFile(keyFile, []byte("ssh-ed25519 AAAA key1\n# comment\nssh-ed25519 BBBB key2\n"), 0o600)
	var o seedOptions
	args := []string{
		"--user-data", "ud.yml", "--meta-data", "md.yml",
		"--network-config", "nc.yml", "--packages", "vim,git,htop",
		"--runcmd", "echo hi", "--runcmd", "ls -l",
		"--user", "alice", "--ssh-key", keyFile, "--ssh-key", "ssh-rsa CCCC",
		"--user", "bob", "--hostname", "My_VM", "--timezone", "Europe/Madrid",
		"--locale", "es_ES.UTF-8", "--regen-seed",
	}
	for i := 0; i < len(args); i++ {
		ok, err := o.parseSeedFlag(args, &i)
		if err != nil || !ok {
			t.Fatalf("parseSeedFlag(%q) ok=%v err=%v", args[i], ok, err)
		}
	}
	if o.userDataFile != "ud.yml" || o.metaDataFile != "md.yml" || o.networkFile != "nc.yml" {
		t.Fatalf("files %+v", o)
	}
	if len(o.packages) != 3 || len(o.runcmds) != 2 {
		t.Fatalf("packages/runcmds %+v", o)
	}
	if len(o.users) != 2 || o.users[0].name != "alice" || o.users[1].name != "bob" {
		t.Fatalf("users %+v", o.users)
	}
	// keyFile expands to 2 keys + literal → 3 on alice; bob has none.
	if len(o.users[0].keys) != 3 || len(o.users[1].keys) != 0 {
		t.Fatalf("keys %+v", o.users)
	}
	if !o.explicitKeys || !o.regen || o.hostname != "my-vm" || o.timezone != "Europe/Madrid" || o.locale != "es_ES.UTF-8" {
		t.Fatalf("opts %+v", o)
	}
	// Valueless flag at end must error, not panic.
	bad := []string{"--user-data"}
	i := 0
	if _, err := o.parseSeedFlag(bad, &i); err == nil {
		t.Fatal("missing value must error")
	}
	// Unrelated flag returns consumed=false.
	i = 0
	ok, _ := o.parseSeedFlag([]string{"--bogus"}, &i)
	if ok {
		t.Fatal("non-seed flag must not be consumed")
	}
}

func TestSeedKeysFrom(t *testing.T) {
	if _, err := seedKeysFrom(""); err == nil {
		t.Fatal("empty key must fail")
	}
	empty := filepath.Join(t.TempDir(), "empty")
	os.WriteFile(empty, []byte("# only comments\n"), 0o600)
	if _, err := seedKeysFrom(empty); err == nil {
		t.Fatal("file with no keys must fail")
	}
	k, err := seedKeysFrom("ssh-ed25519 AAAA me@host")
	if err != nil || len(k) != 1 {
		t.Fatalf("literal key %v %v", k, err)
	}
}

func TestBuildUsers(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USER", "tester")
	os.MkdirAll(filepath.Join(home, ".ssh"), 0o700)
	os.WriteFile(filepath.Join(home, ".ssh", "id_ed25519.pub"), []byte("ssh-ed25519 PUB auto\n"), 0o600)

	// No flags: primary user = $USER with the auto-detected key.
	o := seedOptions{}
	us := o.buildUsers()
	if len(us) != 1 || us[0].name != "tester" || len(us[0].keys) != 1 {
		t.Fatalf("default users %+v", us)
	}
	// Explicit --ssh-key anywhere disables auto-detection.
	o = seedOptions{explicitKeys: true}
	us = o.buildUsers()
	if len(us[0].keys) != 0 {
		t.Fatalf("explicit keys must not auto-detect: %+v", us)
	}
	// Key-before-user attaches to the default primary.
	o = seedOptions{explicitKeys: true, users: []seedUser{{keys: []string{"k1"}}, {name: "x"}}}
	us = o.buildUsers()
	if us[0].name != "tester" || us[0].keys[0] != "k1" {
		t.Fatalf("primary attach %+v", us)
	}
}

func TestBaseUserDataShape(t *testing.T) {
	o := seedOptions{
		packages: []string{"vim", "git"},
		runcmds:  []string{"echo one", "ls -la"},
		users:    []seedUser{{name: "alice", keys: []string{"ssh-ed25519 K1"}}, {name: "bob"}},
		hostname: "custom-host", timezone: "UTC", locale: "C.UTF-8",
		explicitKeys: true,
	}
	doc := o.baseUserData("vm-host")
	m, err := parseCloudDoc(doc, "user-data")
	if err != nil {
		t.Fatalf("generated doc invalid: %v\n%s", err, doc)
	}
	if m["hostname"] != "custom-host" || m["timezone"] != "UTC" || m["locale"] != "C.UTF-8" {
		t.Fatalf("top keys %v", m)
	}
	users, _ := m["users"].([]any)
	if len(users) != 2 {
		t.Fatalf("users %v", users)
	}
	pkgs, _ := m["packages"].([]any)
	if len(pkgs) != 2 || pkgs[0] != "vim" {
		t.Fatalf("packages %v", pkgs)
	}
	rc, _ := m["runcmd"].([]any)
	if len(rc) != 2 {
		t.Fatalf("runcmd %v", rc)
	}
	u0, _ := users[0].(map[string]any)
	keys, _ := u0["ssh_authorized_keys"].([]any)
	if len(keys) != 1 || keys[0] != "ssh-ed25519 K1" {
		t.Fatalf("keys %v", keys)
	}
	if u0["plain_text_passwd"] != "quickemu" {
		t.Fatalf("primary must carry the quickemu password")
	}
}

func TestMergeCloudConfig(t *testing.T) {
	base := `#cloud-config
hostname: base-host
manage_etc_hosts: true
users:
  - name: alice
    sudo: ALL=(ALL) NOPASSWD:ALL
    ssh_authorized_keys:
      - ssh-ed25519 BASE
ssh_pwauth: true
package_update: true
packages:
  - vim
`
	overlay := `#cloud-config
hostname: user-host
users:
  - name: alice
    ssh_authorized_keys:
      - ssh-ed25519 USER
      - ssh-ed25519 BASE
  - name: carol
    shell: /bin/zsh
packages:
  - htop
custom_key: 42
`
	merged, err := mergeCloudConfig(base, overlay)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if !strings.HasPrefix(merged, "#cloud-config\n") {
		t.Fatalf("missing header: %q", merged[:20])
	}
	var m map[string]any
	if err := yaml.Unmarshal([]byte(merged), &m); err != nil {
		t.Fatalf("merged doc unparseable: %v", err)
	}
	// user wins on scalar keys and replaces whole keys.
	if m["hostname"] != "user-host" || m["custom_key"] != 42 {
		t.Fatalf("scalar merge %v", m)
	}
	if p, _ := m["packages"].([]any); len(p) != 1 || p[0] != "htop" {
		t.Fatalf("packages replaced %v", p)
	}
	// users merge by name: alice keeps sudo (base field), gets the
	// UNION of ssh keys (no dup), carol appended.
	users, _ := m["users"].([]any)
	if len(users) != 2 {
		t.Fatalf("users %v", users)
	}
	alice, _ := users[0].(map[string]any)
	if alice["sudo"] != "ALL=(ALL) NOPASSWD:ALL" {
		t.Fatalf("base user field lost: %v", alice)
	}
	keys, _ := alice["ssh_authorized_keys"].([]any)
	if len(keys) != 2 {
		t.Fatalf("ssh keys must union-dedup: %v", keys)
	}
	carol, _ := users[1].(map[string]any)
	if carol["shell"] != "/bin/zsh" {
		t.Fatalf("appended user %v", carol)
	}
	// Invalid overlay must fail.
	if _, err := mergeCloudConfig(base, "not yaml: ["); err == nil {
		t.Fatal("invalid overlay must fail")
	}
	// Non-mapping overlay must fail.
	if _, err := mergeCloudConfig(base, "- just\n- a\n- list\n"); err == nil {
		t.Fatal("list-only overlay must fail")
	}
}

func TestParseCloudDocValidation(t *testing.T) {
	if _, err := parseCloudDoc("", "x"); err == nil {
		t.Fatal("empty doc must fail")
	}
	if _, err := parseCloudDoc("scalar", "x"); err == nil {
		t.Fatal("scalar doc must fail")
	}
	if _, err := parseCloudDoc("a: [unclosed", "x"); err == nil {
		t.Fatal("broken YAML must fail")
	}
	m, err := parseCloudDoc("k: v\n", "x")
	if err != nil || m["k"] != "v" {
		t.Fatalf("valid doc %v %v", m, err)
	}
}

func TestRedactSeedYAML(t *testing.T) {
	doc := `hostname: h
users:
  - name: alice
    plain_text_passwd: quickemu
    ssh_authorized_keys:
      - ssh-ed25519 SECRETKEY1
      - ssh-ed25519 SECRETKEY2
  - name: bob
chpasswd:
  expire: false
  list: |
    alice:hunter2
`
	r := redactSeedYAML(doc)
	for _, secret := range []string{"SECRETKEY1", "SECRETKEY2", "hunter2", "plain_text_passwd: quickemu"} {
		if strings.Contains(r, secret) {
			t.Fatalf("secret %q leaked in:\n%s", secret, r)
		}
	}
	// Non-secret content stays readable for the diff; the chpasswd
	// block's nested lines redact as one secret unit.
	if !strings.Contains(r, "hostname: h") || !strings.Contains(r, "- name: bob") ||
		!strings.Contains(r, "plain_text_passwd: <redacted>") {
		t.Fatalf("over-redacted:\n%s", r)
	}
}

func TestDiffSeedDocs(t *testing.T) {
	old := "a: 1\nb: 2\n"
	now := "a: 1\nb: 3\nc: 4\n"
	d := diffSeedDocs(old, now)
	if !strings.Contains(d, "-b: 2") || !strings.Contains(d, "+b: 3") || !strings.Contains(d, "+c: 4") {
		t.Fatalf("diff %q", d)
	}
	if diffSeedDocs(old, old) != "" {
		t.Fatal("identical docs must produce empty diff")
	}
}

func TestGenSeedNoSecretLog(t *testing.T) {
	dir := t.TempDir()
	vm := filepath.Join(dir, "vms")
	os.MkdirAll(vm, 0o755)
	conf := vm + ".conf"
	os.WriteFile(conf, []byte("guest_os=\"linux\"\n"), 0o644)
	t.Setenv("HOME", dir)
	ud := filepath.Join(dir, "ud.yml")
	os.WriteFile(ud, []byte(`users:
  - name: quickemu
    ssh_authorized_keys:
      - ssh-ed25519 VERYSECRET
`), 0o600)
	out := qetest.CaptureStdout(t, func() {
		rc := genSeedISOOpts(vm, conf, &seedOptions{
			userDataFile: ud,
			regen:        true,
			users:        []seedUser{{keys: []string{"ssh-rsa HIDDEN"}}},
			explicitKeys: true,
		})
		if rc != 0 {
			t.Error("genSeedISOOpts failed")
		}
	})
	for _, s := range []string{"VERYSECRET", "HIDDEN"} {
		if strings.Contains(out, s) {
			t.Fatalf("key material leaked to log: %s", out)
		}
	}
	// The seed files themselves DO contain the keys (that's their job).
	b, _ := os.ReadFile(filepath.Join(vm, "user-data"))
	if !strings.Contains(string(b), "VERYSECRET") || !strings.Contains(string(b), "HIDDEN") {
		t.Fatalf("merged user-data lost keys:\n%s", b)
	}
}

func TestSeedSubcommand(t *testing.T) {
	dir := t.TempDir()
	vm := filepath.Join(dir, "vmx")
	os.MkdirAll(vm, 0o755)
	conf := vm + ".conf"
	os.WriteFile(conf, []byte("guest_os=\"linux\"\nseed=\""+vm+"/seed.iso\"\n"), 0o644)
	t.Setenv("HOME", dir)
	t.Setenv("USER", "tester")

	if rc := seedSubcommand([]string{}); rc != 2 {
		t.Fatalf("usage rc=%d", rc)
	}
	if rc := seedSubcommand([]string{filepath.Join(dir, "nope")}); rc != 1 {
		t.Fatalf("missing vmdir rc=%d", rc)
	}
	if rc := seedSubcommand([]string{vm}); rc != 0 {
		t.Fatalf("create rc=%d", rc)
	}
	// Existing seed requires --regen-seed.
	if rc := seedSubcommand([]string{vm}); rc != 1 {
		t.Fatalf("no-regen rc=%d", rc)
	}
	old, _ := os.ReadFile(filepath.Join(vm, "user-data"))
	out := qetest.CaptureStdout(t, func() {
		if rc := seedSubcommand([]string{vm, "--regen-seed", "--hostname", "renamed"}); rc != 0 {
			t.Errorf("regen rc=%d", rc)
		}
	})
	if !strings.Contains(out, "user-data changes:") || !strings.Contains(out, "renamed") {
		t.Fatalf("regen must show diff: %s", out)
	}
	now, _ := os.ReadFile(filepath.Join(vm, "user-data"))
	if string(old) == string(now) || !strings.Contains(string(now), "hostname: renamed") {
		t.Fatalf("regen did not rewrite user-data")
	}
	// No conf → rc1.
	vm2 := filepath.Join(dir, "vm2")
	os.MkdirAll(vm2, 0o755)
	if rc := seedSubcommand([]string{vm2}); rc != 1 {
		t.Fatalf("missing conf rc=%d", rc)
	}
}

func TestGenSeedOverlays(t *testing.T) {
	dir := t.TempDir()
	vm := filepath.Join(dir, "vmm")
	os.MkdirAll(vm, 0o755)
	conf := vm + ".conf"
	os.WriteFile(conf, []byte("guest_os=\"linux\"\n"), 0o644)
	t.Setenv("HOME", dir)
	md := filepath.Join(dir, "md.yml")
	os.WriteFile(md, []byte("instance-id: custom-id\nlocal-hostname: custom\n"), 0o600)
	nc := filepath.Join(dir, "nc.yml")
	os.WriteFile(nc, []byte("version: 1\nconfig:\n  - type: physical\n"), 0o600)
	rc := genSeedISOOpts(vm, conf, &seedOptions{metaDataFile: md, networkFile: nc})
	if rc != 0 {
		t.Fatal("genSeedISOOpts failed")
	}
	b, _ := os.ReadFile(filepath.Join(vm, "meta-data"))
	if !strings.Contains(string(b), "custom-id") {
		t.Fatalf("meta-data overlay ignored: %s", b)
	}
	if _, err := os.Stat(filepath.Join(vm, "network-config")); err != nil {
		t.Fatal("network-config not staged")
	}
	// Bad meta-data must fail.
	os.WriteFile(md, []byte("- list\n"), 0o600)
	vm2 := filepath.Join(dir, "vm2")
	os.MkdirAll(vm2, 0o755)
	os.WriteFile(vm2+".conf", []byte(""), 0o644)
	if rc := genSeedISOOpts(vm2, vm2+".conf", &seedOptions{metaDataFile: md}); rc != 1 {
		t.Fatal("bad meta-data must fail")
	}
}

// TestSubcommandDispatch covers the new main() arms: the seed/overlay/
// checkbase/rebase/flatten/cache subcommands exist and fail with usage
// rc2 on missing args, and seed-flag parse errors inside get/mkconf
// report without leaking values.
func TestSubcommandDispatch(t *testing.T) {
	cases := []struct {
		name string
		args []string
		rc   int
	}{
		{"seed", []string{"seed"}, 2},
		{"overlay", []string{"overlay"}, 2},
		{"checkbase", []string{"checkbase"}, 2},
		{"rebase", []string{"rebase"}, 2},
		{"flatten", []string{"flatten"}, 2},
		{"cache-bogus", []string{"cache", "bogus"}, 2},
		{"get-missing-seed-value", []string{"get", "x", "--user-data"}, 2},
		{"get-bad-flag", []string{"get", "x", "--bogus"}, 2},
		{"mkconf-missing-seed-value", []string{"mkconf", "--os", "x",
			"--image", "i", "--user-data"}, 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, rc := runVM(t, c.args...)
			if rc != c.rc {
				t.Fatalf("%v rc=%d want %d", c.args, rc, c.rc)
			}
		})
	}
}

func TestYAMLEscaping(t *testing.T) {
	// A hostname/zone value that re-parses as bool must be quoted.
	o := seedOptions{timezone: "yes"}
	doc := o.baseUserData("h")
	m, err := parseCloudDoc(doc, "u")
	if err != nil || m["timezone"] != "yes" {
		t.Fatalf("quoted scalar lost: %v %v", m, err)
	}
	// runcmd with shell metachars must survive a YAML round-trip.
	o = seedOptions{runcmds: []string{"echo 'hi' > /x", "true", "a:b"}}
	doc = o.baseUserData("h")
	m, err = parseCloudDoc(doc, "u")
	if err != nil {
		t.Fatalf("runcmd doc invalid: %v\n%s", err, doc)
	}
	rc, _ := m["runcmd"].([]any)
	if len(rc) != 3 || rc[0] != "echo 'hi' > /x" || rc[1] != "true" || rc[2] != "a:b" {
		t.Fatalf("runcmd mangled: %v", rc)
	}
}
