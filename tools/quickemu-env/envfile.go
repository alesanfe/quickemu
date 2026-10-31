// envfile.go: the env.toml schema (QE-070) — decode the parsed tDoc
// into typed structs, substitute [vars] ${name} references and run
// every semantic check validate reports.
//
//	[env]   name="demo"  version=1
//	[vars]  base="./vms"
//	[[vm]]  name  conf | os/release/edition | ram cpu profile
//	        depends_on=[names]  nets=[net names]
//	[[net]] name  kind=user|socket|mcast  vms=[attached]  forwards
//	[[share]] name  path  attach=[vms]  mode=ro|rw
//	[[healthcheck]] vm  type=tcp|ssh|qga-ping  target  timeout  retries
package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// envErr is one validation diagnostic; line/col 0 means "file-level"
// (no position available — validate still prints file: msg).
type envErr struct {
	line, col int
	msg       string
}

func (e envErr) Error() string { return e.msg }

type envErrs []envErr

func (es envErrs) Error() string {
	var b strings.Builder
	for i, e := range es {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(e.msg)
	}
	return b.String()
}

// ---------- decoded schema ----------

// VM is one [[vm]] element: either a conf= path or an inline spec the
// up path materialises through quickget/mkconf.
type VM struct {
	Name     string
	Conf     string // env-relative or absolute path to a .conf
	OS       string
	Release  string
	Edition  string
	RAM      string // "4G", "512M" — passed through to the conf
	CPU      int    // cpu_cores
	Profile  string // quickemu profile preset name
	DiskSize string // optional disk_size conf key for mkconf specs
	Nets     []string
	Deps     []string
	line     int // line of its name= key (diagnostics)
}

// Net is one [[net]] element. Attach is the merged set of member VMs
// (net.vms + every vm.nets back-reference), in declaration order.
type Net struct {
	Name     string
	Kind     string // user|socket|mcast
	Attach   []string
	Forwards []Fwd
	Port     int // explicit listen/mcast port; 0 = allocate
	Mcast    string
	line     int
}

// Fwd is one port forward: Host port on the host side, Guest port in
// the VM. VM targets one member; "" means every member (only legal
// when the net has exactly one attached VM).
type Fwd struct {
	VM          string
	Host, Guest int
}

// Share is one [[share]] element — a host path exported into guests.
type Share struct {
	Name   string
	Path   string
	Attach []string
	Mode   string // ro|rw
	line   int
}

// Health is one [[healthcheck]] element.
type Health struct {
	VM       string
	Type     string // tcp|ssh|qga-ping
	Target   string // host:port or bare port; "" derives the SSH forward
	Timeout  time.Duration
	Retries  int
	Interval time.Duration
	line     int
}

// Env is a fully-decoded env.toml.
type Env struct {
	File   string // absolute path of env.toml
	Dir    string // env working dir (dirname of File)
	Name   string
	Vars   map[string]string
	VMs    []*VM
	Nets   []*Net
	Shares []*Share
	Health []*Health
}

func (e *Env) vm(name string) *VM {
	for _, v := range e.VMs {
		if v.Name == name {
			return v
		}
	}
	return nil
}

func (e *Env) net(name string) *Net {
	for _, n := range e.Nets {
		if n.Name == name {
			return n
		}
	}
	return nil
}

// ---------- field readers (type-checked, positioned) ----------

// field returns the value for key in tab — the error message uses the
// section name for context ("vm.name").
func vstring(tab tTable, sec, key string) (string, *envErr) {
	v, ok := tab.vals[key]
	if !ok {
		return "", nil
	}
	if v.kind != tString {
		return "", &envErr{v.line, v.col,
			fmt.Sprintf("%s.%s must be a string, got %s", sec, key, v.kindName())}
	}
	return v.s, nil
}

func vint(tab tTable, sec, key string) (int64, *envErr) {
	v, ok := tab.vals[key]
	if !ok {
		return 0, nil
	}
	if v.kind != tInt {
		return 0, &envErr{v.line, v.col,
			fmt.Sprintf("%s.%s must be an integer, got %s", sec, key, v.kindName())}
	}
	return v.i, nil
}

