package batch

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/lengzhao/vm/core"
	"github.com/lengzhao/vm/internal/artifact/artifacttest"
	"github.com/lengzhao/vm/internal/deployment"
	"github.com/lengzhao/vm/internal/state"
)

type readerFunc func(context.Context, string) (*core.CompiledContract, error)

func (f readerFunc) Get(ctx context.Context, a string) (*core.CompiledContract, error) {
	return f(ctx, a)
}
func TestRecoverableFreezesInputsAndRestoresResults(t *testing.T) {
	ctx := context.Background()
	s, e := state.NewBolt(state.BoltOptions{Path: filepath.Join(t.TempDir(), "state.db")})
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Start(ctx); e != nil {
		t.Fatal(e)
	}
	defer s.Stop(ctx)
	var calls atomic.Int32
	exec := executorFunc(func(ctx context.Context, r core.ExecutionRequest, tx core.StateTransaction) (*core.ExecuteResult, error) {
		calls.Add(1)
		tx.Set(ctx, core.StateKey{Key: r.Function}, []byte("yes"))
		if r.Function == "fail" {
			return nil, &core.ExecutionError{Code: core.CodeContractError, Message: "business"}
		}
		return &core.ExecuteResult{Data: []byte(`18446744073709551615`), GasConsumed: 12, Events: []core.Event{{Name: "n", Fields: map[string]any{"v": uint64(18446744073709551615)}}}}, nil
	})
	// This test reader pins a trusted, immutable deployment identity.
	c := artifacttest.Seal(t, &core.CompiledContract{ProtocolVersion: core.ProtocolVersion, Guarantees: core.PolicyGuarantees{Deterministic: true}})
	input := core.DeploymentInput{ChainID: "test", Sender: "test"}
	c.Address, _ = deployment.DeriveAddress(input, c.ManifestSHA256)
	c.Deployment = &core.DeploymentRecord{Version: core.DeploymentVersion, Input: input, ArtifactSHA256: c.ManifestSHA256}
	reader := readerFunc(func(context.Context, string) (*core.CompiledContract, error) { return c, nil })
	b, e := NewRecoverable(RecoverableOptions{Workers: 2, ExecutionID: "test-v1"}, RecoverableDeps{State: s, Executor: exec, Contracts: reader})
	if e != nil {
		t.Fatal(e)
	}
	request := core.RecoverableBatchRequest{ID: "batch", Calls: []core.ExecutionRequest{{Address: c.Address, Function: "ok"}, {Address: c.Address, Function: "fail"}}}
	got, e := b.ExecuteRecoverableBatch(ctx, request)
	if e != nil || len(got) != 2 || got[1].Err == nil {
		t.Fatal(got, e)
	}
	if got[0].Result.Events[0].Fields["v"] != json.Number("18446744073709551615") {
		t.Fatal("fresh result must match receipt numeric representation", got[0].Result)
	}
	count := calls.Load()
	got, e = b.ExecuteRecoverableBatch(ctx, request)
	if e != nil || len(got) != 2 || calls.Load() != count {
		t.Fatal("duplicate execution", got, e, calls.Load())
	}
	if got[0].Result.Events[0].Fields["v"] != json.Number("18446744073709551615") {
		t.Fatal(got[0].Result)
	}
	changed := request
	changed.Calls = append([]core.ExecutionRequest{}, request.Calls...)
	changed.Calls[0].Function = "different"
	if _, e := b.ExecuteRecoverableBatch(ctx, changed); !errors.Is(e, core.ErrBatchIdentityConflict) {
		t.Fatal(e)
	}
}

