package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEnvErrStrings(t *testing.T) {
	if (envErr{1, 2, "boom"}).Error() != "boom" {
		t.Fatal("envErr.Error")
	}
	es := envErrs{{0, 0, "one"}, {1, 1, "two"}}
	if es.Error() != "one\ntwo" {
		t.Fatalf("envErrs.Error = %q", es.Error())
	}
	if (envErrs{}).Error() != "" {
		t.Fatal("empty envErrs")
	}
}

func TestEnvLookups(t *testing.T) {
	e := &Env{
		VMs:  []*VM{{Name: "a"}, {Name: "b"}},
		Nets: []*Net{{Name: "n"}},
	}
	if e.vm("b") == nil || e.vm("b").Name != "b" {
		t.Fatal("vm(b) not found")
	}
	if e.vm("zz") != nil {
		t.Fatal("vm(zz) should be nil")
	}
	if e.net("n") == nil || e.net("zz") != nil {
		t.Fatal("net lookup wrong")
	}
}

func TestFieldReaders(t *testing.T) {
	d := mustParse(t, `[x]
s = "v"
i = 3
l = ["a", "b"]
bad = [1]
`)
	tab := d.singles["x"]
	if s, e := vstring(tab, "x", "s"); e != nil || s != "v" {
		t.Fatalf("vstring: %q %v", s, e)
	}
	if s, e := vstring(tab, "x", "missing"); e != nil || s != "" {
		t.Fatalf("vstring missing: %q %v", s, e)
	}
	if _, e := vstring(tab, "x", "i"); e == nil || !strings.Contains(e.msg, "x.i must be a string") {
		t.Fatalf("vstring type err = %v", e)
	}
	if n, e := vint(tab, "x", "i"); e != nil || n != 3 {
		t.Fatalf("vint: %d %v", n, e)
	}
	if n, e := vint(tab, "x", "missing"); e != nil || n != 0 {
		t.Fatalf("vint missing: %d %v", n, e)
	}
	if _, e := vint(tab, "x", "s"); e == nil || !strings.Contains(e.msg, "x.s must be an integer") {
		t.Fatalf("vint type err = %v", e)
	}
	if l, e := vlist(tab, "x", "l"); e != nil || len(l) != 2 || l[0] != "a" {
		t.Fatalf("vlist: %v %v", l, e)
	}
	if l, e := vlist(tab, "x", "missing"); e != nil || l != nil {
		t.Fatalf("vlist missing: %v %v", l, e)
	}
	if _, e := vlist(tab, "x", "s"); e == nil || !strings.Contains(e.msg, "expected a list") {
		t.Fatalf("vlist type err = %v", e)
	}
	if _, e := vlist(tab, "x", "bad"); e == nil || !strings.Contains(e.msg, "list of strings") {
		t.Fatalf("vlist elem err = %v", e)
	}
}

func TestSubst(t *testing.T) {
	vars := map[string]string{"a": "X", "b": "Y"}
	var errs envErrs
	if out := subst("${a}-${b}-z", vars, 1, 1, &errs); out != "X-Y-z" || len(errs) != 0 {
		t.Fatalf("subst = %q errs=%v", out, errs)
	}
	if out := subst("plain", vars, 1, 1, &errs); out != "plain" {
		t.Fatalf("subst plain = %q", out)
	}
	out := subst("${nope}", vars, 3, 4, &errs)
	if out != "${nope}" || len(errs) != 1 {
		t.Fatalf("subst unknown = %q errs=%v", out, errs)
	}
	if errs[0].line != 3 || errs[0].col != 4 || !strings.Contains(errs[0].msg, "undefined variable") {
		t.Fatalf("err = %+v", errs[0])
	}
}

func TestContains(t *testing.T) {
	if !contains([]string{"a", "b"}, "b") || contains([]string{"a"}, "z") || contains(nil, "x") {
		t.Fatal("contains")
	}
}

