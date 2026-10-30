package vm

import (
	"testing"
)

func TestBuildGasProfile_Info(t *testing.T) {
	src := `
package main
import "github.com/lengzhao/vm/contractapi"
func Info() (uint64, string) {
	h := contractapi.BlockHeight()
	s := contractapi.Sender()
	contractapi.Log("InfoCalled", "sender", s)
	return h, s
}
`
	p, err := BuildGasProfile(src)
	if err != nil {
		t.Fatal(err)
	}
	if p.Version != 1 || p.EntryGas != 10 {
		t.Fatalf("version=%d entry_gas=%d", p.Version, p.EntryGas)
	}
	f := p.Functions["Info"]
	if f.FuncEntries < 1 || f.APICalls["BlockHeight"] != 1 || f.APICalls["Sender"] != 1 || f.APICalls["Log"] != 1 {
		t.Fatalf("%+v", f)
	}
	if f.LoopSites != 0 {
		t.Fatalf("loop_sites=%d", f.LoopSites)
	}
}

func TestBuildGasProfile_Loop(t *testing.T) {
	src := `
package main
func Sum(n int) int {
	s := 0
	for i := 0; i < n; i++ {
		s += i
	}
	return s
}
`
	p, err := BuildGasProfile(src)
	if err != nil {
		t.Fatal(err)
	}
	f := p.Functions["Sum"]
	if f.LoopSites != 1 {
		t.Fatalf("loop_sites=%d want 1", f.LoopSites)
	}
}

func TestBuildGasProfile_HelperCall(t *testing.T) {
	src := `
package main
func helper(x int) int { return x + 1 }
func Add(a, b int) int {
	return helper(a) + b
}
`
	p, err := BuildGasProfile(src)
	if err != nil {
		t.Fatal(err)
	}
	f := p.Functions["Add"]
	if f.FuncEntries < 2 {
		t.Fatalf("func_entries=%d want >= 2", f.FuncEntries)
	}
	if _, ok := p.Functions["helper"]; ok {
		t.Fatal("unexported helper should not have its own profile")
	}
}

func TestEstimateFromProfile(t *testing.T) {
	p := &GasProfile{
		Version:  1,
		EntryGas: 10,
		Functions: map[string]FunctionGasProfile{
			"Info": {
				FuncEntries: 1,
				LoopSites:   0,
				APICalls: map[string]int{
					"BlockHeight": 1,
					"Sender":      1,
					"Log":         1,
				},
			},
			"Loop": {
				FuncEntries: 1,
				LoopSites:   1,
				APICalls:    map[string]int{},
			},
		},
	}

	est, br, err := EstimateFromProfile(p, "Info", 0)
	if err != nil {
		t.Fatal(err)
	}
	// entry 10 + funcs 1 + loops 0 + api (1+1+2) = 15
	if est != 15 {
		t.Fatalf("Info estimated=%d want 15 breakdown=%v", est, br)
	}
	if br["entry"] != 10 || br["funcs"] != 1 || br["loops"] != 0 || br["api"] != 4 {
		t.Fatalf("Info breakdown=%v", br)
	}

	est, br, err = EstimateFromProfile(p, "Loop", 0)
	if err != nil {
		t.Fatal(err)
	}
	// entry 10 + funcs 1 + loops 1*1000 = 1011
	if est != 1011 {
		t.Fatalf("Loop default bound estimated=%d want 1011 breakdown=%v", est, br)
	}
	if br["loops"] != 1000 {
		t.Fatalf("loops breakdown=%d want 1000", br["loops"])
	}

	est, br, err = EstimateFromProfile(p, "Loop", 5)
	if err != nil {
		t.Fatal(err)
	}
	if est != 16 || br["loops"] != 5 {
		t.Fatalf("Loop bound=5 estimated=%d breakdown=%v", est, br)
	}

	if _, _, err := EstimateFromProfile(p, "Missing", 0); err == nil {
		t.Fatal("expected error for missing function")
	}
}
