package assembly_test

import (
	"context"
	"errors"
	"github.com/lengzhao/pluginkit"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/lengzhao/vm/assembly"
	"github.com/lengzhao/vm/core"
)

const ownedContract = `package main
import ("errors";"github.com/lengzhao/vm/contractapi")
func Create(id string) error { _,e:=contractapi.CreateObject(id);return e }
func Put(id,value string) error {return contractapi.OpenObject(id).Set("k",[]byte(value))}
func Read(id string)(string,error) {v,_,e:=contractapi.OpenObject(id).Get("k");return string(v),e}
func Transfer(id,to string) error{return contractapi.OpenObject(id).Transfer(to)}
func Destroy(id string) error{return contractapi.OpenObject(id).Destroy()}
func Fail(id string) error{if e:=contractapi.OpenObject(id).Set("k",[]byte("bad"));e!=nil{return e};return errors.New("rollback")}
func Ignore(id string) string{contractapi.OpenObject(id).Set("k",[]byte("bad"));return "ignored"}
func ChildFail(target,id string) error{_,e:=contractapi.Call(target,"Put",id,"bad-child");if e!=nil{return e};return errors.New("parent rollback")}
func OOG(id string) int {contractapi.OpenObject(id).Set("k",[]byte("oog"));for i:=0;i<100;i++{};return 1}
func Child(target,id string) error{_,e:=contractapi.Call(target,"Put",id,"child");return e}
`

