package vm

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/lengzhao/vm/core"
)

const crossSource = `package main
import (
 "errors"
 "github.com/lengzhao/vm/contractapi"
)
func Read()(string,bool,error){v,ok,err:=contractapi.DefaultObject().Get("x");return string(v),ok,err}
func Child(v string)(string,error){if err:=contractapi.DefaultObject().Set("x",[]byte(v));err!=nil{return "",err};contractapi.Log("child");return v,nil}
func Fail(v string)error{Child(v);return errors.New("child failed")}
func Burn(v string){Child(v);for i:=0;i<10000;i++{}}
func Spin(v string){Child(v);for {}}
func Info(v string)(string,string,uint64,uint64){return contractapi.Sender(),contractapi.ContractAddress(),contractapi.BlockHeight(),contractapi.BlockTime()}
func Relay(next string,fn string,v string)(string,error){contractapi.DefaultObject().Set("x",[]byte("parent"));contractapi.Log("before");data,err:=contractapi.Call(next,fn,v);contractapi.Log("after");return string(data),err}
func Ignore(next string,fn string,v string)string{contractapi.DefaultObject().Set("x",[]byte("ignored"));contractapi.Call(next,fn,v);return "ignored error"}
func Bounce(next string)(string,error){_,err:=contractapi.Call(next,"Bounce",contractapi.ContractAddress());return "",err}
func ReadBack(next string)(string,error){if _,err:=contractapi.Call(next,"Child","pending");err!=nil{return "",err};data,err:=contractapi.Call(next,"Read");return string(data),err}
func Twice(next string)(string,error){contractapi.Log("before");if _,err:=contractapi.Call(next,"Child","one");err!=nil{return "",err};contractapi.Log("middle");data,err:=contractapi.Call(next,"Child","two");contractapi.Log("after");return string(data),err}
`

func deployCrossPair(t *testing.T) (*VMEngine, string, string) {
	t.Helper()
	e := NewVMEngine(VMConfig{ContractStorageDir: t.TempDir(), EnableSecurityChecks: true, EnableGasMetering: true, MaxGasLimit: 1000, ExecutionTimeout: 5 * time.Second})
	c, err := e.Compile(crossSource)
	if err != nil {
		t.Fatal(err)
	}
	a, err := e.DeployWithInput(c, core.DeploymentInput{ChainID: "test", Sender: "deployer", Nonce: 1})
	if err != nil {
		t.Fatal(err)
	}
	b, err := e.DeployWithInput(c, core.DeploymentInput{ChainID: "test", Sender: "deployer", Nonce: 2})
	if err != nil {
		t.Fatal(err)
	}
	return e, a, b
}
func TestCrossContractTemporaryStoreCommitAndDryRun(t *testing.T) {
	e, a, b := deployCrossPair(t)
	result, err := e.ExecuteWithContext(a, "Relay", nil, b, "Child", "stored")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Events) != 3 || result.Events[0].Name != "before" || result.Events[1].Name != "child" || result.Events[2].Name != "after" {
		t.Fatalf("events=%v", result.Events)
	}
	for _, item := range []struct{ address, want string }{{a, `["parent",true]`}, {b, `["stored",true]`}} {
		data, err := e.Execute(item.address, "Read")
		if err != nil || string(data) != item.want {
			t.Fatalf("Read=%s %v", data, err)
		}
	}
	if _, err := e.EstimateGas(a, "Relay", &EstimateOptions{DryRun: true, Args: []any{b, "Child", "dry"}}); err != nil {
		t.Fatal(err)
	}
	if data, err := e.Execute(b, "Read"); err != nil || string(data) != `["stored",true]` {
		t.Fatalf("dry run published: %s %v", data, err)
	}
	result, err = e.ExecuteWithContext(a, "Twice", nil, b)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Events) != 5 || result.Events[0].Name != "before" || result.Events[1].Name != "child" || result.Events[2].Name != "middle" || result.Events[3].Name != "child" || result.Events[4].Name != "after" {
		t.Fatalf("sequential child events=%v", result.Events)
	}
	if data, err := e.Execute(b, "Read"); err != nil || string(data) != `["two",true]` {
		t.Fatalf("second call not visible: %s %v", data, err)
	}
}
func TestCrossContractFailuresDiscardEntireTemporaryStore(t *testing.T) {
	e, a, b := deployCrossPair(t)
	for _, test := range []struct {
		parent, child, target string
		code                  core.ErrorCode
	}{{"Relay", "Fail", b, core.CodeContractError}, {"Ignore", "Fail", b, core.CodeContractError}, {"Relay", "Burn", b, core.CodeGasLimitExceeded}, {"Relay", "Child", a, core.CodeReentrancy}, {"Relay", "Missing", b, core.CodeUnknownFunction}} {
		out, err := e.Engine.Execute(context.Background(), core.ExecutionRequest{Address: a, Function: test.parent, Args: []any{test.target, test.child, "failed"}})
		requireCode(t, err, test.code)
		if out != nil && (len(out.Events) > 0 || len(out.Data) > 0) {
			t.Fatalf("failure published %+v", out)
		}
		for _, address := range []string{a, b} {
			if data, err := e.Execute(address, "Read"); err != nil || string(data) != `["",false]` {
				t.Fatalf("partial commit: %s %v", data, err)
			}
		}
	}
}
func TestCrossContractSenderAndBlockSnapshot(t *testing.T) {
	e, a, b := deployCrossPair(t)
	result, err := e.ExecuteWithContext(a, "Relay", &CallContext{Sender: AddressPtr("origin"), ContractAddress: AddressPtr("display"), BlockHeight: Uint64Ptr(42), BlockTime: Uint64Ptr(99)}, b, "Info", "")
	if err != nil {
		t.Fatal(err)
	}
	var encoded string
	if err := json.Unmarshal(result.Data, &encoded); err != nil {
		t.Fatal(err)
	}
	want, _ := json.Marshal([]any{a, b, 42, 99})
	if encoded != string(want) {
		t.Fatalf("context=%s want %s", encoded, want)
	}
}

