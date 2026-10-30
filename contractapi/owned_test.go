package contractapi

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestOwnedObjectWireAndGas(t *testing.T) {
	InitGas(0)
	var wire bytes.Buffer
	replies := strings.Repeat("{\"version\":3,\"ok\":true,\"exists\":false,\"value\":null,\"owner\":\"alice\"}\n", 5)
	InitObjectIO(strings.NewReader(replies), &wire)
	o, e := CreateObject("x")
	if e != nil {
		t.Fatal(e)
	}
	if e = o.Set("k", []byte("v")); e != nil {
		t.Fatal(e)
	}
	if owner, e := o.Owner(); e != nil || owner != "alice" {
		t.Fatal(owner, e)
	}
	if e = o.Transfer("bob"); e != nil {
		t.Fatal(e)
	}
	if e = o.Destroy(); e != nil {
		t.Fatal(e)
	}
	dec := json.NewDecoder(&wire)
	for _, op := range []string{"create", "set", "owner", "transfer", "destroy"} {
		var r ObjectRequest
		if e = dec.Decode(&r); e != nil || r.Op != op || r.ID != "x" {
			t.Fatal(r, e)
		}
	}
	if GasUsed() != GasObjectCreate+GasObjectSet+GasObjectGet+GasObjectTransfer+GasObjectDestroy {
		t.Fatal(GasUsed())
	}
}

func TestOwnedIDEncodingAndDefaultSeparation(t *testing.T) {
	InitGas(0)
	var wire bytes.Buffer
	InitObjectIO(strings.NewReader(`{"version":3,"ok":true,"exists":false,"value":null,"owner":"alice"}`+"\n"), &wire)
	if _, _, e := OpenObject("").Get("k"); e == nil || wire.Len() != 0 {
		t.Fatal("empty ID reached default object", e)
	}
	if _, e := CreateObject("\x00"); e != nil {
		t.Fatal("valid UTF-8 ID rejected", e)
	}
}
