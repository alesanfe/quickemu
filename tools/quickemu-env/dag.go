// dag.go: dependency resolution for [[vm]].depends_on — cycle
// detection that reports the offending loop, and level assignment for
// the up/down schedulers: level(v) = 1 + max(level(dep)); VMs at the
// same level start in parallel, levels run sequentially.
package main

import "strconv"

// cycle finds one dependency loop via DFS, returning the names in
// cycle order (a -> b -> a) or nil. Only the first cycle found is
// reported — validate surfaces a single actionable error.
func (e *Env) cycle() []string {
	const (
		white = iota // unvisited
		grey         // on the DFS stack
		black        // fully explored
	)
	mark := map[string]int{}
	var stack []string
	var found []string
	var visit func(v *VM) bool
	visit = func(v *VM) bool {
		mark[v.Name] = grey
		stack = append(stack, v.Name)
		for _, d := range v.Deps {
			dep := e.vm(d)
			if dep == nil {
				continue // unknown dep — semantic() reports it
			}
			switch mark[d] {
			case grey:
				// The loop is the slice from d's stack slot to the top.
				i := len(stack) - 1
				for stack[i] != d {
					i--
				}
				found = append(append([]string{}, stack[i:]...), d)
				return true
			case white:
				if visit(dep) {
					return true
				}
			}
		}
		stack = stack[:len(stack)-1]
		mark[v.Name] = black
		return false
	}
	for _, v := range e.VMs {
		if mark[v.Name] == white && visit(v) {
			return found
		}
	}
	return nil
}

// levels assigns each VM a start level (0 = no deps). Invalid
// references are skipped — semantic() already reports them; a cyclic
// env never reaches the scheduler (validate/up gate on errors).
func (e *Env) levels() map[string]int {
	lvl := map[string]int{}
	var resolve func(v *VM, depth int) int
	resolve = func(v *VM, depth int) int {
		if l, ok := lvl[v.Name]; ok {
			return l
		}
		if depth > len(e.VMs) { // defensive: cyclic input
			return 0
		}
		max := 0
		for _, d := range v.Deps {
			if dep := e.vm(d); dep != nil {
				if l := resolve(dep, depth+1) + 1; l > max {
					max = l
				}
			}
		}
		lvl[v.Name] = max
		return max
	}
	for _, v := range e.VMs {
		resolve(v, 0)
	}
	return lvl
}

// selectVMs narrows the env to the requested VM names plus their
// transitive dependencies (docker-compose `up svc` semantics — a VM
// never starts without its deps). A nil/empty selection keeps all.
// Unknown names are returned as errors.
func (e *Env) selectVMs(names []string) ([]*VM, envErrs) {
	var errs envErrs
	want := map[string]bool{}
	for _, n := range names {
		if e.vm(n) == nil {
			errs = append(errs, envErr{0, 0, "unknown vm " + strconv.Quote(n)})
			continue
		}
		want[n] = true
	}
	if len(errs) > 0 {
		return nil, errs
	}
	if len(want) == 0 {
		return e.VMs, nil
	}
	// close over transitive deps
	var addDeps func(v *VM)
	addDeps = func(v *VM) {
		for _, d := range v.Deps {
			if dep := e.vm(d); dep != nil && !want[d] {
				want[d] = true
				addDeps(dep)
			}
		}
	}
	for _, n := range names {
		addDeps(e.vm(n))
	}
	var out []*VM
	for _, v := range e.VMs {
		if want[v.Name] {
			out = append(out, v)
		}
	}
	return out, nil
}

// order groups the given VMs into levels for scheduling. Levels are
// lists of VMs in declaration order; reverse gives the down order.
func (e *Env) order(vms []*VM) [][]*VM {
	lvl := e.levels()
	max := 0
	for _, v := range vms {
		if lvl[v.Name] > max {
			max = lvl[v.Name]
		}
	}
	out := make([][]*VM, max+1)
	for _, v := range vms {
		out[lvl[v.Name]] = append(out[lvl[v.Name]], v)
	}
	return out
}
