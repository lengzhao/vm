package contractapi

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestDefaultObjectRPCAndGas(t *testing.T) {
	ResetGas()
	InitGas(100)
	replies := `{"version":3,"ok":true,"exists":true,"value":"dmFsdWU="}` + "\n" + `{"version":3,"ok":true,"exists":false,"value":null}` + "\n" + `{"version":3,"ok":true,"exists":false,"value":null}` + "\n"
	var output bytes.Buffer
	InitObjectIO(strings.NewReader(replies), &output)
	object := DefaultObject()
	value, exists, err := object.Get("field")
	if err != nil || !exists || string(value) != "value" {
		t.Fatalf("Get=%q %v %v", value, exists, err)
	}
	if err := object.Set("field", []byte("next")); err != nil {
		t.Fatal(err)
	}
	if err := object.Delete("field"); err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(&output)
	for _, op := range []string{"get", "set", "delete"} {
		var req ObjectRequest
		if err := decoder.Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.Version != ProtocolVersion || req.Type != "object" || req.Op != op || req.Key != "field" {
			t.Fatalf("request=%+v", req)
		}
	}
	if GasUsed() != GasObjectGet+GasObjectSet+GasObjectDelete {
		t.Fatal(GasUsed())
	}
}
func TestObjectRejectsMalformedRepliesAndLimits(t *testing.T) {
	for _, reply := range []string{`{}`, `{"version":1,"ok":true}`, `{"version":3,"ok":false,"error":"failed"}`, `{"version":3,"ok":true,"exists":true,"value":"!"}`, `{"version":3,"ok":true,"exists":true,"value":"eA==","extra":1}`} {
		var output bytes.Buffer
		InitObjectIO(strings.NewReader(reply+"\n"), &output)
		ResetGas()
		if _, _, err := DefaultObject().Get("field"); err == nil {
			t.Fatalf("accepted %s", reply)
		}
	}
	var output bytes.Buffer
	InitObjectIO(strings.NewReader(""), &output)
	ResetGas()
	if err := DefaultObject().Set("field", make([]byte, MaxObjectValueBytes+1)); err == nil {
		t.Fatal("large value accepted")
	}
	if _, _, err := DefaultObject().Get(strings.Repeat("x", MaxObjectKeyBytes+1)); err == nil {
		t.Fatal("large key accepted")
	}
	if _, _, err := DefaultObject().Get(string([]byte{255})); err == nil {
		t.Fatal("invalid utf8 key accepted")
	}
	if output.Len() != 0 {
		t.Fatal("invalid input sent to host")
	}
}
