package assembly_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lengzhao/vm/assembly"
	"github.com/lengzhao/vm/core"
	_ "github.com/lengzhao/vm/plugins/all"
)

func wasiConfig(t testing.TB) assembly.Config {
	t.Helper()
	// These integration tests verify semantics, not latency: race instruments
	// wazero's compiler, and a nested call JITs two complete Go modules.
	// Dedicated runner tests retain short deadlines to verify interruption.
	cfg := assembly.DefaultConfig(core.VMConfig{ContractStorageDir: t.TempDir(), EnableSecurityChecks: false, EnableGasMetering: true, MaxGasLimit: 1000000, ExecutionTimeout: 90 * time.Second})
	cfg.Plugins["compiler"].(map[string]any)["use"] = "compiler/wasip1"
	cfg.Plugins["runner"] = map[string]any{"use": "runner/wazero", "config": map[string]any{"timeout": "60s", "memoryMB": 128, "engine": "compiler"}}
	cfg.Plugins["validator"].(map[string]any)["config"] = map[string]any{"targetOS": "wasip1", "targetArch": "wasm"}
	return cfg
}
func loadWASI(t testing.TB, source string) (*assembly.Runtime, string) {
	t.Helper()
	ctx := context.Background()
	rt, err := assembly.Build(ctx, wasiConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := rt.Stop(context.Background()); err != nil {
			t.Error(err)
		}
	})
	c, err := rt.Engine.Compile(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	a, err := rt.Engine.Deploy(ctx, core.DeploymentRequest{Contract: c, Input: core.DeploymentInput{ChainID: "wasi", Sender: "test"}})
	if err != nil {
		t.Fatal(err)
	}
	return rt, a
}

const wasiContract = `package contract
import("github.com/lengzhao/vm/contractapi";"errors";"strconv")
func Add(a,b int64) int64 {return a+b}
func Width() int {return strconv.IntSize}
func Write(v string) error {contractapi.Log("write","v",v);return contractapi.DefaultObject().Set("value",[]byte(v))}
func Read() string {v,_,_:=contractapi.DefaultObject().Get("value");return string(v)}
func FailedWrite() error {contractapi.DefaultObject().Set("value",[]byte("bad"));return errors.New("failure")}
func OOGWrite() int {contractapi.DefaultObject().Set("value",[]byte("oog"));for i:=0;i<100;i++{};return 1}
func Child(a string) error {_,err:=contractapi.Call(a,"Write","child");return err}
func ChildFailure(a string) error {contractapi.Call(a,"Write","bad-child");return errors.New("failure")}`

const wasiIsolationContract = `package contract
import("os";"net")
func Isolation(path string) bool {_,r:=os.ReadFile(path);w:=os.WriteFile(path,[]byte("bad"),0600);sock,n:=net.Dial("tcp","1.1.1.1:53");if sock!=nil{sock.Close()};return r!=nil && w!=nil && n!=nil && os.Getenv("VM_TEST_SECRET")==""}
`

const wasiResourceContract = `package contract
import "fmt"
func Allocate() int {v:=make([]byte,256<<20);for i:=0;i<len(v);i+=4096 {v[i]=1};return len(v)}
func Flood() int {for i:=0;i<2000;i++ {fmt.Print("xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx")};return 0}`

