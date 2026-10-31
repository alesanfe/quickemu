// nets.go: QE-072 rootless multi-VM networking. QEMU needs no
// privileges for user, socket and mcast netdevs, so every env net
// lands on the QEMU command line via the conf `extra_args` escape
// hatch — the least invasive mechanism: nothing rewrites the user's
// .conf; `quickemu --vm x.conf --start --extra_args "<args>"` merges
// it through applyOverrides into a throwaway conf. (extra_args is
// word-split by the launcher, so every emitted fragment is
// whitespace-free — share paths are validated accordingly.)
//
// Emitted fragments per net kind:
//
//	user:    (default user netdev already carries ssh_port +
//	         conf port_forwards; env forwards add a SECOND
//	         point-to-point-less netdev carrying hostfwd entries)
//	         -netdev user,id=qenvN,hostfwd=tcp::H-:G,... -device virtio-net-pci,netdev=qenvN
//	socket:  first attached VM listens, the rest connect
//	         -netdev socket,id=qenvN,listen=:PORT      -device virtio-net-pci,netdev=qenvN
//	         -netdev socket,id=qenvN,connect=127.0.0.1:PORT
//	mcast:   every member joins the same group
//	         -netdev socket,id=qenvN,mcast=ADDR:PORT  -device virtio-net-pci,netdev=qenvN
//	share:   -virtfs local,path=P,mount_tag=NAME,security_model=none[,readonly=on]
//
// Port allocation goes through the quickemu-ports sibling
// (--registry <envdir>/.quickemu-env/ports) so reservations are
// visible to `quickemu-ports list` and stale entries (dead owner pid)
// are pruned by the registry itself. When the sibling is missing the
// allocator degrades to a local bind probe.
package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// portRange is the env allocation band — above the launcher's
// ssh/monitor/spice bands so env nets never steal service ports.
const (
	portLo = 7100
	portHi = 7999
)

// netDevice is the emulated NIC attached to env netdevs — virtio is
// the only model that is right on every guest/machine-type quickemu
// supports.
const netDevice = "virtio-net-pci"

// listener picks the VM that binds listen= for a socket net: the
// first attached VM in declaration order.
func (n *Net) listener() string {
	if len(n.Attach) == 0 {
		return ""
	}
	return n.Attach[0]
}

// mcastAddr returns the group "addr:port" for an mcast net — explicit
// `mcast=` wins, else the default group on the allocated port.
func (n *Net) mcastAddr(port int) string {
	if n.Mcast != "" {
		return n.Mcast
	}
	return "230.0.0.1:" + strconv.Itoa(port)
}

// ---------- port allocation ----------

// portsBin locates the quickemu-ports sibling ("" when absent — the
// allocator then works registry-less).
func portsBin() string {
	b, err := findTool("quickemu-ports")
	if err != nil {
		return ""
	}
	return b
}

// portsFind asks the registry for a free port; falls back to a raw
// bind probe. Returns 0 when nothing is free.
func (e *Env) portsFind() int {
	if portsBin() != "" {
		out, err := runTool("quickemu-ports", "", nil, "find",
			"--registry", e.portsReg(), "--lo", strconv.Itoa(portLo),
			"--hi", strconv.Itoa(portHi))
		if err == nil {
			if p, err2 := strconv.Atoi(strings.TrimSpace(strings.Split(out, "\n")[0])); err2 == nil {
				return p
			}
		}
	}
	// Fallback: probe the band ourselves.
	for p := portLo; p <= portHi; p++ {
		ln, err := net.Listen("tcp", ":"+strconv.Itoa(p))
		if err == nil {
			ln.Close()
			return p
		}
	}
	return 0
}

// portsReserve records owner/instance for a port. Missing sibling →
// best-effort no-op (the persisted state.json assignment still holds).
func (e *Env) portsReserve(port, owner int, instance string) {
	if portsBin() == "" {
		return
	}
	runTool("quickemu-ports", "", nil, "reserve",
		"--registry", e.portsReg(), "--port", strconv.Itoa(port),
		"--pid", strconv.Itoa(owner), "--instance", instance)
}

