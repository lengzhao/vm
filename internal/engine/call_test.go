package engine

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/lengzhao/vm/core"
	"github.com/lengzhao/vm/internal/callcontext"
	"github.com/lengzhao/vm/internal/state"
)

func TestCrossCallSharesTemporaryStore(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "commit", true: "discard"}[fail], func(t *testing.T) {
			base := state.New()
			store := &faultStore{StateStore: base}
			sentinel := errors.New("child failed")
			d := &depsStub{run: func(ctx context.Context) (*core.CallResult, error) {
				o, _ := core.ObjectStoreFromContext(ctx)
				_ = o.Set(ctx, "x", []byte("pending"))
				cc, _ := core.ContractCallerFromContext(ctx)
				contextValue, _ := callcontext.CallContextFrom(ctx)
				if *contextValue.ContractAddress == "root" {
					child, err := cc.Call(ctx, core.CrossCallRequest{Address: "child", Function: "F", GasConsumed: 20, Events: []core.Event{{Name: "before"}}})
					if fail {
						if err == nil {
							t.Fatal("missing child error")
						}
						return &core.CallResult{Data: []byte("1"), GasConsumed: 50}, nil
					}
					if err != nil {
						t.Fatal(err)
					}
					if child.GasConsumed != 15 {
						t.Fatal(child.GasConsumed)
					}
					return &core.CallResult{Data: []byte("1"), GasConsumed: 40, Events: []core.Event{{Name: "after"}}}, nil
				}
				if *contextValue.Sender != "root" {
					t.Fatal("wrong direct sender")
				}
				if fail {
					return nil, sentinel
				}
				return &core.CallResult{Data: []byte("2"), GasConsumed: 15, Events: []core.Event{{Name: "child"}}}, nil
			}}
			e, err := New(Options{}, Deps{State: store, Policy: d, ABI: d, Compiler: d, Deployer: d, Contracts: d, Runner: d, Estimator: d, Validator: d})
			if err != nil {
				t.Fatal(err)
			}
			out, err := e.Execute(context.Background(), core.ExecutionRequest{Address: "root", Function: "F"})
			if fail {
				if !errors.Is(err, sentinel) {
					t.Fatalf("ignored child failure: %v", err)
				}
				if store.commits != 0 {
					t.Fatal("failed chain committed")
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if store.commits != 1 || len(out.Events) != 3 || out.Events[0].Name != "before" || out.Events[1].Name != "child" || out.Events[2].Name != "after" {
					t.Fatalf("out=%+v commits=%d", out, store.commits)
				}
			}
			view, _ := base.Begin(context.Background())
			defer view.Rollback()
			for _, address := range []core.Address{"root", "child"} {
				_, exists, _ := view.Get(context.Background(), core.StateKey{Contract: address, Key: "x"})
				if exists == fail {
					t.Fatalf("%s exists=%v", address, exists)
				}
			}
		})
	}
}

