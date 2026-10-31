package main

import (
	"strings"
	"testing"
)

// dagChain is a <- b <- c plus standalone d.
func dagChain() *Env {
	return &Env{VMs: []*VM{
		{Name: "a"},
		{Name: "b", Deps: []string{"a"}},
		{Name: "c", Deps: []string{"b"}},
		{Name: "d"},
	}}
}

func TestLevelsChain(t *testing.T) {
	lvl := dagChain().levels()
	want := map[string]int{"a": 0, "b": 1, "c": 2, "d": 0}
	for name, w := range want {
		if lvl[name] != w {
			t.Fatalf("level(%s) = %d, want %d", name, lvl[name], w)
		}
	}
}

func TestLevelsSharedDeps(t *testing.T) {
	// c depends on both a (level 0) and b (level 1) -> level 2.
	e := &Env{VMs: []*VM{
		{Name: "a"},
		{Name: "b", Deps: []string{"a"}},
		{Name: "c", Deps: []string{"a", "b"}},
	}}
	if lvl := e.levels(); lvl["c"] != 2 {
		t.Fatalf("level(c) = %d, want 2", lvl["c"])
	}
}

func TestLevelsUnknownDepSkipped(t *testing.T) {
	e := &Env{VMs: []*VM{{Name: "a", Deps: []string{"ghost"}}}}
	if lvl := e.levels(); lvl["a"] != 0 {
		t.Fatalf("level(a) = %d, want 0", lvl["a"])
	}
}

func TestLevelsCyclicInputTerminates(t *testing.T) {
	// A cyclic env never reaches the scheduler, but levels() must not
	// hang if called anyway — the depth guard kicks in.
	e := &Env{VMs: []*VM{
		{Name: "a", Deps: []string{"b"}},
		{Name: "b", Deps: []string{"a"}},
	}}
	done := make(chan map[string]int, 1)
	go func() { done <- e.levels() }()
	lvl := <-done
	if len(lvl) != 2 {
		t.Fatalf("levels = %v", lvl)
	}
}

func TestCycleNone(t *testing.T) {
	if c := dagChain().cycle(); c != nil {
		t.Fatalf("unexpected cycle %v", c)
	}
}

func TestCycleFindsLoop(t *testing.T) {
	e := &Env{VMs: []*VM{
		{Name: "a", Deps: []string{"b"}},
		{Name: "b", Deps: []string{"c"}},
		{Name: "c", Deps: []string{"a"}},
		{Name: "d"},
	}}
	c := e.cycle()
	if len(c) != 4 {
		t.Fatalf("cycle = %v, want 4 entries", c)
	}
	// Cycle order wraps around: last element repeats the first.
	if c[0] != c[len(c)-1] {
		t.Fatalf("cycle does not close: %v", c)
	}
	joined := strings.Join(c, " -> ")
	if !strings.Contains(joined, "a -> b -> c -> a") &&
		!strings.Contains(joined, "b -> c -> a -> b") &&
		!strings.Contains(joined, "c -> a -> b -> c") {
		t.Fatalf("cycle = %v", c)
	}
}

func TestCycleSelfLoop(t *testing.T) {
	e := &Env{VMs: []*VM{{Name: "a", Deps: []string{"a"}}}}
	c := e.cycle()
	if len(c) != 2 || c[0] != "a" || c[1] != "a" {
		t.Fatalf("self cycle = %v", c)
	}
}

func TestCycleUnknownDepSkipped(t *testing.T) {
	e := &Env{VMs: []*VM{{Name: "a", Deps: []string{"ghost"}}}}
	if c := e.cycle(); c != nil {
		t.Fatalf("unexpected cycle %v", c)
	}
}

func TestSelectVMsAllOnEmpty(t *testing.T) {
	vms, errs := dagChain().selectVMs(nil)
	if len(errs) > 0 || len(vms) != 4 {
		t.Fatalf("nil selection: vms=%d errs=%v", len(vms), errs)
	}
	vms, errs = dagChain().selectVMs([]string{})
	if len(errs) > 0 || len(vms) != 4 {
		t.Fatalf("empty selection: vms=%d errs=%v", len(vms), errs)
	}
}

func TestSelectVMsUnknown(t *testing.T) {
	_, errs := dagChain().selectVMs([]string{"zzz"})
	if len(errs) != 1 || !strings.Contains(errs[0].msg, `unknown vm "zzz"`) {
		t.Fatalf("errs = %v", errs)
	}
}

func TestSelectVMsPullsTransitiveDeps(t *testing.T) {
	vms, errs := dagChain().selectVMs([]string{"c"})
	if len(errs) > 0 {
		t.Fatalf("errs = %v", errs)
	}
	got := map[string]bool{}
	for _, v := range vms {
		got[v.Name] = true
	}
	if len(vms) != 3 || !got["a"] || !got["b"] || !got["c"] {
		t.Fatalf("selection = %v", got)
	}
}

func TestSelectVMsDeclarationOrder(t *testing.T) {
	vms, _ := dagChain().selectVMs([]string{"c", "a"})
	var names []string
	for _, v := range vms {
		names = append(names, v.Name)
	}
	if strings.Join(names, ",") != "a,b,c" {
		t.Fatalf("order = %v", names)
	}
}

func TestOrderGroupsByLevel(t *testing.T) {
	e := dagChain()
	vms, _ := e.selectVMs(nil)
	levels := e.order(vms)
	if len(levels) != 3 {
		t.Fatalf("levels = %d", len(levels))
	}
	if len(levels[0]) != 2 || len(levels[1]) != 1 || len(levels[2]) != 1 {
		t.Fatalf("level sizes = %d/%d/%d",
			len(levels[0]), len(levels[1]), len(levels[2]))
	}
	if levels[1][0].Name != "b" || levels[2][0].Name != "c" {
		t.Fatalf("level members wrong: %v %v", levels[1], levels[2])
	}
	// Level 0 holds a and d in declaration order.
	if levels[0][0].Name != "a" || levels[0][1].Name != "d" {
		t.Fatalf("level 0 members = %v", levels[0])
	}
}

func TestOrderEmptySelection(t *testing.T) {
	e := dagChain()
	levels := e.order(nil)
	if len(levels) != 1 || len(levels[0]) != 0 {
		t.Fatalf("levels = %v", levels)
	}
}

func TestOrderSelectedSkipsHigherLevels(t *testing.T) {
	e := dagChain()
	vms, _ := e.selectVMs([]string{"a", "d"})
	levels := e.order(vms)
	if len(levels) != 1 || len(levels[0]) != 2 {
		t.Fatalf("levels = %v", levels)
	}
}