func (e *Env) portsRelease(port int) {
	if portsBin() == "" {
		return
	}
	runTool("quickemu-ports", "", nil, "release",
		"--registry", e.portsReg(), "--port", strconv.Itoa(port))
}

// portsFree reports whether a bind succeeds — used to revalidate
// persisted assignments whose owner pid died.
func portsFree(port int) bool {
	ln, err := net.Listen("tcp", ":"+strconv.Itoa(port))
	if err != nil {
		return false
	}
	ln.Close()
	return true
}

// orphanSweep drops net assignments whose owner pid is dead — the
// "sockets huérfanos" case: the VM that owned listen= died. A live
// foreign pid holding the registry/port keeps it (we reallocate); a
// dead owner frees the port for re-use under the new boot.
func (e *Env) orphanSweep(st *envState, logf func(string, ...any)) {
	for name, ns := range st.Nets {
		if ns == nil || ns.Port == 0 {
			continue
		}
		if ns.Owner > 0 && alive(ns.Owner) {
			continue // rightful owner still holds it
		}
		// A live registry owner (another env, a re-reservation) keeps
		// the port — orphan detection only reaps dead owners.
		if b, err := os.ReadFile(filepath.Join(e.portsReg(), strconv.Itoa(ns.Port))); err == nil {
			owner := 0
			fmt.Sscanf(string(b), "%d", &owner)
			if owner > 0 && alive(owner) {
				ns.Port, ns.Owner = 0, 0
				logf("net %s: port taken by live pid %d — reallocating", name, owner)
				continue
			}
		}
		e.portsRelease(ns.Port) // dead-owner reservation is stale
		if !portsFree(ns.Port) {
			logf("net %s: port %d bound by another process — reallocating", name, ns.Port)
			ns.Port = 0
		}
		ns.Owner = 0
	}
}

// ensurePorts assigns a port to every socket/mcast net that lacks a
// live one, persisting into st. Port allocation happens before any VM
// boots so connect= arguments can be rendered for the whole env.
func (e *Env) ensurePorts(st *envState, logf func(string, ...any)) error {
	for _, n := range e.Nets {
		if n.Kind == "user" || len(n.Attach) == 0 {
			continue
		}
		ns := st.Nets[n.Name]
		if ns == nil {
			ns = &netState{Kind: n.Kind, Listener: n.listener()}
			st.Nets[n.Name] = ns
		}
		ns.Kind, ns.Listener = n.Kind, n.listener()
		if n.Port != 0 { // explicit pin always wins
			ns.Port = n.Port
		}
		if ns.Port != 0 {
			continue
		}
		p := e.portsFind()
		if p == 0 {
			return fmt.Errorf("net %s: no free port in %d-%d", n.Name, portLo, portHi)
		}
		ns.Port = p
		e.portsReserve(p, os.Getpid(), "env:"+e.Name+"/"+n.Name)
		logf("net %s: allocated port %d (%s)", n.Name, p, n.Kind)
	}
	return nil
}

// ---------- extra_args rendering ----------

// netdevID is the QEMU id for the vm's nth env net — deterministic so
// `quickemu-env` regenerates identical args across runs.
func netdevID(vm *VM, n int) string {
	return "qenv" + vm.Name + "n" + strconv.Itoa(n)
}