func TestParseFwd(t *testing.T) {
	for _, tc := range []struct {
		in       string
		vm       string
		host     int
		guest    int
		errMatch string
	}{
		{"8080:80", "", 8080, 80, ""},
		{"web:2222:22", "web", 2222, 22, ""},
		{"abc", "", 0, 0, "host:guest or vm:host:guest"},
		{"a:b", "", 0, 0, "host:guest port numbers"},
		{"v:a:b", "", 0, 0, "vm:host:guest port numbers"},
		{"a:b:c:d", "", 0, 0, "host:guest or vm:host:guest"},
		{"0:80", "", 0, 0, "outside 1-65535"},
		{"70000:80", "", 0, 0, "outside 1-65535"},
		{"80:0", "", 0, 0, "outside 1-65535"},
		{"80:70000", "", 0, 0, "outside 1-65535"},
	} {
		f, msg := parseFwd(tc.in)
		if tc.errMatch != "" {
			if !strings.Contains(msg, tc.errMatch) {
				t.Fatalf("%q: err = %q, want %q", tc.in, msg, tc.errMatch)
			}
			continue
		}
		if msg != "" || f.VM != tc.vm || f.Host != tc.host || f.Guest != tc.guest {
			t.Fatalf("%q: %+v %q", tc.in, f, msg)
		}
	}
}

func TestParseMcast(t *testing.T) {
	for _, tc := range []struct {
		in       string
		errMatch string
	}{
		{"nocolon", "must be addr:port"},
		{"230.0.0.1:bad", "bad port"},
		{"230.0.0.1:0", "bad port"},
		{"230.0.0.1:70000", "bad port"},
		{"230.0.0.1", "must be addr:port"},
		{"1.2.3:5000", "not an IPv4 group"},
		{"10.0.0.1:5000", "outside 224.0.0.0/4"},
		{"240.0.0.1:5000", "outside 224.0.0.0/4"},
	} {
		if _, _, msg := parseMcast(tc.in); !strings.Contains(msg, tc.errMatch) {
			t.Fatalf("%q: err = %q, want %q", tc.in, msg, tc.errMatch)
		}
	}
	addr, port, msg := parseMcast("230.0.0.1:5000")
	if msg != "" || addr != "230.0.0.1" || port != 5000 {
		t.Fatalf("valid mcast: %q %d %q", addr, port, msg)
	}
}

func TestLoadEnvMissingFile(t *testing.T) {
	e, errs := loadEnv(filepath.Join(t.TempDir(), "none.toml"))
	if e != nil || len(errs) != 1 || errs[0].line != 0 {
		t.Fatalf("missing file: env=%v errs=%v", e, errs)
	}
}

func TestLoadEnvParseError(t *testing.T) {
	dir := t.TempDir()
	_, errs := loadEnv(writeEnvFile(t, dir, "[env\nx=1"))
	if len(errs) != 1 || errs[0].line < 1 {
		t.Fatalf("parse error: %v", errs)
	}
}