func TestCrossContractDepthBoundary(t *testing.T) {
	e := NewVMEngine(VMConfig{ContractStorageDir: t.TempDir(), EnableSecurityChecks: true, EnableGasMetering: true, MaxGasLimit: 100000})
	source := `package main
 import "github.com/lengzhao/vm/contractapi"
 func Chain(path string)(string,error){contractapi.DefaultObject().Set("x",[]byte("chain"));if path==""{return "done",nil};i:=0;for i<len(path)&&path[i]!=';'{i++};if i==len(path){return "done",nil};_,err:=contractapi.Call(path[:i],"Chain",path[i+1:]);return "done",err}
 func Read()(string,bool,error){v,ok,err:=contractapi.DefaultObject().Get("x");return string(v),ok,err}`
	c, err := e.Compile(source)
	if err != nil {
		t.Fatal(err)
	}
	addresses := make([]string, 9)
	for i := range addresses {
		addresses[i], err = e.DeployWithInput(c, core.DeploymentInput{ChainID: "depth", Sender: "alice", Nonce: uint64(i)})
		if err != nil {
			t.Fatal(err)
		}
	}
	path := strings.Join(addresses[1:8], ";") + ";"
	if _, err := e.Execute(addresses[0], "Chain", path); err != nil {
		t.Fatalf("8 frames rejected: %v", err)
	}
	path = strings.Join(addresses[1:], ";") + ";"
	_, err = e.Execute(addresses[0], "Chain", path)
	requireCode(t, err, core.CodeCallDepth)
	if data, err := e.Execute(addresses[8], "Read"); err != nil || string(data) != `["",false]` {
		t.Fatalf("ninth frame ran: %s %v", data, err)
	}
}
func TestCrossContractPendingWritesAndIndirectReentry(t *testing.T) {
	e, a, b := deployCrossPair(t)
	data, err := e.Execute(a, "ReadBack", b)
	if err != nil {
		t.Fatal(err)
	}
	var encoded string
	if err := json.Unmarshal(data, &encoded); err != nil {
		t.Fatal(err)
	}
	if encoded != `["pending",true]` {
		t.Fatalf("child missed temporary write: %s", encoded)
	}
	_, err = e.Execute(a, "Bounce", b)
	requireCode(t, err, core.CodeReentrancy)
}
func TestCrossContractDeterministicReplay(t *testing.T) {
	var reference *ExecuteResult
	var rootAddress string
	for i := 0; i < 2; i++ {
		e := NewVMEngine(VMConfig{ContractStorageDir: t.TempDir(), RequireDeterminism: true, EnableGasMetering: true, MaxGasLimit: 1000})
		c, err := e.Compile(crossSource)
		if err != nil {
			t.Fatal(err)
		}
		a, err := e.DeployWithInput(c, core.DeploymentInput{ChainID: "replay", Sender: "alice", Nonce: 1})
		if err != nil {
			t.Fatal(err)
		}
		b, err := e.DeployWithInput(c, core.DeploymentInput{ChainID: "replay", Sender: "alice", Nonce: 2})
		if err != nil {
			t.Fatal(err)
		}
		result, err := e.ExecuteWithContext(a, "Relay", nil, b, "Child", "same")
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			reference = result
			rootAddress = a
		} else if a != rootAddress || !reflect.DeepEqual(reference, result) {
			t.Fatalf("replay differed: %+v %+v", reference, result)
		}
	}
}
func TestCrossContractGasChargedExactlyOnce(t *testing.T) {
	e, a, b := deployCrossPair(t)
	child, err := e.EstimateGas(b, "Child", &EstimateOptions{DryRun: true, Args: []any{"v"}})
	if err != nil {
		t.Fatal(err)
	}
	parent, err := e.EstimateGas(a, "Relay", &EstimateOptions{DryRun: true, Args: []any{b, "Child", "v"}})
	if err != nil {
		t.Fatal(err)
	}
	if parent.Estimated != 35+child.Estimated {
		t.Fatalf("parent=%d child=%d", parent.Estimated, child.Estimated)
	}
	limit := parent.Estimated - 1
	_, err = e.ExecuteWithContext(a, "Relay", &CallContext{GasLimit: &limit}, b, "Child", "v")
	requireCode(t, err, core.CodeGasLimitExceeded)
	for _, address := range []string{a, b} {
		if data, err := e.Execute(address, "Read"); err != nil || string(data) != `["",false]` {
			t.Fatalf("gas failure committed %s %v", data, err)
		}
	}
}
func TestIgnoredLocalCallEncodingFailureDiscardsStore(t *testing.T) {
	e := NewVMEngine(VMConfig{ContractStorageDir: t.TempDir()})
	source := `package main;import "github.com/lengzhao/vm/contractapi"
 func F()int{contractapi.DefaultObject().Set("x",[]byte("bad"));contractapi.Call("child","F",complex(1,2));return 7}
 func Read()(string,bool,error){v,ok,err:=contractapi.DefaultObject().Get("x");return string(v),ok,err}`
	c, err := e.Compile(source)
	if err != nil {
		t.Fatal(err)
	}
	a, err := e.Deploy(c)
	if err != nil {
		t.Fatal(err)
	}
	_, err = e.Execute(a, "F")
	requireCode(t, err, core.CodeCallFailure)
	if data, err := e.Execute(a, "Read"); err != nil || string(data) != `["",false]` {
		t.Fatalf("ignored local failure committed %s %v", data, err)
	}
}
