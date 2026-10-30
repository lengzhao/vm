package vm

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/lengzhao/vm/core"
)

const objectSource = `package main
import (
 "errors"
 "github.com/lengzhao/vm/contractapi"
)
func Put(v string) error {return contractapi.DefaultObject().Set("value",[]byte(v))}
func Read() (string,bool,error) {v,ok,err:=contractapi.DefaultObject().Get("value");return string(v),ok,err}
func Remove() error {return contractapi.DefaultObject().Delete("value")}
func Change(v string) (string,error) {o:=contractapi.DefaultObject();if err:=o.Set("value",[]byte(v));err!=nil{return "",err};b,_,err:=o.Get("value");contractapi.Log("Changed","value",string(b));return string(b),err}
func Fail() error {if err:=Put("failed");err!=nil{return err};contractapi.Log("Failed");return errors.New("business failure")}
func Panic() {Put("panic");contractapi.Log("Panicked");panic("failed")}
func Burn() {Put("oog");contractapi.Log("Burned");for i:=0;i<10000;i++{}}
func Spin() {Put("timeout");contractapi.Log("Spin");for {}}
`

func TestDefaultObjectEndToEndAndDryRun(t *testing.T) {
	e := NewVMEngine(VMConfig{ContractStorageDir: t.TempDir(), EnableSecurityChecks: true, EnableGasMetering: true, MaxGasLimit: 1000})
	c, err := e.Compile(objectSource)
	if err != nil {
		t.Fatal(err)
	}
	address, err := e.Deploy(c)
	if err != nil {
		t.Fatal(err)
	}
	read := func(want string) {
		t.Helper()
		data, err := e.Execute(address, "Read")
		if err != nil || string(data) != want {
			t.Fatalf("Read=%s %v want %s", data, err, want)
		}
	}
	read(`["",false]`)
	result, err := e.ExecuteWithContext(address, "Change", &CallContext{ContractAddress: AddressPtr("spoofed")}, "stored")
	if err != nil || len(result.Events) != 1 || string(result.Data) != `"stored"` {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	read(`["stored",true]`)
	if _, err := e.EstimateGas(address, "Change", &EstimateOptions{DryRun: true, Args: []any{"dry"}}); err != nil {
		t.Fatal(err)
	}
	read(`["stored",true]`)
	if _, err := e.Engine.EstimateGas(context.Background(), address, "Change", &core.EstimateOptions{DryRun: true, Args: []any{"native-dry"}}); err != nil {
		t.Fatal(err)
	}
	read(`["stored",true]`)
	for _, fn := range []string{"Fail", "Panic", "Burn"} {
		out, err := e.Engine.Execute(context.Background(), core.ExecutionRequest{Address: address, Function: fn})
		if err == nil {
			t.Fatalf("%s succeeded", fn)
		}
		if out != nil && (len(out.Events) != 0 || len(out.Data) != 0) {
			t.Fatalf("%s published %+v", fn, out)
		}
		read(`["stored",true]`)
	}
	if _, err := e.Execute(address, "Put", ""); err != nil {
		t.Fatal(err)
	}
	read(`["",true]`)
	if _, err := e.Execute(address, "Remove"); err != nil {
		t.Fatal(err)
	}
	read(`["",false]`)
}
func TestObjectIsolationAndCancellation(t *testing.T) {
	e := NewVMEngine(VMConfig{ContractStorageDir: t.TempDir(), EnableSecurityChecks: true, ExecutionTimeout: 5 * time.Second})
	c, err := e.Compile(objectSource)
	if err != nil {
		t.Fatal(err)
	}
	a, err := e.DeployWithInput(c, core.DeploymentInput{ChainID: "test", Sender: "alice", Nonce: 1})
	if err != nil {
		t.Fatal(err)
	}
	b, err := e.DeployWithInput(c, core.DeploymentInput{ChainID: "test", Sender: "alice", Nonce: 2})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Execute(a, "Put", "first"); err != nil {
		t.Fatal(err)
	}
	if data, err := e.Execute(b, "Read"); err != nil || string(data) != `["",false]` {
		t.Fatalf("cross-contract state: %s %v", data, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := e.Engine.Execute(ctx, core.ExecutionRequest{Address: a, Function: "Spin"}); err == nil {
		t.Fatal("uncanceled spin")
	}
	if data, err := e.Execute(a, "Read"); err != nil || string(data) != `["first",true]` {
		t.Fatalf("canceled call committed: %s %v", data, err)
	}
}

func TestObjectDeterministicReplay(t *testing.T) {
	const source = `package main
 import "github.com/lengzhao/vm/contractapi"
 func Change(v string)(string,error){o:=contractapi.DefaultObject();if err:=o.Set("x",[]byte(v));err!=nil{return "",err};b,_,err:=o.Get("x");contractapi.Log("Changed","value",string(b));return string(b),err}`
	var reference *ExecuteResult
	for i := 0; i < 2; i++ {
		e := NewVMEngine(VMConfig{ContractStorageDir: t.TempDir(), RequireDeterminism: true, EnableGasMetering: true, MaxGasLimit: 1000})
		c, err := e.Compile(source)
		if err != nil {
			t.Fatal(err)
		}
		a, err := e.Deploy(c)
		if err != nil {
			t.Fatal(err)
		}
		result, err := e.ExecuteWithContext(a, "Change", nil, "same")
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			reference = result
		} else if string(result.Data) != string(reference.Data) || result.GasConsumed != reference.GasConsumed || !reflect.DeepEqual(result.Events, reference.Events) {
			t.Fatalf("replay differed: %+v %+v", reference, result)
		}
	}
}

func TestObjectRuntimeControlsRemainPrivate(t *testing.T) {
	for _, secure := range []bool{false, true} {
		e := NewVMEngine(VMConfig{ContractStorageDir: t.TempDir(), EnableSecurityChecks: secure})
		for _, body := range []string{`contractapi.InitObjectIO(nil,nil)`, `_ = contractapi.ObjectRequest{}`, `_ = contractapi.ObjectResponse{}`, `_ = contractapi.MaxObjectCalls`, `_ = contractapi.ObjectSource`, `_ = contractapi.Object{}`} {
			if _, err := e.Compile(`package main;import "github.com/lengzhao/vm/contractapi";func F(){` + body + `}`); err == nil {
				t.Fatalf("runtime symbol accepted: %s", body)
			}
		}
	}
}
