package engine

import (
	"context"
	"errors"
	"github.com/lengzhao/vm/abi"
	"github.com/lengzhao/vm/core"
	"testing"
)

type depsStub struct {
	loads int
	run   func(context.Context) (*core.CallResult, error)
}

func (*depsStub) Review(string) error               { return nil }
func (*depsStub) ID() string                        { return "test-v1" }
func (*depsStub) Generate(string) (*abi.ABI, error) { return nil, nil }
func (*depsStub) Compile(ctx context.Context, _ string) (*core.CompiledContract, error) {
	return nil, ctx.Err()
}
func (*depsStub) Deploy(ctx context.Context, _ *core.CompiledContract) (string, error) {
	return "", ctx.Err()
}
func (d *depsStub) Get(ctx context.Context, _ string) (*core.CompiledContract, error) {
	d.loads++
	return &core.CompiledContract{}, ctx.Err()
}
func (d *depsStub) Run(ctx context.Context, _ *core.CompiledContract, _ core.CallRequest) (*core.CallResult, error) {
	return d.run(ctx)
}
func (*depsStub) NewMeter() core.GasMetering { return nil }
func (*depsStub) Estimate(*core.GasProfile, string, uint64) (uint64, map[string]uint64, error) {
	return 0, nil, nil
}
func (*depsStub) ValidateArtifact(ctx context.Context, _ *core.CompiledContract) error {
	return ctx.Err()
}
func testEngine(t *testing.T, d *depsStub) *Engine {
	t.Helper()
	e, err := New(Options{}, Deps{Policy: d, ABI: d, Compiler: d, Deployer: d, Contracts: d, Runner: d, Meters: d, Estimator: d, Validator: d})
	if err != nil {
		t.Fatal(err)
	}
	return e
}
func TestCanceledOperationDoesNotReadStorage(t *testing.T) {
	d := &depsStub{}
	e := testEngine(t, d)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := e.Execute(ctx, core.ExecutionRequest{Address: "x", Function: "F"}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := e.Get(ctx, "x"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if d.loads != 0 {
		t.Fatalf("canceled operation reached storage: %d", d.loads)
	}
}
func TestCallerCancellationReachesRunner(t *testing.T) {
	entered := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d := &depsStub{run: func(ctx context.Context) (*core.CallResult, error) {
		close(entered)
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	e := testEngine(t, d)
	done := make(chan error, 1)
	go func() { _, err := e.Execute(ctx, core.ExecutionRequest{Address: "x", Function: "F"}); done <- err }()
	<-entered
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