// netArgs renders the extra_args fragments a VM needs for env nets.
// nets is the resolved port map (net name -> port) from ensurePorts.
func netArgs(vm *VM, e *Env, st *envState) []string {
	var args []string
	idx := 0
	for _, n := range e.Nets {
		if !contains(n.Attach, vm.Name) {
			continue
		}
		id := netdevID(vm, idx)
		idx++
		switch n.Kind {
		case "socket":
			port := 0
			if ns := st.Nets[n.Name]; ns != nil {
				port = ns.Port
			}
			if port == 0 {
				continue
			}
			if n.listener() == vm.Name {
				args = append(args,
					"-netdev socket,id="+id+",listen=:"+strconv.Itoa(port),
					"-device "+netDevice+",netdev="+id)
			} else {
				args = append(args,
					"-netdev socket,id="+id+",connect=127.0.0.1:"+strconv.Itoa(port),
					"-device "+netDevice+",netdev="+id)
			}
		case "mcast":
			port := 0
			if ns := st.Nets[n.Name]; ns != nil {
				port = ns.Port
			}
			args = append(args,
				"-netdev socket,id="+id+",mcast="+n.mcastAddr(port),
				"-device "+netDevice+",netdev="+id)
		case "user":
			var fwds []string
			for _, f := range n.Forwards {
				if f.VM == "" || f.VM == vm.Name {
					fwds = append(fwds,
						"hostfwd=tcp::"+strconv.Itoa(f.Host)+"-:"+strconv.Itoa(f.Guest),
						"hostfwd=udp::"+strconv.Itoa(f.Host)+"-:"+strconv.Itoa(f.Guest))
				}
			}
			if len(fwds) == 0 {
				continue // plain user net == quickemu's default netdev
			}
			args = append(args,
				"-netdev user,id="+id+","+strings.Join(fwds, ","),
				"-device "+netDevice+",netdev="+id)
		}
	}
	return args
}

// shareArgs renders -virtfs fragments for shares attached to a VM.
// security_model=none keeps the guest's view of the files owned by
// the invoking user — the only model that needs no root and no xattr
// cooperation; ro shares get readonly=on.
func shareArgs(vm *VM, e *Env) []string {
	var args []string
	for _, s := range e.Shares {
		if !contains(s.Attach, vm.Name) {
			continue
		}
		p := s.Path
		if !filepath.IsAbs(p) {
			p = filepath.Join(e.Dir, p)
		}
		a := "-virtfs local,path=" + p + ",mount_tag=" + s.Name + ",security_model=none"
		if s.Mode == "ro" {
			a += ",readonly=on"
		}
		args = append(args, a)
	}
	return args
}

// extraArgs joins net + share fragments into the single --extra_args
// value handed to `quickemu --start`.
func extraArgs(vm *VM, e *Env, st *envState) string {
	return strings.Join(append(netArgs(vm, e, st), shareArgs(vm, e)...), " ")
}

// ---------- inventory view ----------

// netInfo is the inventory/status view of one env net.
type netInfo struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
	Port int    `json:"port,omitempty"`
	Role string `json:"role,omitempty"` // listen|connect|member (per-VM)
}

// vmNets lists the nets a VM belongs to with its role in each.
func vmNets(vm *VM, e *Env, st *envState) []netInfo {
	var out []netInfo
	for _, n := range e.Nets {
		if !contains(n.Attach, vm.Name) {
			continue
		}
		ni := netInfo{Name: n.Name, Kind: n.Kind}
		if ns := st.Nets[n.Name]; ns != nil {
			ni.Port = ns.Port
		}
		switch n.Kind {
		case "socket":
			if n.listener() == vm.Name {
				ni.Role = "listen"
			} else {
				ni.Role = "connect"
			}
		default:
			ni.Role = "member"
		}
		out = append(out, ni)
	}
	return out
}

// ---------- JSON helper for `ports list` debugging ----------

// portsList returns the raw registry contents (used by status -v and
// tests); absent sibling → empty.
func (e *Env) portsList() []map[string]any {
	if portsBin() == "" {
		return nil
	}
	out, err := runTool("quickemu-ports", "", nil, "list",
		"--registry", e.portsReg())
	if err != nil {
		return nil
	}
	var env struct {
		Data struct {
			Reservations []map[string]any `json:"reservations"`
		} `json:"data"`
	}
	if json.Unmarshal([]byte(out), &env) != nil {
		return nil
	}
	return env.Data.Reservations
}