func TestDecodeFullEnvFields(t *testing.T) {
	dir := t.TempDir()
	e := loadGood(t, dir, `
[env]
name = "demo"
version = 1

[vars]
base = "/srv"
maddr = "229.9.9.9:4000"

[[vm]]
name = "web"
os = "ubuntu"
release = "24.04"
edition = "desktop"
ram = "4G"
cpu = 4
profile = "minimal"
disk_size = "20G"
depends_on = ["db"]
nets = ["lan"]

[[vm]]
name = "db"
conf = "db.conf"
nets = ["lan"]

[[net]]
name = "lan"
kind = "socket"
vms = ["web"]
port = 7300

[[net]]
name = "edge"
vms = ["web"]
forwards = ["web:8080:80", "9090:90"]

[[net]]
name = "bus"
kind = "mcast"
vms = ["web", "db"]
mcast = "${maddr}"

[[share]]
name = "www"
path = "${base}/www"
attach = ["web"]
mode = "ro"

[[share]]
name = "data"
path = "/data"
attach = ["web"]

[[healthcheck]]
vm = "web"
type = "ssh"
target = "127.0.0.1:2222"
timeout = "250ms"
interval = 2
retries = 9

[[healthcheck]]
vm = "db"
`)
	if e.Name != "demo" {
		t.Fatalf("name = %q", e.Name)
	}
	if !strings.HasSuffix(e.File, "env.toml") || e.Dir == "" {
		t.Fatalf("file/dir = %q %q", e.File, e.Dir)
	}
	if e.Vars["base"] != "/srv" || e.Vars["maddr"] != "229.9.9.9:4000" {
		t.Fatalf("vars = %v", e.Vars)
	}
	if len(e.VMs) != 2 {
		t.Fatalf("vms = %d", len(e.VMs))
	}
	web := e.vm("web")
	if web.OS != "ubuntu" || web.Release != "24.04" || web.Edition != "desktop" ||
		web.RAM != "4G" || web.CPU != 4 || web.Profile != "minimal" ||
		web.DiskSize != "20G" {
		t.Fatalf("web spec = %+v", web)
	}
	if len(web.Deps) != 1 || web.Deps[0] != "db" {
		t.Fatalf("web deps = %v", web.Deps)
	}
	if len(web.Nets) != 1 || web.Nets[0] != "lan" {
		t.Fatalf("web nets = %v", web.Nets)
	}
	if e.vm("db").Conf != "db.conf" {
		t.Fatalf("db conf = %q", e.vm("db").Conf)
	}
	lan := e.net("lan")
	if lan.Kind != "socket" || lan.Port != 7300 {
		t.Fatalf("lan = %+v", lan)
	}
	// vm.nets back-references merged into lan.Attach: web is declared
	// on both sides (deduped), db joined via its own nets=[...].
	if len(lan.Attach) != 2 || lan.Attach[0] != "web" || lan.Attach[1] != "db" {
		t.Fatalf("lan attach = %v", lan.Attach)
	}
	edge := e.net("edge")
	if edge.Kind != "user" || len(edge.Forwards) != 2 {
		t.Fatalf("edge = %+v", edge)
	}
	if edge.Forwards[0] != (Fwd{VM: "web", Host: 8080, Guest: 80}) ||
		edge.Forwards[1] != (Fwd{Host: 9090, Guest: 90}) {
		t.Fatalf("edge forwards = %+v", edge.Forwards)
	}
	if e.net("bus").Mcast != "229.9.9.9:4000" {
		t.Fatalf("bus mcast = %q", e.net("bus").Mcast)
	}
	if len(e.Shares) != 2 {
		t.Fatalf("shares = %d", len(e.Shares))
	}
	sh := e.Shares[0]
	if sh.Name != "www" || sh.Path != "/srv/www" || sh.Mode != "ro" ||
		len(sh.Attach) != 1 || sh.Attach[0] != "web" {
		t.Fatalf("share = %+v", sh)
	}
	if e.Shares[1].Mode != "rw" {
		t.Fatalf("default share mode = %q", e.Shares[1].Mode)
	}
	if len(e.Health) != 2 {
		t.Fatalf("health = %d", len(e.Health))
	}
	h := e.Health[0]
	if h.VM != "web" || h.Type != "ssh" || h.Target != "127.0.0.1:2222" ||
		h.Timeout != 250*time.Millisecond || h.Interval != 2*time.Second ||
		h.Retries != 9 {
		t.Fatalf("health = %+v", h)
	}
	d := e.Health[1]
	if d.Type != "tcp" || d.Timeout != 5*time.Second ||
		d.Retries != 30 || d.Interval != time.Second {
		t.Fatalf("health defaults = %+v", d)
	}
}

func TestDecodeVarsKinds(t *testing.T) {
	dir := t.TempDir()
	e := loadGood(t, dir, `
[vars]
s = "str"
i = 42
b = true

[[vm]]
name = "a"
conf = "a.conf"
`)
	if e.Vars["s"] != "str" || e.Vars["i"] != "42" || e.Vars["b"] != "true" {
		t.Fatalf("vars = %v", e.Vars)
	}
}

func TestDecodeEnvNameDefault(t *testing.T) {
	dir := t.TempDir()
	e := loadGood(t, dir, `
[[vm]]
name = "a"
conf = "a.conf"
`)
	if e.Name != "env" {
		t.Fatalf("default env name = %q", e.Name)
	}
}

