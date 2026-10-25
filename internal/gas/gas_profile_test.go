package gas

import (
	"github.com/lengzhao/vm/core"
	"testing"
)

func TestEstimateRejectsUnsupportedProfileVersion(t *testing.T) {
	for _, version := range []int{0, -1, 2} {
		p := &core.GasProfile{Version: version, Functions: map[string]core.FunctionGasProfile{"Add": {FuncEntries: 1}}}
		if _, _, err := EstimateFromProfile(p, "Add", 0); err == nil {
			t.Errorf("unsupported profile version %d accepted", version)
		}
	}
}
func TestBuilderProducesSupportedProfile(t *testing.T) {
	p, err := BuildGasProfile("package main\nfunc Add(a,b int)int{return a+b}\n")
	if err != nil {
		t.Fatal(err)
	}
	if p.Version != 1 {
		t.Fatalf("profile version=%d, want 1", p.Version)
	}
	if _, _, err := EstimateFromProfile(p, "Add", 0); err != nil {
		t.Fatal(err)
	}
}
