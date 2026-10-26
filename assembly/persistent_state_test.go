package assembly_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lengzhao/vm/assembly"
	"github.com/lengzhao/vm/core"
)

func TestPersistentStateRealTransactionsBatchAndReopen(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	cfg := parallelConfig(t, 3)
	cfg.Plugins["state"] = map[string]any{"use": "state/bbolt", "config": map[string]any{"path": filepath.Join(dir, "state.db")}}
	rt, err := assembly.Build(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { rt.Stop(ctx) }()
	c, err := rt.Engine.Compile(ctx, batchContract)
	if err != nil {
		t.Fatal(err)
	}
	a, err := rt.Engine.Deploy(ctx, core.DeploymentRequest{Contract: c, Input: core.DeploymentInput{ChainID: "persist", Sender: "test"}})
	if err != nil {
		t.Fatal(err)
	}
	b, err := rt.Engine.Deploy(ctx, core.DeploymentRequest{Contract: c, Input: core.DeploymentInput{ChainID: "persist", Sender: "test", Nonce: 1}})
	if err != nil {
		t.Fatal(err)
	}
	batch := rt.Engine.(core.BatchExecutor)
	items, err := batch.ExecuteBatch(ctx, []core.ExecutionRequest{{Address: a, Function: "Inc"}, {Address: a, Function: "Inc"}, {Address: a, Function: "Child", Args: []any{b}}, {Address: a, Function: "ChildFail", Args: []any{b}}, {Address: a, Function: "OOG", CallContext: &core.CallContext{GasLimit: core.Uint64Ptr(40)}}, {Address: a, Function: "Fail"}})
	if err != nil || len(items) != 6 {
		t.Fatal(items, err)
	}
	for i := 0; i < 3; i++ {
		if items[i].Err != nil {
			t.Fatal(items[i].Err)
		}
	}
	for i := 3; i < 6; i++ {
		if items[i].Err == nil || items[i].Result != nil && (items[i].Result.Data != nil || items[i].Result.Events != nil) {
			t.Fatal(items[i])
		}
	}
	if _, err := rt.Engine.EstimateGas(ctx, a, "Inc", &core.EstimateOptions{DryRun: true}); err != nil {
		t.Fatal(err)
	}
	if err := rt.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	rt, err = assembly.Build(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	for addr, want := range map[string]string{a: `"2"`, b: `"1"`} {
		r, e := rt.Engine.Execute(ctx, core.ExecutionRequest{Address: addr, Function: "Read"})
		if e != nil || string(r.Data) != want {
			t.Fatal(r, e)
		}
	}
}
func TestPersistentStateStartupFailureDoesNotLeakLock(t *testing.T) {
	ctx := context.Background()
	cfg := parallelConfig(t, 2)
	path := filepath.Join(t.TempDir(), "state.db")
	cfg.Plugins["state"] = map[string]any{"use": "state/bbolt", "config": map[string]any{"path": path}}
	first, err := assembly.Build(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Stop(ctx)
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := assembly.Build(cancelCtx, cfg); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	cfg.Plugins["state"].(map[string]any)["config"].(map[string]any)["lockTimeout"] = "100ms"
	if second, err := assembly.Build(ctx, cfg); err == nil {
		second.Stop(ctx)
		t.Fatal("second store opened same file")
	}
	if err := first.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	again, err := assembly.Build(ctx, cfg)
	if err != nil {
		t.Fatal("lock leaked", err)
	}
	again.Stop(ctx)
}

func TestPersistentPresetLifecycle(t *testing.T) {
	raw, e := os.ReadFile("../presets/persistent.yaml")
	if e != nil {
		t.Fatal(e)
	}
	dir := t.TempDir()
	text := strings.ReplaceAll(string(raw), "./contracts", filepath.Join(dir, "contracts"))
	text = strings.ReplaceAll(text, "./state.db", filepath.Join(dir, "state.db"))
	rt, e := assembly.Load(context.Background(), strings.NewReader(text))
	if e != nil {
		t.Fatal(e)
	}
	if _, ok := rt.Engine.(core.BatchExecutor); !ok {
		t.Fatal("batch capability missing")
	}
	if e = rt.Stop(context.Background()); e != nil {
		t.Fatal(e)
	}
	rt, e = assembly.Load(context.Background(), strings.NewReader(text))
	if e != nil {
		t.Fatal("database lock leaked", e)
	}
	rt.Stop(context.Background())
}