func objectsConfig(t *testing.T) assembly.Config {
	cfg := recoveryConfig(t)
	cfg.Plugins["auth"] = map[string]any{"use": "objects/owner"}
	cfg.Plugins["objects"] = map[string]any{"use": "objects/transactional", "deps": map[string]any{"authorizer": "auth"}}
	engine := cfg.Plugins["engine"].(map[string]any)
	cfg.Plugins["components"] = map[string]any{"use": "engine/components", "deps": engine["deps"]}
	engine["use"] = "engine/objects"
	engine["deps"] = map[string]any{"components": "components", "objects": "objects"}
	cfg.Plugins["batch"].(map[string]any)["config"].(map[string]any)["executionID"] = "owned-v1"
	return cfg
}
func TestOwnedObjectsRealExecutionRecoveryAndAuthorization(t *testing.T) {
	ctx := context.Background()
	cfg := objectsConfig(t)
	cfg.Plugins["runner"] = map[string]any{"use": "test/recovery-runner", "config": map[string]any{"fail": false}}
	rt, e := assembly.Build(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer func() { rt.Stop(ctx) }()
	c, e := rt.Engine.Compile(ctx, ownedContract)
	if e != nil {
		t.Fatal(e)
	}
	deploy := func(n uint64) string {
		a, e := rt.Engine.Deploy(ctx, core.DeploymentRequest{Contract: c, Input: core.DeploymentInput{ChainID: "owned", Sender: "test", Nonce: n}})
		if e != nil {
			t.Fatal(e)
		}
		return a
	}
	a, b := deploy(0), deploy(1)
	req := func(addr, sender, fn string, args ...any) core.ExecutionRequest {
		s := core.Address(sender)
		return core.ExecutionRequest{Address: addr, Function: fn, Args: args, CallContext: &core.CallContext{Sender: &s}}
	}
	run := func(r core.ExecutionRequest, fail bool) *core.ExecuteResult {
		t.Helper()
		v, e := rt.Engine.Execute(ctx, r)
		if (e != nil) != fail {
			t.Fatalf("%s: %v", r.Function, e)
		}
		return v
	}
	run(req(a, "alice", "Create", "x"), false)
	run(req(a, "alice", "Put", "x", "good"), false)
	run(req(a, "bob", "Ignore", "x"), true)
	run(req(a, "alice", "Fail", "x"), true)
	if v := run(req(a, "bob", "Read", "x"), false); string(v.Data) != `"good"` {
		t.Fatal(string(v.Data))
	}
	run(req(b, "alice", "Read", "x"), true)
	run(req(b, "alice", "Create", "x"), false)
	run(req(a, "alice", "Child", b, "x"), true) // child Sender is a, not alice
	run(req(b, "alice", "Transfer", "x", a), false)
	run(req(a, "alice", "Child", b, "x"), false)
	run(req(a, "alice", "ChildFail", b, "x"), true)
	if v := run(req(b, "alice", "Read", "x"), false); string(v.Data) != `"child"` {
		t.Fatal("child rollback", string(v.Data))
	}
	low := req(a, "alice", "OOG", "x")
	low.CallContext.GasLimit = core.Uint64Ptr(40)
	run(low, true)
	batchReq := core.RecoverableBatchRequest{ID: "owned-batch", Calls: []core.ExecutionRequest{req(a, "alice", "Transfer", "x", "bob"), req(a, "alice", "Put", "x", "denied"), req(a, "bob", "Put", "x", "final"), req(a, "alice", "Read", "x")}}
	// Inject an infrastructure interruption after the ownership changes commit.
	for _, inst := range rt.Result.Instances {
		if inst.ID == "runner" {
			inst.Value.(*recoveryFaultRunner).fail.Store(true)
		}
	}
	batch := rt.Engine.(core.RecoverableBatchExecutor)
	items, e := batch.ExecuteRecoverableBatch(ctx, batchReq)
	if e == nil || len(items) != 3 {
		t.Fatal(items, e)
	}
	if items[1].Err == nil {
		t.Fatal("old owner wrote")
	}
	if e = rt.Stop(ctx); e != nil {
		t.Fatal(e)
	}
	rt, e = assembly.Build(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	again, e := rt.Engine.(core.RecoverableBatchExecutor).ExecuteRecoverableBatch(ctx, batchReq)
	if e != nil || len(again) != 4 || !reflect.DeepEqual(normalize(t, items), normalize(t, again[:3])) {
		t.Fatal("replay differs", again, e)
	}
	if v := run(req(a, "alice", "Read", "x"), false); string(v.Data) != `"final"` {
		t.Fatal(string(v.Data))
	}
	replay, e := rt.Engine.(core.RecoverableBatchExecutor).ExecuteRecoverableBatch(ctx, batchReq)
	if e != nil || !reflect.DeepEqual(normalize(t, again), normalize(t, replay)) {
		t.Fatal("completed replay", e)
	}
	// EstimateGas DryRun uses the same scoped service, with caller-supplied context.
	_, e = rt.Engine.EstimateGas(ctx, a, "Put", &core.EstimateOptions{DryRun: true, Args: []any{"x", "dry"}, CallCtx: req(a, "bob", "Put").CallContext})
	if e != nil {
		t.Fatal(e)
	}
	if v := run(req(a, "bob", "Read", "x"), false); string(v.Data) != `"final"` {
		t.Fatal("DryRun committed", string(v.Data))
	}
	run(req(a, "bob", "Destroy", "x"), false)
	run(req(a, "bob", "Create", "x"), true)
}

// The object factory consumes only the core capability; an extension may wrap it
// without exposing implementation details or lifecycle methods.
type minimalObjectService struct{ core.ObjectService }

func init() {
	pluginkit.Register("test/minimal-objects", func(_ struct{}, d struct {
		Service core.ObjectService `json:"service"`
	}) (core.ObjectService, error) { return minimalObjectService{d.Service}, nil })
}
func TestOwnedPresetMinimalCapabilitiesAndDisabledDefault(t *testing.T) {
	ctx := context.Background()
	cfg := objectsConfig(t)
	cfg.Plugins["minimal"] = map[string]any{"use": "test/minimal-objects", "deps": map[string]any{"service": "objects"}}
	cfg.Plugins["engine"].(map[string]any)["deps"].(map[string]any)["objects"] = "minimal"
	rt, e := assembly.Build(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	rt.Stop(ctx)
	raw, e := os.ReadFile("../presets/objects.yaml")
	if e != nil {
		t.Fatal(e)
	}
	dir := t.TempDir()
	config := strings.ReplaceAll(string(raw), "./contracts", filepath.Join(dir, "contracts"))
	config = strings.ReplaceAll(config, "./state.db", filepath.Join(dir, "state.db"))
	rt, e = assembly.Load(ctx, strings.NewReader(config))
	if e != nil {
		t.Fatal(e)
	}
	rt.Stop(ctx)
	cfg = parallelConfig(t, 1)
	cfg.Root = "engine"
	rt, e = assembly.Build(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer rt.Stop(ctx)
	c, e := rt.Engine.Compile(ctx, ownedContract)
	if e != nil {
		t.Fatal(e)
	}
	a, e := rt.Engine.Deploy(ctx, core.DeploymentRequest{Contract: c, Input: core.DeploymentInput{ChainID: "disabled", Sender: "test"}})
	if e != nil {
		t.Fatal(e)
	}
	sender := core.Address("alice")
	_, e = rt.Engine.Execute(ctx, core.ExecutionRequest{Address: a, Function: "Create", Args: []any{"x"}, CallContext: &core.CallContext{Sender: &sender}})
	var ee *core.ExecutionError
	if !errors.As(e, &ee) || ee.Code != core.CodeStateFailure {
		t.Fatal("disabled service accepted", e)
	}
}
