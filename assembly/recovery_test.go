package assembly_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lengzhao/pluginkit"
	"github.com/lengzhao/vm/assembly"
	"github.com/lengzhao/vm/core"
	"github.com/lengzhao/vm/internal/process"
)

type recoveryFaultRunner struct {
	core.Runner
	fail atomic.Bool
}

func (r *recoveryFaultRunner) Run(ctx context.Context, c *core.CompiledContract, req core.CallRequest) (*core.CallResult, error) {
	if req.Function == "Read" && r.fail.CompareAndSwap(true, false) {
		return nil, &core.ExecutionError{Code: core.CodeProcessFailure, Message: "transient runner failure"}
	}
	return r.Runner.Run(ctx, c, req)
}
func init() {
	pluginkit.Register("test/recovery-runner", func(c struct {
		Fail bool `json:"fail"`
	}) (core.Runner, error) {
		r := &recoveryFaultRunner{Runner: process.New(30 * time.Second)}
		r.fail.Store(c.Fail)
		return r, nil
	})
}

type receiptOnlyState struct{ core.RecoverableBatchStore }

func init() {
	pluginkit.Register("test/receipt-only-state", func(_ struct{}, d struct {
		Store core.RecoverableBatchStore `json:"store"`
	}) (core.RecoverableBatchStore, error) { return &receiptOnlyState{RecoverableBatchStore: d.Store}, nil })
}
func recoveryConfig(t *testing.T) assembly.Config {
	cfg := parallelConfig(t, 2)
	cfg.Plugins["state"] = map[string]any{"use": "state/bbolt", "config": map[string]any{"path": filepath.Join(t.TempDir(), "state.db")}}
	cfg.Plugins["lease"] = map[string]any{"use": "test/receipt-only-state", "deps": map[string]any{"store": "state"}}
	cfg.Plugins["batch"] = map[string]any{"use": "batch/recoverable", "config": map[string]any{"workers": 2, "executionID": "strict-process-gas1m-v1"}, "deps": map[string]any{"state": "lease", "executor": "engine", "contracts": "engine"}}
	cfg.Plugins["app"] = map[string]any{"use": "engine/recoverable", "deps": map[string]any{"engine": "engine", "batch": "batch"}}
	return cfg
}
func TestRecoverableRealBatchResumesAndReplays(t *testing.T) {
	ctx := context.Background()
	cfg := recoveryConfig(t)
	cfg.Plugins["runner"] = map[string]any{"use": "test/recovery-runner", "config": map[string]any{"fail": true}}
	rt, e := assembly.Build(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer func() { rt.Stop(ctx) }()
	c, e := rt.Engine.Compile(ctx, batchContract)
	if e != nil {
		t.Fatal(e)
	}
	deploy := func(n uint64) string {
		a, e := rt.Engine.Deploy(ctx, core.DeploymentRequest{Contract: c, Input: core.DeploymentInput{ChainID: "recover", Sender: "test", Nonce: n}})
		if e != nil {
			t.Fatal(e)
		}
		return a
	}
	a, b := deploy(0), deploy(1)
	request := core.RecoverableBatchRequest{ID: "block-1", Calls: []core.ExecutionRequest{{Address: a, Function: "Inc"}, {Address: a, Function: "Read"}, {Address: a, Function: "Inc"}, {Address: a, Function: "Fail"}, {Address: a, Function: "OOG", CallContext: &core.CallContext{GasLimit: core.Uint64Ptr(40)}}, {Address: a, Function: "Child", Args: []any{b}}, {Address: a, Function: "ChildFail", Args: []any{b}}, {Address: b, Function: "Read"}}}
	batch := rt.Engine.(core.RecoverableBatchExecutor)
	prefix, e := batch.ExecuteRecoverableBatch(ctx, request)
	var ee *core.ExecutionError
	if !errors.As(e, &ee) || ee.Code != core.CodeProcessFailure || len(prefix) != 1 {
		t.Fatal(prefix, e)
	}
	limited, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	if _, e := rt.Engine.Execute(limited, core.ExecutionRequest{Address: a, Function: "Read"}); !errors.Is(e, core.ErrBatchPending) {
		t.Fatal("ordinary Execute bypassed durable fence", e)
	}
	if e := rt.Stop(ctx); e != nil {
		t.Fatal(e)
	}
	cfg.Plugins["runner"].(map[string]any)["config"].(map[string]any)["fail"] = false
	rt, e = assembly.Build(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	batch = rt.Engine.(core.RecoverableBatchExecutor)
	got, e := batch.ExecuteRecoverableBatch(ctx, request)
	if e != nil || len(got) != len(request.Calls) {
		t.Fatal(got, e)
	}
	expected := normalize(t, got)
	again, e := batch.ExecuteRecoverableBatch(ctx, request)
	if e != nil || !reflect.DeepEqual(expected, normalize(t, again)) {
		t.Fatal("replay differs", again, e)
	}
	for addr, want := range map[string]string{a: `"2"`, b: `"1"`} {
		r, e := rt.Engine.Execute(ctx, core.ExecutionRequest{Address: addr, Function: "Read"})
		if e != nil || string(r.Data) != want {
			t.Fatal(r, e)
		}
	}
	changed := request
	changed.Calls = append([]core.ExecutionRequest(nil), request.Calls...)
	changed.Calls[0].Function = "Read"
	if _, e := batch.ExecuteRecoverableBatch(ctx, changed); !errors.Is(e, core.ErrBatchIdentityConflict) {
		t.Fatal(e)
	}
}

func TestRecoverablePresetAndMinimalCapabilities(t *testing.T) {
	raw, e := os.ReadFile("../presets/recoverable.yaml")
	if e != nil {
		t.Fatal(e)
	}
	dir := t.TempDir()
	data := strings.ReplaceAll(string(raw), "./contracts", filepath.Join(dir, "contracts"))
	data = strings.ReplaceAll(data, "./state.db", filepath.Join(dir, "state.db"))
	rt, e := assembly.Load(context.Background(), strings.NewReader(data))
	if e != nil {
		t.Fatal(e)
	}
	defer rt.Stop(context.Background())
	recovery, ok := rt.Engine.(core.RecoverableBatchExecutor)
	if !ok {
		t.Fatal("missing recoverable capability")
	}
	items, e := recovery.ExecuteRecoverableBatch(context.Background(), core.RecoverableBatchRequest{ID: "empty"})
	if e != nil || len(items) != 0 {
		t.Fatal(items, e)
	}
	// Recoverable factory rejects a backend lacking the session/receipt contract.
	cfg := recoveryConfig(t)
	cfg.Plugins["state"] = map[string]any{"use": "state/mvcc"}
	if _, e := assembly.Build(context.Background(), cfg); e == nil {
		t.Fatal("non-durable backend accepted")
	}
}
