package contractapi

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestCrossCallWireGasAndEventPrefix(t *testing.T) {
	Reset()
	InitGas(100)
	ConsumeGas(11)
	Log("before")
	var output bytes.Buffer
	InitObjectIO(strings.NewReader(`{"version":3,"ok":true,"result":"Nw==","gas":15}`+"\n"), &output)
	value, err := Call("child", "F", uint64(18446744073709551615))
	if err != nil || string(value) != "7" {
		t.Fatalf("call=%s %v", value, err)
	}
	var req CrossCallRequest
	dec := json.NewDecoder(&output)
	dec.UseNumber()
	if err := dec.Decode(&req); err != nil {
		t.Fatal(err)
	}
	if req.Version != ProtocolVersion || req.Type != "call" || req.Address != "child" || len(req.Events) != 1 || req.Events[0].Name != "before" {
		t.Fatalf("req=%+v", req)
	}
	if *req.Gas != 23 || GasUsed() != 38 || len(DrainEvents()) != 0 {
		t.Fatalf("gas=%d", GasUsed())
	}
}
func TestCrossCallLocalFailureCannotBeIgnored(t *testing.T) {
	var output bytes.Buffer
	InitObjectIO(strings.NewReader(""), &output)
	ResetGas()
	if _, err := Call("child", "F", make(chan int)); err == nil {
		t.Fatal("invalid argument accepted")
	}
	if CallFailure() == nil {
		t.Fatal("ignored call failure not latched")
	}
}
