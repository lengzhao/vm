package gas

import (
	"github.com/lengzhao/vm/contractapi"
	"testing"
)

func TestOwnedObjectStaticCosts(t *testing.T) {
	for _, tc := range []struct {
		name, body, api string
		cost            uint64
	}{
		{"create", `_,e:=api.CreateObject(id);return e`, "CreateObject", contractapi.GasObjectCreate},
		{"owner", `_,e:=api.OpenObject(id).Owner();return e`, "Object.Owner", contractapi.GasObjectGet},
		{"transfer", `return api.OpenObject(id).Transfer("bob")`, "Object.Transfer", contractapi.GasObjectTransfer},
		{"destroy", `return api.OpenObject(id).Destroy()`, "Object.Destroy", contractapi.GasObjectDestroy},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, e := BuildGasProfile(`package main; import api "github.com/lengzhao/vm/contractapi"; func Run(id string) error {` + tc.body + `}`)
			if e != nil {
				t.Fatal(e)
			}
			if p.Functions["Run"].APICalls[tc.api] != 1 {
				t.Fatalf("missing %s: %+v", tc.api, p.Functions["Run"])
			}
			got, b, e := EstimateFromProfile(p, "Run", 0)
			if e != nil || got != 11+tc.cost || b["api"] != tc.cost {
				t.Fatalf("estimate=%d breakdown=%v err=%v", got, b, e)
			}
		})
	}
}
