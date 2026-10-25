package gas

import "testing"

func TestObjectMethodGasProfile(t *testing.T) {
	profile, err := BuildGasProfile(`package main
 import api "github.com/lengzhao/vm/contractapi"
 func F(){o:=api.DefaultObject();o.Get("x");o.Set("x",nil);api.DefaultObject().Delete("x")}`)
	if err != nil {
		t.Fatal(err)
	}
	calls := profile.Functions["F"].APICalls
	if calls["Object.Get"] != 1 || calls["Object.Set"] != 1 || calls["Object.Delete"] != 1 {
		t.Fatalf("calls=%v", calls)
	}
	_, parts, err := EstimateFromProfile(profile, "F", 1)
	if err != nil || parts["api"] != 20 {
		t.Fatalf("parts=%v err=%v", parts, err)
	}
}