func TestDecodeVMNetsMergeIntoAttach(t *testing.T) {
	// A socket net joined only via vm.nets still counts members.
	dir := t.TempDir()
	e := loadGood(t, dir, `
[[vm]]
name = "a"
conf = "a.conf"
nets = ["lan"]

[[vm]]
name = "b"
conf = "b.conf"
nets = ["lan"]

[[net]]
name = "lan"
kind = "socket"
`)
	lan := e.net("lan")
	if len(lan.Attach) != 2 || lan.Attach[0] != "a" || lan.Attach[1] != "b" {
		t.Fatalf("merged attach = %v", lan.Attach)
	}
}

func TestDecodeInvalidDocs(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		doc  string
		want string
	}{
		// top-level + [env] + [vars]
		{`x = 1`, "unknown top-level key"},
		{"[env]\nbogus = 2", "env.bogus: unknown key"},
		{"[env]\nname = 5", "env.name must be a string, got integer"},
		{"[env]\nversion = \"x\"", "env.version must be an integer"},
		{"[env]\nversion = 2", "env.version 2 is unsupported"},
		{"[vars]\nf = 1.5", "vars.f must be a string, integer or boolean"},
		{"[vars]\nl = [1]", "vars.l must be a string, integer or boolean"},
		// [[vm]] decode
		{"[[vm]]\nbogus = 1", "vm.bogus: unknown key"},
		{"[[vm]]\nname = 5", "vm.name must be a string, got integer"},
		{"[[vm]]\nname = \"a\"\nconf = 5", "vm.conf must be a string"},
		{"[[vm]]\nname = \"a\"\nos = 5", "vm.os must be a string"},
		{"[[vm]]\nname = \"a\"\nrelease = 5", "vm.release must be a string"},
		{"[[vm]]\nname = \"a\"\nedition = 5", "vm.edition must be a string"},
		{"[[vm]]\nname = \"a\"\nram = 5", "vm.ram must be a string"},
		{"[[vm]]\nname = \"a\"\nprofile = 5", "vm.profile must be a string"},
		{"[[vm]]\nname = \"a\"\ndisk_size = 5", "vm.disk_size must be a string"},
		{"[[vm]]\nname = \"a\"\nconf = \"x\"\ncpu = \"4\"", "vm.cpu must be an integer"},
		{"[[vm]]\nname = \"a\"\nconf = \"x\"\ndepends_on = \"b\"", "vm.depends_on: expected a list"},
		{"[[vm]]\nname = \"a\"\nconf = \"x\"\ndepends_on = [1]", "vm.depends_on: expected a list of strings"},
		{"[[vm]]\nname = \"a\"\nconf = \"x\"\nnets = 5", "vm.nets: expected a list"},
		{"[[vm]]\nname = \"a\"\nconf = \"${nope}\"", "undefined variable"},
		{"[[vm]]\nname = \"a\"\nos = \"${nope}\"", "undefined variable"},
		{"[[vm]]\nname = \"a\"\nrelease = \"${nope}\"\nos = \"u\"", "undefined variable"},
		// [[vm]] semantic
		{"[[vm]]\nconf = \"x\"", "vm.name is required"},
		{"[[vm]]\nname = \"1 bad\"\nconf = \"x\"", "must match"},
		{"[[vm]]\nname = \"a\"\nconf = \"x\"\n[[vm]]\nname = \"a\"\nconf = \"y\"", "duplicate vm \"a\""},
		{"[[vm]]\nname = \"a\"\nconf = \"x\"\nos = \"u\"", "conf and an inline spec"},
		{"[[vm]]\nname = \"a\"\nram = \"2G\"\nconf = \"x\"", "conf and an inline spec"},
		{"[[vm]]\nname = \"a\"", "needs conf or an inline spec"},
		{"[[vm]]\nname = \"a\"\nrelease = \"1\"", "release/edition without os"},
		{"[[vm]]\nname = \"a\"\nedition = \"e\"", "release/edition without os"},
		{"[[vm]]\nname = \"a\"\nconf = \"x\"\ncpu = -1", "cpu must be positive"},
		{"[[vm]]\nname = \"a\"\nconf = \"x\"\ndepends_on = [\"a\"]", "depends on itself"},
		{"[[vm]]\nname = \"a\"\nconf = \"x\"\ndepends_on = [\"b\"]", "depends on unknown vm \"b\""},
		{"[[vm]]\nname = \"a\"\nconf = \"x\"\nnets = [\"n1\"]", "references unknown net \"n1\""},
		// [[net]] decode
		{"[[net]]\nbogus = 1", "net.bogus: unknown key"},
		{"[[net]]\nname = 5", "net.name must be a string"},
		{"[[net]]\nname = \"n\"\nkind = 5", "net.kind must be a string"},
		{"[[net]]\nname = \"n\"\nmcast = 5", "net.mcast must be a string"},
		{"[[net]]\nname = \"n\"\nvms = \"a\"", "net.vms: expected a list"},
		{"[[net]]\nname = \"n\"\nforwards = \"x\"", "net.forwards: expected a list"},
		{"[[net]]\nname = \"n\"\nport = \"x\"", "net.port must be an integer"},
		{"[[net]]\nname = \"n\"\nforwards = [\"abc\"]", "must be host:guest or vm:host:guest"},
		{"[[net]]\nname = \"n\"\nforwards = [\"a:b\"]", "must be host:guest port numbers"},
		{"[[net]]\nname = \"n\"\nforwards = [\"v:a:b\"]", "must be vm:host:guest port numbers"},
		{"[[net]]\nname = \"n\"\nforwards = [\"0:5\"]", "has a port outside 1-65535"},
		{"[[net]]\nname = \"n\"\nmcast = \"${nope}\"", "undefined variable"},
		// [[net]] semantic
		{"[[net]]\nkind = \"user\"", "net.name is required"},
		{"[[net]]\nname = \"1 bad\"", "must match"},
		{"[[net]]\nname = \"n\"\n[[net]]\nname = \"n\"", "duplicate net \"n\""},
		{"[[net]]\nname = \"n\"\nkind = \"bogus\"", "kind \"bogus\" must be user|socket|mcast"},
		{"[[vm]]\nname=\"a\"\nconf=\"x\"\n[[net]]\nname = \"n\"\nvms = [\"zz\"]", "attaches unknown vm \"zz\""},
		{"[[vm]]\nname=\"a\"\nconf=\"x\"\n[[net]]\nname = \"n\"\nkind = \"socket\"\nvms = [\"a\"]", "needs 2 or more"},
		{"[[net]]\nname = \"n\"\nport = 70000", "outside 1-65535"},
		{"[[net]]\nname = \"n\"\nport = -5", "outside 1-65535"},
		{"[[vm]]\nname=\"a\"\nconf=\"x\"\n[[net]]\nname = \"n\"\nforwards = [\"zz:8080:80\"]", "forwards targets unknown vm \"zz\""},
		{"[[vm]]\nname=\"a\"\nconf=\"x\"\n[[vm]]\nname=\"b\"\nconf=\"y\"\n[[net]]\nname = \"n\"\nvms = [\"a\"]\nforwards = [\"b:8080:80\"]", "forwards targets \"b\" which is not attached"},
		{"[[vm]]\nname=\"a\"\nconf=\"x\"\n[[vm]]\nname=\"b\"\nconf=\"y\"\n[[net]]\nname = \"n\"\nvms = [\"a\",\"b\"]\nforwards = [\"8080:80\"]", "needs a vm prefix"},
		{"[[net]]\nname = \"n\"\nkind = \"mcast\"\nmcast = \"10.0.0.1:5\"", "outside 224.0.0.0/4"},
		{"[[net]]\nname = \"n\"\nkind = \"mcast\"\nmcast = \"bogus\"", "must be addr:port"},
		// [[share]] decode
		{"[[share]]\nbogus = 1", "share.bogus: unknown key"},
		{"[[share]]\nname = 5", "share.name must be a string"},
		{"[[share]]\nname = \"s\"\npath = 5", "share.path must be a string"},
		{"[[share]]\nname = \"s\"\npath = \"x\"\nattach = \"a\"", "share.attach: expected a list"},
		{"[[share]]\nname = \"s\"\npath = \"x\"\nmode = 5", "share.mode must be a string"},
		{"[[share]]\nname = \"s\"\npath = \"${nope}\"", "undefined variable"},
		// [[share]] semantic
		{"[[share]]\npath = \"x\"", "share.name is required"},
		{"[[share]]\nname = \"s\"\npath = \"x\"\n[[share]]\nname = \"s\"\npath = \"y\"", "duplicate share \"s\""},
		{"[[share]]\nname = \"s\"", "share \"s\".path is required"},
		{"[[share]]\nname = \"s\"\npath = \"x\"\nmode = \"bogus\"", "mode \"bogus\" must be ro|rw"},
		{"[[share]]\nname = \"s\"\npath = \"a b\"", "path contains whitespace"},
		{"[[share]]\nname = \"s\"\npath = \"x\"\nattach = [\"zz\"]", "share \"s\" attaches unknown vm \"zz\""},
		// [[healthcheck]] decode
		{"[[healthcheck]]\nbogus = 1", "healthcheck.bogus: unknown key"},
		{"[[healthcheck]]\nvm = 5", "healthcheck.vm must be a string"},
		{"[[healthcheck]]\ntype = 5", "healthcheck.type must be a string"},
		{"[[healthcheck]]\ntarget = 5", "healthcheck.target must be a string"},
		{"[[healthcheck]]\ntarget = \"${nope}\"", "undefined variable"},
		{"[[vm]]\nname=\"a\"\nconf=\"x\"\n[[healthcheck]]\nvm = \"a\"\ntimeout = \"bogus\"", "healthcheck.timeout:"},
		{"[[vm]]\nname=\"a\"\nconf=\"x\"\n[[healthcheck]]\nvm = \"a\"\ntimeout = true", "healthcheck.timeout must be a duration string or seconds"},
		{"[[vm]]\nname=\"a\"\nconf=\"x\"\n[[healthcheck]]\nvm = \"a\"\ninterval = true", "healthcheck.interval must be a duration string or seconds"},
		{"[[vm]]\nname=\"a\"\nconf=\"x\"\n[[healthcheck]]\nvm = \"a\"\nretries = \"x\"", "healthcheck.retries must be an integer"},
		// [[healthcheck]] semantic
		{"[[healthcheck]]\ntype = \"tcp\"", "healthcheck.vm is required"},
		{"[[healthcheck]]\nvm = \"zz\"", "healthcheck targets unknown vm \"zz\""},
		{"[[vm]]\nname=\"a\"\nconf=\"x\"\n[[healthcheck]]\nvm = \"a\"\ntype = \"bogus\"", "type \"bogus\" must be tcp|ssh|qga-ping"},
		{"[[vm]]\nname=\"a\"\nconf=\"x\"\n[[healthcheck]]\nvm = \"a\"\nretries = -1", "healthcheck retries must be >= 0"},
		// cycles
		{"[[vm]]\nname=\"a\"\nconf=\"x\"\ndepends_on=[\"b\"]\n[[vm]]\nname=\"b\"\nconf=\"y\"\ndepends_on=[\"a\"]", "dependency cycle: "},
	} {
		got := loadErrs(t, dir, tc.doc)
		if !strings.Contains(got, tc.want) {
			t.Fatalf("doc %q:\n want substring %q\n got: %s", tc.doc, tc.want, got)
		}
	}
}

func TestDecodeSocketNetValidWithTwoAttach(t *testing.T) {
	dir := t.TempDir()
	loadGood(t, dir, `
[[vm]]
name = "a"
conf = "a.conf"

[[vm]]
name = "b"
conf = "b.conf"

[[net]]
name = "lan"
kind = "socket"
vms = ["a", "b"]
port = 7200
`)
}

func TestDecodeNetExplicitPortAccepted(t *testing.T) {
	dir := t.TempDir()
	e := loadGood(t, dir, `
[[net]]
name = "n"
port = 7300
`)
	if e.net("n").Port != 7300 {
		t.Fatalf("port = %d", e.net("n").Port)
	}
}

func TestSubstInOSSpec(t *testing.T) {
	dir := t.TempDir()
	e := loadGood(t, dir, `
[vars]
osname = "ubuntu"
rel = "24.04"

[[vm]]
name = "a"
os = "${osname}"
release = "${rel}"
`)
	v := e.vm("a")
	if v.OS != "ubuntu" || v.Release != "24.04" {
		t.Fatalf("subst spec = %+v", v)
	}
}