func TestRecoverableRejectsHostAndCustomArgumentsBeforeExecution(t *testing.T) {
	s, _ := state.NewBolt(state.BoltOptions{Path: filepath.Join(t.TempDir(), "state.db")})
	s.Start(context.Background())
	defer s.Stop(context.Background())
	exec := executorFunc(func(context.Context, core.ExecutionRequest, core.StateTransaction) (*core.ExecuteResult, error) {
		t.Fatal("invalid request executed")
		return nil, nil
	})
	reader := readerFunc(func(context.Context, string) (*core.CompiledContract, error) {
		t.Fatal("invalid input reached artifact lookup")
		return nil, nil
	})
	b, e := NewRecoverable(RecoverableOptions{ExecutionID: "v1"}, RecoverableDeps{State: s, Executor: exec, Contracts: reader})
	if e != nil {
		t.Fatal(e)
	}
	for _, call := range []core.ExecutionRequest{{CallContext: &core.CallContext{Host: &core.MemoryHost{}}}, {Args: []any{map[string]any{"x": 1}}}} {
		if _, e := b.ExecuteRecoverableBatch(context.Background(), core.RecoverableBatchRequest{ID: "bad", Calls: []core.ExecutionRequest{call}}); e == nil {
			t.Fatal("invalid input accepted")
		}
	}
	for _, o := range []RecoverableOptions{{}, {ExecutionID: "v1", Workers: 65}, {ExecutionID: "v1", MaxBatch: 65537}} {
		if _, e := NewRecoverable(o, RecoverableDeps{State: s, Executor: exec, Contracts: reader}); e == nil {
			t.Fatal(o)
		}
	}
}

func TestRecoverableCopiesContextAndBindsExecutionIdentity(t *testing.T) {
	ctx := context.Background()
	s, _ := state.NewBolt(state.BoltOptions{Path: filepath.Join(t.TempDir(), "state.db")})
	if e := s.Start(ctx); e != nil {
		t.Fatal(e)
	}
	defer s.Stop(ctx)
	c := artifacttest.Seal(t, &core.CompiledContract{ProtocolVersion: core.ProtocolVersion, Guarantees: core.PolicyGuarantees{Deterministic: true}})
	input := core.DeploymentInput{ChainID: "copy", Sender: "test"}
	c.Address, _ = deployment.DeriveAddress(input, c.ManifestSHA256)
	c.Deployment = &core.DeploymentRecord{Version: core.DeploymentVersion, Input: input, ArtifactSHA256: c.ManifestSHA256}
	sender := core.Address("alice")
	height := uint64(7)
	args := []any{uint64(18446744073709551615)}
	request := core.RecoverableBatchRequest{ID: "copy", Calls: []core.ExecutionRequest{{Address: c.Address, Function: "F", Args: args, CallContext: &core.CallContext{Sender: &sender, BlockHeight: &height}}}}
	exec := executorFunc(func(_ context.Context, r core.ExecutionRequest, _ core.StateTransaction) (*core.ExecuteResult, error) {
		sender = "changed"
		height = 9
		args[0] = uint64(0)
		request.Calls[0].Function = "changed"
		if r.Function != "F" || *r.CallContext.Sender != "alice" || *r.CallContext.BlockHeight != 7 || r.Args[0] != json.Number("18446744073709551615") {
			t.Error("caller mutation changed frozen request", r)
		}
		return &core.ExecuteResult{Data: []byte(`1`)}, nil
	})
	deps := RecoverableDeps{State: s, Executor: exec, Contracts: readerFunc(func(context.Context, string) (*core.CompiledContract, error) { return c, nil })}
	b, e := NewRecoverable(RecoverableOptions{ExecutionID: "v1"}, deps)
	if e != nil {
		t.Fatal(e)
	}
	if _, e := b.ExecuteRecoverableBatch(ctx, request); e != nil {
		t.Fatal(e)
	}
	sender = "alice"
	height = 7
	args[0] = uint64(18446744073709551615)
	request.Calls[0].Function = "F"
	different, e := NewRecoverable(RecoverableOptions{ExecutionID: "v2"}, deps)
	if e != nil {
		t.Fatal(e)
	}
	if _, e := different.ExecuteRecoverableBatch(ctx, request); !errors.Is(e, core.ErrBatchIdentityConflict) {
		t.Fatal(e)
	}
}
