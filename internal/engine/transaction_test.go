package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/lengzhao/vm/core"
	"github.com/lengzhao/vm/internal/state"
)

func TestExecuteTransactionLeavesOwnershipWithCaller(t *testing.T) {
	ctx := context.Background()
	store := state.NewMVCC()
	tx, _ := store.BeginSnapshot(ctx)
	defer tx.Rollback()
	d := &depsStub{run: func(ctx context.Context) (*core.CallResult, error) {
		o, ok := core.ObjectStoreFromContext(ctx)
		if !ok {
			t.Fatal("no object")
		}
		if e := o.Set(ctx, "x", []byte("value")); e != nil {
			t.Fatal(e)
		}
		return &core.CallResult{Data: []byte(`1`), GasConsumed: 12, Events: []core.Event{{Name: "event"}}}, nil
	}}
	e := testEngine(t, d)
	r, err := e.ExecuteTransaction(ctx, core.ExecutionRequest{Address: "a", Function: "F"}, tx)
	if err != nil || string(r.Data) != "1" {
		t.Fatal(r, err)
	}
	outside, _ := store.BeginSnapshot(ctx)
	defer outside.Rollback()
	if _, ok, _ := outside.Get(ctx, core.StateKey{Contract: "a", Key: "x"}); ok {
		t.Fatal("committed before caller")
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	check, _ := store.Begin(ctx)
	defer check.Rollback()
	v, _, _ := check.Get(ctx, core.StateKey{Contract: "a", Key: "x"})
	if string(v) != "value" {
		t.Fatal(string(v))
	}
}
func TestExecuteTransactionFailureAndCancellation(t *testing.T) {
	ctx := context.Background()
	tx, _ := state.NewMVCC().BeginSnapshot(ctx)
	defer tx.Rollback()
	boom := errors.New("business")
	d := &depsStub{run: func(ctx context.Context) (*core.CallResult, error) {
		o, _ := core.ObjectStoreFromContext(ctx)
		o.Set(ctx, "x", []byte("bad"))
		return &core.CallResult{Data: []byte(`1`), Events: []core.Event{{Name: "bad"}}, GasConsumed: 12}, boom
	}}
	e := testEngine(t, d)
	r, err := e.ExecuteTransaction(ctx, core.ExecutionRequest{Address: "a", Function: "F"}, tx)
	if !errors.Is(err, boom) || r == nil || r.Data != nil || r.Events != nil || r.GasConsumed != 12 {
		t.Fatal(r, err)
	}
	if err := tx.Validate(ctx); err != nil {
		t.Fatal("callee closed transaction", err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := e.ExecuteTransaction(canceled, core.ExecutionRequest{Address: "a", Function: "F"}, tx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := e.ExecuteTransaction(ctx, core.ExecutionRequest{Address: "a", Function: "F"}, nil); err == nil {
		t.Fatal("nil transaction")
	}
}

type failedReader struct{ err error }

func (r failedReader) Get(context.Context, string) (*core.CompiledContract, error) { return nil, r.err }
func TestTransactionExecutionMatchesLoadError(t *testing.T) {
	e := testEngine(t, &depsStub{})
	e.deps.Contracts = failedReader{err: errors.New("storage unavailable")}
	ctx := context.Background()
	tx, _ := state.NewMVCC().Begin(ctx)
	defer tx.Rollback()
	_, serial := e.Execute(ctx, core.ExecutionRequest{Address: "a", Function: "F"})
	_, scheduled := e.ExecuteTransaction(ctx, core.ExecutionRequest{Address: "a", Function: "F"}, tx)
	if serial == nil || scheduled == nil || serial.Error() != scheduled.Error() {
		t.Fatal(serial, scheduled)
	}
}

func TestExecutionRequestShapeHasStableCode(t *testing.T) {
	ctx := context.Background()
	e := testEngine(t, &depsStub{})
	tx, _ := state.NewMVCC().Begin(ctx)
	defer tx.Rollback()
	for _, req := range []core.ExecutionRequest{{Address: "a"}, {Function: "F"}} {
		_, serial := e.Execute(ctx, req)
		_, scheduled := e.ExecuteTransaction(ctx, req, tx)
		for _, err := range []error{serial, scheduled} {
			var ee *core.ExecutionError
			if !errors.As(err, &ee) || ee.Code != core.CodeInvalidRequest {
				t.Fatalf("unclassified request: %v", err)
			}
		}
	}
}
