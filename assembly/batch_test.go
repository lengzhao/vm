package assembly_test

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/lengzhao/pluginkit"
	"github.com/lengzhao/pluginkit/build"
	"github.com/lengzhao/vm/assembly"
	"github.com/lengzhao/vm/core"
	"github.com/lengzhao/vm/internal/state"
)

func parallelConfig(t *testing.T, workers int) assembly.Config {
	cfg := assembly.DefaultConfig(core.VMConfig{ContractStorageDir: t.TempDir(), EnableSecurityChecks: true, RequireDeterminism: true, EnableGasMetering: true, MaxGasLimit: 1000000})
	cfg.Plugins["state"] = map[string]any{"use": "state/mvcc"}
	cfg.Plugins["batch"] = map[string]any{"use": "batch/ordered", "config": map[string]any{"workers": workers}, "deps": map[string]any{"state": "state", "executor": "engine"}}
	cfg.Plugins["app"] = map[string]any{"use": "engine/batched", "deps": map[string]any{"engine": "engine", "batch": "batch"}}
	cfg.Root = "app"
	return cfg
}

const batchContract = `package contract
import ("github.com/lengzhao/vm/contractapi";"strconv";"errors")
func Inc() (int64,error) {o:=contractapi.DefaultObject();v,_,err:=o.Get("n");if err!=nil{return 0,err};n,_:=strconv.ParseInt(string(v),10,64);n++;if err=o.Set("n",[]byte(strconv.FormatInt(n,10)));err!=nil{return 0,err};contractapi.Log("inc","n",n);return n,nil}
func Read() string {v,_,_:=contractapi.DefaultObject().Get("n");return string(v)}
func Need() (int64,error) {v,ok,_:=contractapi.DefaultObject().Get("n");if !ok{return 0,errors.New("missing")};n,_:=strconv.ParseInt(string(v),10,64);return n,nil}
func Fail() error {contractapi.DefaultObject().Set("n",[]byte("bad"));contractapi.Log("bad");return errors.New("business")}
func OOG() int {contractapi.DefaultObject().Set("n",[]byte("oog"));for i:=0;i<100;i++{};return 1}
func Child(a string)error {_,err:=contractapi.Call(a,"Inc");return err}
func ChildFail(a string)error {contractapi.Call(a,"Inc");return errors.New("parent failed")}
func Clear()error{return contractapi.DefaultObject().Delete("n")}`

type normalizedItem struct {
	Result *core.ExecuteResult
	Code   core.ErrorCode
}