func vlist(tab tTable, sec, key string) ([]string, *envErr) {
	v, ok := tab.vals[key]
	if !ok {
		return nil, nil
	}
	out, err := v.strs()
	if err != nil {
		return nil, &envErr{v.line, v.col, fmt.Sprintf("%s.%s: %s", sec, key, err)}
	}
	return out, nil
}

var nameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// ---------- var substitution ----------

var varRef = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_.-]*)\}`)

// subst expands ${var} references in s from vars; an unknown reference
// is an error positioned at the value that used it.
func subst(s string, vars map[string]string, line, col int, errs *envErrs) string {
	bad := false
	out := varRef.ReplaceAllStringFunc(s, func(m string) string {
		name := m[2 : len(m)-1]
		v, ok := vars[name]
		if !ok {
			bad = true
			return m
		}
		return v
	})
	if bad {
		*errs = append(*errs, envErr{line, col,
			fmt.Sprintf("undefined variable in %q", s)})
	}
	return out
}

// ---------- decode ----------

// decodeEnv converts a parsed tDoc into an Env, accumulating every
// diagnostic instead of stopping at the first one.
func decodeEnv(file, dir string, d *tDoc) (*Env, envErrs) {
	var errs envErrs
	e := &Env{File: file, Dir: dir, Vars: map[string]string{}}

	// Top-level keys have no home in the schema.
	for k := range d.globals.vals {
		p := d.globals.pos[k]
		errs = append(errs, envErr{p[0], p[1], k + ": unknown top-level key (belongs inside a section)"})
	}

	// [env]
	if tab, ok := d.singles["env"]; ok {
		known := map[string]bool{"name": true, "version": true}
		for k := range tab.vals {
			if !known[k] {
				p := tab.pos[k]
				errs = append(errs, envErr{p[0], p[1], "env." + k + ": unknown key"})
			}
		}
		name, e0 := vstring(tab, "env", "name")
		ver, e1 := vint(tab, "env", "version")
		if e0 != nil {
			errs = append(errs, *e0)
		}
		if e1 != nil {
			errs = append(errs, *e1)
		}
		e.Name = name
		if ver != 0 && ver != 1 {
			p := tab.pos["version"]
			errs = append(errs, envErr{p[0], p[1],
				fmt.Sprintf("env.version %d is unsupported (this tool understands 1)", ver)})
		}
	}
	if e.Name == "" {
		e.Name = "env"
	}

	// [vars] — values may be strings or ints (ints render via Itoa).
	if tab, ok := d.singles["vars"]; ok {
		for k, v := range tab.vals {
			switch v.kind {
			case tString:
				e.Vars[k] = v.s
			case tInt:
				e.Vars[k] = strconv.FormatInt(v.i, 10)
			case tBool:
				e.Vars[k] = strconv.FormatBool(v.b)
			default:
				errs = append(errs, envErr{v.line, v.col,
					"vars." + k + " must be a string, integer or boolean"})
			}
		}
	}

	// Substitution into string fields happens per-section below.

	// [[vm]]
	vmKeys := map[string]bool{
		"name": true, "conf": true, "os": true, "release": true,
		"edition": true, "ram": true, "cpu": true, "profile": true,
		"disk_size": true, "depends_on": true, "nets": true,
	}
	for _, tab := range d.arrays["vm"] {
		vm := &VM{}
		for k := range tab.vals {
			if !vmKeys[k] {
				p := tab.pos[k]
				errs = append(errs, envErr{p[0], p[1], "vm." + k + ": unknown key"})
			}
		}
		get := func(key string) (string, bool) {
			v, ok := tab.vals[key]
			if !ok {
				return "", false
			}
			if v.kind != tString {
				errs = append(errs, envErr{v.line, v.col,
					fmt.Sprintf("vm.%s must be a string, got %s", key, v.kindName())})
				return "", false
			}
			return v.s, true
		}
		vm.Name, _ = get("name")
		if p, ok := tab.pos["name"]; ok {
			vm.line = p[0]
		}
		if s, ok := get("conf"); ok {
			vm.Conf = subst(s, e.Vars, tab.vals["conf"].line, tab.vals["conf"].col, &errs)
		}
		if s, ok := get("os"); ok {
			vm.OS = subst(s, e.Vars, tab.vals["os"].line, tab.vals["os"].col, &errs)
		}
		if s, ok := get("release"); ok {
			vm.Release = subst(s, e.Vars, tab.vals["release"].line, tab.vals["release"].col, &errs)
		}
		vm.Edition, _ = get("edition")
		vm.RAM, _ = get("ram")
		vm.Profile, _ = get("profile")
		vm.DiskSize, _ = get("disk_size")
		if n, e2 := vint(tab, "vm", "cpu"); e2 != nil {
			errs = append(errs, *e2)
		} else {
			vm.CPU = int(n)
		}
		if l, e3 := vlist(tab, "vm", "depends_on"); e3 != nil {
			errs = append(errs, *e3)
		} else {
			vm.Deps = l
		}
		if l, e4 := vlist(tab, "vm", "nets"); e4 != nil {
			errs = append(errs, *e4)
		} else {
			vm.Nets = l
		}
		e.VMs = append(e.VMs, vm)
	}

	// [[net]]
	netKeys := map[string]bool{
		"name": true, "kind": true, "vms": true, "forwards": true,
		"port": true, "mcast": true,
	}
	for _, tab := range d.arrays["net"] {
		n := &Net{Kind: "user"}
		for k := range tab.vals {
			if !netKeys[k] {
				p := tab.pos[k]
				errs = append(errs, envErr{p[0], p[1], "net." + k + ": unknown key"})
			}
		}
		if s, e0 := vstring(tab, "net", "name"); e0 != nil {
			errs = append(errs, *e0)
		} else {
			n.Name = s
		}
		if p, ok := tab.pos["name"]; ok {
			n.line = p[0]
		}
		if s, e0 := vstring(tab, "net", "kind"); e0 != nil {
			errs = append(errs, *e0)
		} else if s != "" {
			n.Kind = s
		}
		if s, e0 := vstring(tab, "net", "mcast"); e0 != nil {
			errs = append(errs, *e0)
		} else if s != "" {
			n.Mcast = subst(s, e.Vars, tab.vals["mcast"].line, tab.vals["mcast"].col, &errs)
		}
		if l, e1 := vlist(tab, "net", "vms"); e1 != nil {
			errs = append(errs, *e1)
		} else {
			n.Attach = l
		}
		if l, e2 := vlist(tab, "net", "forwards"); e2 != nil {
			errs = append(errs, *e2)
		} else {
			v := tab.vals["forwards"]
			for _, f := range l {
				f = subst(f, e.Vars, v.line, v.col, &errs)
				fw, ferr := parseFwd(f)
				if ferr != "" {
					errs = append(errs, envErr{v.line, v.col, ferr})
					continue
				}
				n.Forwards = append(n.Forwards, fw)
			}
		}
		if p, e3 := vint(tab, "net", "port"); e3 != nil {
			errs = append(errs, *e3)
		} else {
			n.Port = int(p)
		}
		e.Nets = append(e.Nets, n)
	}

	// [[share]]
	shKeys := map[string]bool{"name": true, "path": true, "attach": true, "mode": true}
	for _, tab := range d.arrays["share"] {
		sh := &Share{Mode: "rw"}
		for k := range tab.vals {
			if !shKeys[k] {
				p := tab.pos[k]
				errs = append(errs, envErr{p[0], p[1], "share." + k + ": unknown key"})
			}
		}
		if s, e0 := vstring(tab, "share", "name"); e0 != nil {
			errs = append(errs, *e0)
		} else {
			sh.Name = s
		}
		if p, ok := tab.pos["name"]; ok {
			sh.line = p[0]
		}
		if v, ok := tab.vals["path"]; ok {
			s, e1 := vstring(tab, "share", "path")
			if e1 != nil {
				errs = append(errs, *e1)
			} else {
				sh.Path = subst(s, e.Vars, v.line, v.col, &errs)
			}
		}
		if l, e2 := vlist(tab, "share", "attach"); e2 != nil {
			errs = append(errs, *e2)
		} else {
			sh.Attach = l
		}
		if s, e3 := vstring(tab, "share", "mode"); e3 != nil {
			errs = append(errs, *e3)
		} else if s != "" {
			sh.Mode = s
		}
		e.Shares = append(e.Shares, sh)
	}

	// [[healthcheck]]
	hcKeys := map[string]bool{
		"vm": true, "type": true, "target": true,
		"timeout": true, "retries": true, "interval": true,
	}
	for _, tab := range d.arrays["healthcheck"] {
		h := &Health{Type: "tcp", Timeout: 5 * time.Second, Retries: 30, Interval: time.Second}
		for k := range tab.vals {
			if !hcKeys[k] {
				p := tab.pos[k]
				errs = append(errs, envErr{p[0], p[1], "healthcheck." + k + ": unknown key"})
			}
		}
		if s, e0 := vstring(tab, "healthcheck", "vm"); e0 != nil {
			errs = append(errs, *e0)
		} else {
			h.VM = s
		}
		if p, ok := tab.pos["vm"]; ok {
			h.line = p[0]
		}
		if s, e0 := vstring(tab, "healthcheck", "type"); e0 != nil {
			errs = append(errs, *e0)
		} else if s != "" {
			h.Type = s
		}
		if v, ok := tab.vals["target"]; ok {
			s, e1 := vstring(tab, "healthcheck", "target")
			if e1 != nil {
				errs = append(errs, *e1)
			} else {
				h.Target = subst(s, e.Vars, v.line, v.col, &errs)
			}
		}
		for _, dk := range []struct {
			key string
			dst *time.Duration
		}{{"timeout", &h.Timeout}, {"interval", &h.Interval}} {
			v, ok := tab.vals[dk.key]
			if !ok {
				continue
			}
			switch v.kind {
			case tString:
				d, err := time.ParseDuration(v.s)
				if err != nil {
					errs = append(errs, envErr{v.line, v.col,
						fmt.Sprintf("healthcheck.%s: %v", dk.key, err)})
				} else {
					*dk.dst = d
				}
			case tInt:
				*dk.dst = time.Duration(v.i) * time.Second
			default:
				errs = append(errs, envErr{v.line, v.col,
					fmt.Sprintf("healthcheck.%s must be a duration string or seconds", dk.key)})
			}
		}
		if v, ok := tab.vals["retries"]; ok {
			if v.kind != tInt {
				errs = append(errs, envErr{v.line, v.col,
					fmt.Sprintf("healthcheck.retries must be an integer, got %s", v.kindName())})
			} else {
				h.Retries = int(v.i)
			}
		}
		e.Health = append(e.Health, h)
	}
	return e, errs
}

// parseFwd parses "host:guest" or "vm:host:guest" forward specs.
func parseFwd(s string) (Fwd, string) {
	parts := strings.Split(s, ":")
	f := Fwd{}
	switch len(parts) {
	case 2:
		h, err1 := strconv.Atoi(parts[0])
		g, err2 := strconv.Atoi(parts[1])
		if err1 != nil || err2 != nil {
			return f, fmt.Sprintf("forwards entry %q must be host:guest port numbers", s)
		}
		f.Host, f.Guest = h, g
	case 3:
		f.VM = parts[0]
		h, err1 := strconv.Atoi(parts[1])
		g, err2 := strconv.Atoi(parts[2])
		if err1 != nil || err2 != nil {
			return f, fmt.Sprintf("forwards entry %q must be vm:host:guest port numbers", s)
		}
		f.Host, f.Guest = h, g
	default:
		return f, fmt.Sprintf("forwards entry %q must be host:guest or vm:host:guest", s)
	}
	if f.Host < 1 || f.Host > 65535 || f.Guest < 1 || f.Guest > 65535 {
		return f, fmt.Sprintf("forwards entry %q has a port outside 1-65535", s)
	}
	return f, ""
}

// ---------- semantic validation ----------

// semantic runs the cross-reference checks that need the whole
// document: unique names, resolvable references, cycle detection.
func (e *Env) semantic() envErrs {
	var errs envErrs
	// vm.nets back-references merge into net.Attach BEFORE the checks
	// below count members (a socket net joined only via vm.nets still
	// needs its >=2 members). Declaration order, deduped.
	for _, v := range e.VMs {
		for _, nname := range v.Nets {
			if n := e.net(nname); n != nil && !contains(n.Attach, v.Name) {
				n.Attach = append(n.Attach, v.Name)
			}
		}
	}
	seen := map[string]*VM{}
	for _, v := range e.VMs {
		if v.Name == "" {
			errs = append(errs, envErr{v.line, 1, "vm.name is required"})
			continue
		}
		if !nameRe.MatchString(v.Name) {
			errs = append(errs, envErr{v.line, 1,
				fmt.Sprintf("vm.name %q must match %s", v.Name, nameRe)})
		}
		if prev, dup := seen[v.Name]; dup {
			errs = append(errs, envErr{v.line, 1,
				fmt.Sprintf("duplicate vm %q (first defined at line %d)", v.Name, prev.line)})
		}
		seen[v.Name] = v
		// conf XOR inline spec
		hasSpec := v.OS != "" || v.Release != "" || v.Edition != "" ||
			v.RAM != "" || v.CPU != 0 || v.Profile != "" || v.DiskSize != ""
		if v.Conf != "" && hasSpec {
			errs = append(errs, envErr{v.line, 1,
				fmt.Sprintf("vm %q sets conf and an inline spec — pick one", v.Name)})
		}
		if v.Conf == "" && !hasSpec {
			errs = append(errs, envErr{v.line, 1,
				fmt.Sprintf("vm %q needs conf or an inline spec (os/release/...)", v.Name)})
		}
		if v.OS == "" && (v.Release != "" || v.Edition != "") {
			errs = append(errs, envErr{v.line, 1,
				fmt.Sprintf("vm %q sets release/edition without os", v.Name)})
		}
		if v.CPU < 0 {
			errs = append(errs, envErr{v.line, 1,
				fmt.Sprintf("vm %q cpu must be positive", v.Name)})
		}
		for _, d := range v.Deps {
			if d == v.Name {
				errs = append(errs, envErr{v.line, 1,
					fmt.Sprintf("vm %q depends on itself", v.Name)})
			} else if e.vm(d) == nil {
				errs = append(errs, envErr{v.line, 1,
					fmt.Sprintf("vm %q depends on unknown vm %q", v.Name, d)})
			}
		}
		for _, n := range v.Nets {
			if e.net(n) == nil {
				errs = append(errs, envErr{v.line, 1,
					fmt.Sprintf("vm %q references unknown net %q", v.Name, n)})
			}
		}
	}
	netSeen := map[string]bool{}
	for _, n := range e.Nets {
		if n.Name == "" {
			errs = append(errs, envErr{n.line, 1, "net.name is required"})
			continue
		}
		if !nameRe.MatchString(n.Name) {
			errs = append(errs, envErr{n.line, 1,
				fmt.Sprintf("net.name %q must match %s", n.Name, nameRe)})
		}
		if netSeen[n.Name] {
			errs = append(errs, envErr{n.line, 1, "duplicate net " + strconv.Quote(n.Name)})
		}
		netSeen[n.Name] = true
		switch n.Kind {
		case "user", "socket", "mcast":
		default:
			errs = append(errs, envErr{n.line, 1,
				fmt.Sprintf("net %q kind %q must be user|socket|mcast", n.Name, n.Kind)})
		}
		for _, a := range n.Attach {
			if e.vm(a) == nil {
				errs = append(errs, envErr{n.line, 1,
					fmt.Sprintf("net %q attaches unknown vm %q", n.Name, a)})
			}
		}
		if n.Kind == "socket" && len(n.Attach) > 0 && len(n.Attach) < 2 {
			errs = append(errs, envErr{n.line, 1,
				fmt.Sprintf("net %q (socket) attaches %d vm — point-to-point needs 2 or more", n.Name, len(n.Attach))})
		}
		if n.Port != 0 && (n.Port < 1 || n.Port > 65535) {
			errs = append(errs, envErr{n.line, 1,
				fmt.Sprintf("net %q port %d outside 1-65535", n.Name, n.Port)})
		}
		for _, f := range n.Forwards {
			if f.VM != "" && e.vm(f.VM) == nil {
				errs = append(errs, envErr{n.line, 1,
					fmt.Sprintf("net %q forwards targets unknown vm %q", n.Name, f.VM)})
			}
			if f.VM != "" && len(n.Attach) > 0 && !contains(n.Attach, f.VM) {
				errs = append(errs, envErr{n.line, 1,
					fmt.Sprintf("net %q forwards targets %q which is not attached", n.Name, f.VM)})
			}
		}
		if n.Mcast != "" {
			if _, _, err := parseMcast(n.Mcast); err != "" {
				errs = append(errs, envErr{n.line, 1, "net " + n.Name + " mcast: " + err})
			}
		}
	}
	shSeen := map[string]bool{}
	for _, s := range e.Shares {
		if s.Name == "" {
			errs = append(errs, envErr{s.line, 1, "share.name is required"})
			continue
		}
		if shSeen[s.Name] {
			errs = append(errs, envErr{s.line, 1, "duplicate share " + strconv.Quote(s.Name)})
		}
		shSeen[s.Name] = true
		if s.Path == "" {
			errs = append(errs, envErr{s.line, 1, "share " + strconv.Quote(s.Name) + ".path is required"})
		}
		if s.Mode != "ro" && s.Mode != "rw" {
			errs = append(errs, envErr{s.line, 1,
				fmt.Sprintf("share %q mode %q must be ro|rw", s.Name, s.Mode)})
		}
		if strings.ContainsAny(s.Path, " \t\n") {
			errs = append(errs, envErr{s.line, 1,
				fmt.Sprintf("share %q path contains whitespace (extra_args cannot carry it)", s.Name)})
		}
		for _, a := range s.Attach {
			if e.vm(a) == nil {
				errs = append(errs, envErr{s.line, 1,
					fmt.Sprintf("share %q attaches unknown vm %q", s.Name, a)})
			}
		}
	}
	for _, h := range e.Health {
		if h.VM == "" {
			errs = append(errs, envErr{h.line, 1, "healthcheck.vm is required"})
		} else if e.vm(h.VM) == nil {
			errs = append(errs, envErr{h.line, 1,
				fmt.Sprintf("healthcheck targets unknown vm %q", h.VM)})
		}
		switch h.Type {
		case "tcp", "ssh", "qga-ping":
		default:
			errs = append(errs, envErr{h.line, 1,
				fmt.Sprintf("healthcheck %q type %q must be tcp|ssh|qga-ping", h.VM, h.Type)})
		}
		if h.Retries < 0 {
			errs = append(errs, envErr{h.line, 1, "healthcheck retries must be >= 0"})
		}
	}

	// forwards without a target fan out to every attached VM — legal
	// only when the net has exactly one member.
	for _, n := range e.Nets {
		for _, f := range n.Forwards {
			if f.VM == "" && len(n.Attach) > 1 {
				errs = append(errs, envErr{n.line, 1,
					fmt.Sprintf("net %q forward %d:%d needs a vm prefix — %d vms attached",
						n.Name, f.Host, f.Guest, len(n.Attach))})
			}
		}
	}

	if cyc := e.cycle(); cyc != nil {
		errs = append(errs, envErr{0, 0,
			"dependency cycle: " + strings.Join(cyc, " -> ")})
	}
	return errs
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

// parseMcast validates "addr:port" multicast groups.
func parseMcast(s string) (string, int, string) {
	i := strings.LastIndexByte(s, ':')
	if i < 0 {
		return "", 0, fmt.Sprintf("mcast %q must be addr:port", s)
	}
	p, err := strconv.Atoi(s[i+1:])
	if err != nil || p < 1 || p > 65535 {
		return "", 0, fmt.Sprintf("mcast %q has a bad port", s)
	}
	addr := s[:i]
	parts := strings.Split(addr, ".")
	if len(parts) != 4 {
		return "", 0, fmt.Sprintf("mcast %q is not an IPv4 group", s)
	}
	first, _ := strconv.Atoi(parts[0])
	if first < 224 || first > 239 {
		return "", 0, fmt.Sprintf("mcast %q is outside 224.0.0.0/4", s)
	}
	return addr, p, ""
}

// loadEnv reads + parses + validates an env.toml.
func loadEnv(file string) (*Env, envErrs) {
	b, err := readFile(file)
	if err != nil {
		return nil, envErrs{{0, 0, err.Error()}}
	}
	abs := file
	if a, err2 := absPath(file); err2 == nil {
		abs = a
	}
	d, err := parseTOML(file, string(b))
	if err != nil {
		if pe, ok := err.(*parseErr); ok {
			return nil, envErrs{{pe.line, pe.col, pe.msg}}
		}
		return nil, envErrs{{0, 0, err.Error()}}
	}
	e, errs := decodeEnv(abs, dirOf(abs), d)
	errs = append(errs, e.semantic()...)
	return e, errs
}