func TestCallDepthReentrancyAndSharedBudget(t *testing.T) {
	for _, mode := range []string{"depth", "reentry", "budget", "success"} {
		t.Run(mode, func(t *testing.T) {
			base := state.New()
			store := &faultStore{StateStore: base}
			runs := 0
			d := &depsStub{run: func(ctx context.Context) (*core.CallResult, error) {
				runs++
				cc, _ := callcontext.CallContextFrom(ctx)
				address := string(*cc.ContractAddress)
				o, _ := core.ObjectStoreFromContext(ctx)
				_ = o.Set(ctx, "x", []byte(address))
				n, _ := strconv.Atoi(address)
				if mode == "success" && n == 7 {
					return &core.CallResult{Data: []byte("1"), GasConsumed: 11}, nil
				}
				caller, _ := core.ContractCallerFromContext(ctx)
				next := strconv.Itoa(n + 1)
				if mode == "reentry" {
					next = "0"
				}
				result, err := caller.Call(ctx, core.CrossCallRequest{Address: next, Function: "F", GasConsumed: 20})
				if err != nil {
					return nil, err
				}
				return &core.CallResult{Data: []byte("1"), GasConsumed: 20 + result.GasConsumed}, nil
			}}
			options := Options{}
			if mode == "depth" {
				options.MaxCallDepth = 3
			}
			e, err := New(options, Deps{State: store, Policy: d, ABI: d, Compiler: d, Deployer: d, Contracts: d, Runner: d, Estimator: d, Validator: d})
			if err != nil {
				t.Fatal(err)
			}
			req := core.ExecutionRequest{Address: "0", Function: "F"}
			if mode == "budget" {
				req.CallContext = &core.CallContext{GasLimit: core.Uint64Ptr(25)}
			}
			out, err := e.Execute(context.Background(), req)
			if mode == "success" {
				if err != nil || runs != 8 || store.commits != 1 || out.GasConsumed != 151 {
					t.Fatalf("out=%+v err=%v runs=%d", out, err, runs)
				}
				return
			}
			var ee *core.ExecutionError
			if !errors.As(err, &ee) {
				t.Fatal(err)
			}
			want := map[string]core.ErrorCode{"depth": core.CodeCallDepth, "reentry": core.CodeReentrancy, "budget": core.CodeGasLimitExceeded}[mode]
			if ee.Code != want || store.commits != 0 || store.rollbacks != 1 {
				t.Fatalf("err=%v commits=%d rollback=%d", err, store.commits, store.rollbacks)
			}
			if mode == "budget" && runs != 1 {
				t.Fatal("zero remainder became unlimited child budget")
			}
		})
	}
}
func TestCrossCallLimitsAndUnderreportedGas(t *testing.T) {
	for _, mode := range []string{"call-count", "events", "underreported", "overflow"} {
		t.Run(mode, func(t *testing.T) {
			base := state.New()
			store := &faultStore{StateStore: base}
			d := &depsStub{run: func(ctx context.Context) (*core.CallResult, error) {
				cc, _ := callcontext.CallContextFrom(ctx)
				if *cc.ContractAddress == "child" {
					return &core.CallResult{Data: []byte("1"), GasConsumed: 11}, nil
				}
				caller, _ := core.ContractCallerFromContext(ctx)
				used := uint64(10)
				count := 1
				if mode == "call-count" {
					count = core.MaxCrossCalls + 1
				}
				for i := 0; i < count; i++ {
					request := core.CrossCallRequest{Address: "child", Function: "F", GasConsumed: used}
					if mode == "events" {
						request.Events = []core.Event{{Name: strings.Repeat("x", core.MaxCallEventBytes)}}
					}
					if mode == "overflow" {
						request.GasConsumed = ^uint64(0)
					}
					child, err := caller.Call(ctx, request)
					if err != nil {
						return nil, err
					}
					used += child.GasConsumed + 10
				}
				gas := used
				if mode == "underreported" {
					gas = 1
				}
				return &core.CallResult{Data: []byte("1"), GasConsumed: gas}, nil
			}}
			e, err := New(Options{}, Deps{State: store, Policy: d, ABI: d, Compiler: d, Deployer: d, Contracts: d, Runner: d, Estimator: d, Validator: d})
			if err != nil {
				t.Fatal(err)
			}
			_, err = e.Execute(context.Background(), core.ExecutionRequest{Address: "root", Function: "F"})
			if err == nil || store.commits != 0 || store.rollbacks != 1 {
				t.Fatalf("err=%v commits=%d", err, store.commits)
			}
		})
	}
}

func TestCrossCallCannotDetachRootCancellation(t *testing.T) {
	store := &faultStore{StateStore: state.New()}
	entered := make(chan struct{})
	d := &depsStub{run: func(ctx context.Context) (*core.CallResult, error) {
		cc, _ := callcontext.CallContextFrom(ctx)
		if *cc.ContractAddress == "child" {
			close(entered)
			<-ctx.Done()
			return nil, ctx.Err()
		}
		caller, _ := core.ContractCallerFromContext(ctx)
		_, err := caller.Call(context.Background(), core.CrossCallRequest{Address: "child", Function: "F", GasConsumed: 20})
		return nil, err
	}}
	e, err := New(Options{}, Deps{State: store, Policy: d, ABI: d, Compiler: d, Deployer: d, Contracts: d, Runner: d, Estimator: d, Validator: d})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := e.Execute(ctx, core.ExecutionRequest{Address: "root", Function: "F"}); done <- err }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("child did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("child detached root cancellation")
	}
	if store.commits != 0 || store.rollbacks != 1 {
		t.Fatalf("commits=%d rollbacks=%d", store.commits, store.rollbacks)
	}
}
func TestCallDepthConfiguration(t *testing.T) {
	d := &depsStub{}
	deps := Deps{State: state.New(), Policy: d, ABI: d, Compiler: d, Deployer: d, Contracts: d, Runner: d, Estimator: d, Validator: d}
	for _, depth := range []int{-1, 33} {
		if _, err := New(Options{MaxCallDepth: depth}, deps); err == nil {
			t.Fatalf("accepted depth %d", depth)
		}
	}
	for _, depth := range []int{0, 1, 32} {
		if _, err := New(Options{MaxCallDepth: depth}, deps); err != nil {
			t.Fatal(err)
		}
	}
}