func normalize(t *testing.T, items []core.BatchItemResult) []normalizedItem {
	t.Helper()
	out := make([]normalizedItem, len(items))
	for i, item := range items {
		out[i].Result = item.Result
		if item.Err != nil {
			var ee *core.ExecutionError
			if !errors.As(item.Err, &ee) {
				t.Fatal(item.Err)
			}
			out[i].Code = ee.Code
		}
	}
	return out
}
func TestParallelMatchesRealSerialExecution(t *testing.T) {
	var baseline []normalizedItem
	var expectedState []string
	for _, workers := range []int{0, 1, 2, 4} {
		t.Run(strconv.Itoa(workers), func(t *testing.T) {
			ctx := context.Background()
			rt, err := assembly.Build(ctx, parallelConfig(t, max(workers, 1)))
			if err != nil {
				t.Fatal(err)
			}
			defer rt.Stop(ctx)
			c, err := rt.Engine.Compile(ctx, batchContract)
			if err != nil {
				t.Fatal(err)
			}
			deploy := func(n uint64) string {
				a, e := rt.Engine.Deploy(ctx, core.DeploymentRequest{Contract: c, Input: core.DeploymentInput{ChainID: "batch", Sender: "test", Nonce: n}})
				if e != nil {
					t.Fatal(e)
				}
				return a
			}
			a, b := deploy(0), deploy(1)
			requests := []core.ExecutionRequest{{Address: a, Function: "Inc"}, {Address: a, Function: "Need"}, {Address: a, Function: "Fail"}, {Address: a, Function: "Read"}, {Address: a, Function: "Child", Args: []any{b}}, {Address: a, Function: "ChildFail", Args: []any{b}}, {Address: b, Function: "Read"}, {Address: a, Function: "OOG", CallContext: &core.CallContext{GasLimit: core.Uint64Ptr(40)}}, {Address: a, Function: "Clear"}, {Address: a, Function: "Need"}, {Address: b, Function: "Inc"}, {Address: a, Function: "Read"}}
			requests = append(requests, core.ExecutionRequest{}, core.ExecutionRequest{Address: a}, core.ExecutionRequest{Address: b, Function: "Read"})
			var items []core.BatchItemResult
			if workers == 0 {
				for _, req := range requests {
					r, e := rt.Engine.Execute(ctx, req)
					items = append(items, core.BatchItemResult{Result: r, Err: e})
				}
			} else {
				batch, ok := rt.Engine.(core.BatchExecutor)
				if !ok {
					t.Fatal("batch capability missing")
				}
				items, err = batch.ExecuteBatch(ctx, requests)
				if err != nil {
					t.Fatal(err)
				}
			}
			got := normalize(t, items)
			var final []string
			for _, addr := range []string{a, b} {
				r, e := rt.Engine.Execute(ctx, core.ExecutionRequest{Address: addr, Function: "Read"})
				if e != nil {
					t.Fatal(e)
				}
				final = append(final, string(r.Data))
			}
			if workers == 0 {
				baseline = got
				expectedState = final
				if got[2].Code != core.CodeContractError || got[7].Code != core.CodeGasLimitExceeded {
					t.Fatal(got)
				}
			} else if !reflect.DeepEqual(baseline, got) || !reflect.DeepEqual(expectedState, final) {
				t.Fatalf("workers=%d\nserial=%+v\nparallel=%+v\nstate=%v want=%v", workers, baseline, got, final, expectedState)
			}
		})
	}
}
func TestBatchGraphRequiresOnlyNarrowCapabilities(t *testing.T) {
	cfg := parallelConfig(t, 2)
	ctx := context.Background()
	batch, result, err := build.Build[core.BatchExecutor](ctx, cfg.Plugins, "batch")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := build.GetByID[core.TransactionExecutor](result, "engine"); !ok {
		t.Fatal("missing transaction executor")
	}
	if _, ok := build.GetByID[core.SnapshotStore](result, "state"); !ok {
		t.Fatal("missing snapshot store")
	}
	if _, err := batch.ExecuteBatch(ctx, nil); err != nil {
		t.Fatal(err)
	}
	cfg.Plugins["state"] = map[string]any{"use": "state/memory"}
	if _, _, err := build.Build[core.BatchExecutor](ctx, cfg.Plugins, "batch"); err == nil {
		t.Fatal("non-validating backend accepted")
	}
}
func TestParallelPreset(t *testing.T) {
	data, err := os.ReadFile("../presets/parallel.yaml")
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.ReplaceAll(string(data), "./contracts", t.TempDir()))
	rt, err := assembly.Load(context.Background(), strings.NewReader(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Stop(context.Background())
	if _, ok := rt.Engine.(core.BatchExecutor); !ok {
		t.Fatal("preset lacks batch executor")
	}
}

// These test plugins deliberately omit StateStore.Begin and all Engine methods.
type snapshotOnly struct{ core.SnapshotStore }
type transactionOnly struct{}

func (*transactionOnly) ExecuteTransaction(context.Context, core.ExecutionRequest, core.StateTransaction) (*core.ExecuteResult, error) {
	return &core.ExecuteResult{Data: []byte(`1`)}, nil
}
func init() {
	pluginkit.Register("test/snapshot-only", func(struct{}) (core.SnapshotStore, error) { return &snapshotOnly{SnapshotStore: state.NewMVCC()}, nil })
	pluginkit.Register("test/transaction-only", func(struct{}) (core.TransactionExecutor, error) { return &transactionOnly{}, nil })
}
func TestBatchAcceptsMinimalCustomPlugins(t *testing.T) {
	graph := map[string]any{
		"state":    map[string]any{"use": "test/snapshot-only"},
		"executor": map[string]any{"use": "test/transaction-only"},
		"batch":    map[string]any{"use": "batch/ordered", "deps": map[string]any{"state": "state", "executor": "executor"}},
	}
	b, _, err := build.Build[core.BatchExecutor](context.Background(), graph, "batch")
	if err != nil {
		t.Fatal(err)
	}
	result, err := b.ExecuteBatch(context.Background(), []core.ExecutionRequest{{}})
	if err != nil || len(result) != 1 || string(result[0].Result.Data) != "1" {
		t.Fatal(result, err)
	}
}