func TestWASIHostTransactions(t *testing.T) {
	rt, a := loadWASI(t, wasiContract)
	ctx := context.Background()
	call := func(addr, fn string, args ...any) *core.ExecuteResult {
		t.Helper()
		started := time.Now()
		r, err := rt.Engine.Execute(ctx, core.ExecutionRequest{Address: addr, Function: fn, Args: args})
		t.Logf("%s elapsed=%s err=%v", fn, time.Since(started), err)
		if err != nil {
			t.Fatal(fn, err)
		}
		return r
	}
	if string(call(a, "Add", json.Number("9223372036854775806"), 1).Data) != "9223372036854775807" || string(call(a, "Width").Data) != "64" {
		t.Fatal("integer semantics differ")
	}
	if len(call(a, "Write", "good").Events) != 1 {
		t.Fatal("event missing")
	}
	if _, err := rt.Engine.Execute(ctx, core.ExecutionRequest{Address: a, Function: "FailedWrite"}); err == nil {
		t.Fatal("business failure succeeded")
	}
	if string(call(a, "Read").Data) != `"good"` {
		t.Fatal("failure committed")
	}
	if _, err := rt.Engine.EstimateGas(ctx, a, "Write", &core.EstimateOptions{DryRun: true, Args: []any{"dry"}}); err != nil {
		t.Fatal(err)
	}
	if string(call(a, "Read").Data) != `"good"` {
		t.Fatal("DryRun committed")
	}
	_, err := rt.Engine.Execute(ctx, core.ExecutionRequest{Address: a, Function: "OOGWrite", CallContext: &core.CallContext{GasLimit: core.Uint64Ptr(40)}})
	var ee *core.ExecutionError
	if !errors.As(err, &ee) || ee.Code != core.CodeGasLimitExceeded {
		t.Fatalf("OOG: %v", err)
	}
	if string(call(a, "Read").Data) != `"good"` {
		t.Fatal("OOG committed")
	}
	c, err := rt.Engine.Get(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	c2 := *c
	c2.Address = ""
	c2.Deployment = nil
	b, err := rt.Engine.Deploy(ctx, core.DeploymentRequest{Contract: &c2, Input: core.DeploymentInput{ChainID: "wasi", Sender: "test", Nonce: 1}})
	if err != nil {
		t.Fatal(err)
	}
	call(a, "Child", b)
	if string(call(b, "Read").Data) != `"child"` {
		t.Fatal("child write missing")
	}
	if _, err := rt.Engine.Execute(ctx, core.ExecutionRequest{Address: a, Function: "ChildFailure", Args: []any{b}}); err == nil {
		t.Fatal("parent failure succeeded")
	}
	if string(call(b, "Read").Data) != `"child"` {
		t.Fatal("child write escaped rollback")
	}

}
func TestWASIMatchesNativeAndConcurrentCalls(t *testing.T) {
	source := `package main
 import "github.com/lengzhao/vm/contractapi"
 func Value(v uint64)uint64 {contractapi.Log("value","v",v);return v}
 `
	var results []*core.ExecuteResult
	for _, wasm := range []bool{false, true} {
		cfg := wasiConfig(t)
		if !wasm {
			cfg = assembly.DefaultConfig(core.VMConfig{ContractStorageDir: t.TempDir(), EnableSecurityChecks: true, RequireDeterminism: true, EnableGasMetering: true, MaxGasLimit: 1000000})
		} else {
			cfg.Plugins["policy"].(map[string]any)["config"] = map[string]any{"enableSecurity": true, "deterministic": true}
			cfg.Plugins["compiler"].(map[string]any)["config"].(map[string]any)["requireDeterminism"] = true
		}
		rt, err := assembly.Build(context.Background(), cfg)
		if err != nil {
			t.Fatal(err)
		}
		c, err := rt.Engine.Compile(context.Background(), source)
		if err != nil {
			t.Fatal(err)
		}
		a, err := rt.Engine.Deploy(context.Background(), core.DeploymentRequest{Contract: c, Input: core.DeploymentInput{ChainID: "test", Sender: "test"}})
		if err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		errs := make(chan error, 3)
		out := make(chan *core.ExecuteResult, 3)
		for i := 0; i < 3; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				r, e := rt.Engine.Execute(context.Background(), core.ExecutionRequest{Address: a, Function: "Value", Args: []any{json.Number("18446744073709551615")}})
				if e != nil {
					errs <- e
				} else {
					out <- r
				}
			}()
		}
		wg.Wait()
		close(errs)
		close(out)
		for e := range errs {
			t.Fatal(e)
		}
		var baseline *core.ExecuteResult
		for r := range out {
			if baseline == nil {
				baseline = r
			}
			if !reflect.DeepEqual(baseline, r) {
				t.Fatal("concurrent results differ")
			}
		}
		results = append(results, baseline)
		if err := rt.Stop(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(results[0], results[1]) {
		t.Fatalf("native/WASI differ: %+v %+v", results[0], results[1])
	}
}

func TestWASIPresetBuildsWithoutStartingResources(t *testing.T) {
	data, err := os.ReadFile("../presets/wasi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	rt, err := assembly.Load(context.Background(), strings.NewReader(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestWASIInterpreterChoice(t *testing.T) {
	cfg := wasiConfig(t)
	cfg.Plugins["runner"].(map[string]any)["config"].(map[string]any)["engine"] = "interpreter"
	rt, err := assembly.Build(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Stop(context.Background())
	c, err := rt.Engine.Compile(context.Background(), "package main\nfunc Add(a,b int64)int64{return a+b}")
	if err != nil {
		t.Fatal(err)
	}
	a, err := rt.Engine.Deploy(context.Background(), core.DeploymentRequest{Contract: c, Input: core.DeploymentInput{ChainID: "test", Sender: "test"}})
	if err != nil {
		t.Fatal(err)
	}
	r, err := rt.Engine.Execute(context.Background(), core.ExecutionRequest{Address: a, Function: "Add", Args: []any{2, 3}})
	if err != nil || string(r.Data) != "5" {
		t.Fatalf("interpreter: %v %v", r, err)
	}
}

// Isolation imports are intentionally separate: linking net/fmt into every
// transactional frame needlessly multiplies JIT work in nested race tests.
func TestWASIIsolation(t *testing.T) {
	rt, a := loadWASI(t, wasiIsolationContract)
	ctx := context.Background()
	call := func(addr, fn string, args ...any) *core.ExecuteResult {
		t.Helper()
		r, err := rt.Engine.Execute(ctx, core.ExecutionRequest{Address: addr, Function: fn, Args: args})
		if err != nil {
			t.Fatal(fn, err)
		}
		return r
	}
	path := t.TempDir() + "/secret"
	if err := os.WriteFile(path, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VM_TEST_SECRET", "host-secret")
	if string(call(a, "Isolation", path).Data) != "true" {
		t.Fatal("ambient capability leaked")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "secret" {
		t.Fatal("guest wrote host file")
	}
}

func TestWASIResources(t *testing.T) {
	rt, a := loadWASI(t, wasiResourceContract)
	ctx := context.Background()
	for _, fn := range []string{"Allocate", "Flood"} {
		_, err := rt.Engine.Execute(ctx, core.ExecutionRequest{Address: a, Function: fn})
		var ee *core.ExecutionError
		if !errors.As(err, &ee) {
			t.Fatalf("%s resource limit missing: %v", fn, err)
		}
		if fn == "Flood" && ee.Code != core.CodeOutputLimit {
			t.Fatalf("output limit misclassified: %v", err)
		}
		if fn == "Allocate" && (ee.Code != core.CodeProcessFailure || !strings.Contains(strings.ToLower(err.Error()), "memory")) {
			t.Fatalf("memory limit misclassified: %v", err)
		}
	}
}
