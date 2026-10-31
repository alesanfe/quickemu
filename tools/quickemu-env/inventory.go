// inventory.go: QE-073 — DNS hints and machine-readable inventory.
//
// Guest IP resolution is best-effort, in order:
//  1. a QGA `ip` verb, when a quickemu-qga binary happens to be in
//     PATH (the qga module is owned elsewhere — absence degrades
//     silently to step 2). The probe is `quickemu-qga ip --socket
//     <vmdir>/<vm>-agent.sock`, first IPv4 token wins.
//  2. 127.0.0.1 for any VM with a host-side port forward (the
//     ssh hostfwd / .ports records) — a forwarded port IS the
//     host-reachable address.
//  3. "" — socket/mcast guests number themselves; nothing on the
//     host can know without QGA.
package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
)

// qgaIP probes the guest agent for an address. The verb contract is
// deliberately loose — any first IPv4 on stdout counts.
func qgaIP(conf string) string {
	bin, err := findTool("quickemu-qga")
	if err != nil {
		return ""
	}
	sock := agentSock(conf)
	if _, err := os.Stat(sock); err != nil {
		return ""
	}
	cmd := execCommand(bin, "ip", "--socket", sock)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	for _, tok := range strings.Fields(string(out)) {
		tok = strings.Trim(tok, `",[]{}`)
		if ip := net.ParseIP(tok); ip != nil && ip.To4() != nil &&
			!ip.IsLoopback() && !ip.IsLinkLocalUnicast() {
			return tok
		}
	}
	return ""
}

// guestIP resolves a VM's reachable address ("" when unknowable).
func guestIP(e *Env, conf string, rt vmRuntime) string {
	if conf == "" {
		return ""
	}
	if ip := qgaIP(conf); ip != "" {
		return ip
	}
	if sshPort(conf, rt) != 0 || len(rt.Ports) > 0 {
		return "127.0.0.1"
	}
	return ""
}

// ---------- hosts ----------

// cmdHosts prints a self-delimiting /etc/hosts block — append with
// `quickemu-env hosts | sudo tee -a /etc/hosts`, or diff it first.
func cmdHosts(e *Env, sel []string) int {
	vms, errs := e.selectVMs(sel)
	if len(errs) > 0 {
		return printErrs(e.File, errs)
	}
	st := e.loadState()
	fmt.Printf("# >>> quickemu-env %s — BEGIN\n", e.Name)
	for _, vm := range vms {
		conf := vmConf(e, st, vm)
		rt := inspect(conf)
		ip := guestIP(e, conf, rt)
		if ip == "" {
			fmt.Printf("# %-18s %s  # no host-reachable address\n", "-", vm.Name)
			continue
		}
		fmt.Printf("%-18s %s\n", ip, vm.Name)
	}
	fmt.Printf("# >>> quickemu-env %s — END\n", e.Name)
	return 0
}

// ---------- inventory ----------

// inventory builds the vm -> {state, ip, ports, deps} map both
// --json and -o ansible render.
func inventory(e *Env, st *envState, vms []*VM) map[string]any {
	out := map[string]any{}
	for _, vm := range vms {
		conf := vmConf(e, st, vm)
		rt := inspect(conf)
		rec := map[string]any{
			"state": rt.State,
			"deps":  vm.Deps,
			"conf":  conf,
		}
		if rt.PID > 0 {
			rec["pid"] = rt.PID
		}
		if ip := guestIP(e, conf, rt); ip != "" {
			rec["ip"] = ip
		}
		if len(rt.Ports) > 0 {
			rec["ports"] = rt.Ports
		}
		if nets := vmNets(vm, e, st); len(nets) > 0 {
			rec["nets"] = nets
		}
		out[vm.Name] = rec
	}
	return out
}

// cmdInventory implements `quickemu-env inventory [--json|-o ansible]`.
func cmdInventory(e *Env, sel []string, jsonOut bool, format string) int {
	vms, errs := e.selectVMs(sel)
	if len(errs) > 0 {
		return printErrs(e.File, errs)
	}
	st := e.loadState()
	inv := inventory(e, st, vms)
	switch {
	case format == "ansible":
		return renderAnsible(e, st, vms, inv)
	case jsonOut || format == "json":
		b, _ := json.MarshalIndent(map[string]any{
			"env": e.Name, "vms": inv}, "", "  ")
		fmt.Println(string(b))
		return 0
	default:
		// human table — the same data, transposed for reading.
		fmt.Printf("%-20s %-9s %-8s %-15s %s\n", "VM", "STATE", "PID", "IP", "DEPS")
		names := make([]string, 0, len(inv))
		for n := range inv {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			rec := inv[n].(map[string]any)
			pid := ""
			if p, ok := rec["pid"].(int); ok {
				pid = strconv.Itoa(p)
			}
			ip, _ := rec["ip"].(string)
			fmt.Printf("%-20s %-9s %-8s %-15s %s\n",
				n, rec["state"], pid, ip, strings.Join(e.vm(n).Deps, ","))
		}
		return 0
	}
}

// renderAnsible emits an INI inventory: each VM a host line carrying
// ansible_host/ansible_port when a forward exists, plus a group per
// env name so `ansible env_demo -m ping` addresses the whole set.
func renderAnsible(e *Env, st *envState, vms []*VM, inv map[string]any) int {
	grp := "env_" + strings.NewReplacer("-", "_", ".", "_").Replace(e.Name)
	fmt.Printf("[%s]\n", grp)
	for _, vm := range vms {
		conf := vmConf(e, st, vm)
		rt := inspect(conf)
		parts := []string{vm.Name}
		if ip := guestIP(e, conf, rt); ip != "" {
			parts = append(parts, "ansible_host="+ip)
			if p := sshPort(conf, rt); p != 0 {
				parts = append(parts, "ansible_port="+strconv.Itoa(p))
			}
		} else {
			parts = append(parts, "ansible_host="+vm.Name,
				"# no host-reachable address — needs a hosts entry or forward")
		}
		fmt.Println(strings.Join(parts, " "))
	}
	fmt.Printf("\n[%s:vars]\nansible_user=user\nansible_ssh_common_args='-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null'\n", grp)
	return 0
}
