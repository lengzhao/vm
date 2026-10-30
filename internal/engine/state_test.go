package engine

import (
	"context"
	"errors"
	"github.com/lengzhao/vm/internal/gas"
	"strings"
	"testing"

	"github.com/lengzhao/vm/core"
	"github.com/lengzhao/vm/internal/state"
)

func TestExecutionStateCommitAndRollback(t *testing.T) {
	for _, mode := range []string{"success", "error", "nil", "cancel", "dry_run"} {
		t.Run(mode, func(t *testing.T) {
			store := state.New()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			d := &depsStub{run: func(ctx context.Context) (*core.CallResult, error) {
				object, ok := core.ObjectStoreFromContext(ctx)
				if !ok {
					t.Fatal("runner lacks scoped object")
				}
				if err := object.Set(ctx, "field", []byte("value")); err != nil {
					t.Fatal(err)
				}
				result := &core.CallResult{Data: []byte("1"), Events: []core.Event{{Name: "written"}}, GasConsumed: 20}
				switch mode {
				case "error":
					return result, errors.New("failed")
				case "nil":
					return nil, nil
				case "cancel":
					cancel()
				}
				return result, nil
			}}
			e, err := New(Options{}, Deps{Policy: d, ABI: d, Compiler: d, Deployer: d, Contracts: d, Runner: d, Estimator: d, Validator: d, State: store})
			if err != nil {
				t.Fatal(err)
			}
			req := core.ExecutionRequest{Address: "real", Function: "F", CallContext: &core.CallContext{ContractAddress: core.AddressPtr("spoofed")}}
			var result *core.ExecuteResult
			if mode == "dry_run" {
				_, err = e.EstimateGas(ctx, "real", "F", &core.EstimateOptions{DryRun: true})
			} else {
				result, err = e.Execute(ctx, req)
			}
			if mode == "success" {
				if err != nil || len(result.Events) != 1 {
					t.Fatalf("result=%v err=%v", result, err)
				}
			} else if mode != "dry_run" {
				if err == nil {
					t.Fatal("expected failure")
				}
				if result != nil && (len(result.Events) > 0 || len(result.Data) > 0) {
					t.Fatal("failure published data/events")
				}
			} else if err != nil {
				t.Fatal(err)
			}
			view, _ := store.Begin(context.Background())
			defer view.Rollback()
			_, exists, _ := view.Get(context.Background(), core.StateKey{Contract: "real", Key: "field"})
			if exists != (mode == "success") {
				t.Fatalf("state committed in %s: %v", mode, exists)
			}
			_, exists, _ = view.Get(context.Background(), core.StateKey{Contract: "spoofed", Key: "field"})
			if exists {
				t.Fatal("context address escaped storage scope")
			}
		})
	}
}

type faultStore struct {
	core.StateStore
	commitErr, rollbackErr error
	commits, rollbacks     int
}
type faultTransaction struct {
	core.StateTransaction
	owner *faultStore
}

func (s *faultStore) Begin(ctx context.Context) (core.StateTransaction, error) {
	tx, err := s.StateStore.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return &faultTransaction{StateTransaction: tx, owner: s}, nil
}
func (tx *faultTransaction) Commit(ctx context.Context) error {
	tx.owner.commits++
	if tx.owner.commitErr != nil {
		return tx.owner.commitErr
	}
	return tx.StateTransaction.Commit(ctx)
}
func (tx *faultTransaction) Rollback() error {
	tx.owner.rollbacks++
	err := tx.StateTransaction.Rollback()
	return errors.Join(err, tx.owner.rollbackErr)
}

type lowMeterFactory struct{}

func (lowMeterFactory) NewMeter() core.GasMetering { m := gas.NewMeter(); m.SetGasLimit(1); return m }

func TestStateFailureAndGasRejectionRollback(t *testing.T) {
	for _, mode := range []string{"conflict", "backend", "ignored-object-error", "gas", "panic", "rollback"} {
		t.Run(mode, func(t *testing.T) {
			base := state.New()
			sentinel := errors.New("storage failure")
			store := &faultStore{StateStore: base}
			if mode == "conflict" {
				store.commitErr = core.ErrStateConflict
			}
			if mode == "backend" {
				store.commitErr = sentinel
			}
			if mode == "rollback" {
				store.rollbackErr = sentinel
			}
			d := &depsStub{run: func(ctx context.Context) (*core.CallResult, error) {
				o, _ := core.ObjectStoreFromContext(ctx)
				if err := o.Set(ctx, "x", []byte("pending")); err != nil {
					t.Fatal(err)
				}
				if mode == "ignored-object-error" {
					_ = o.Set(ctx, strings.Repeat("x", core.MaxObjectKeyBytes+1), nil)
				}
				if mode == "panic" {
					panic("custom runner panic")
				}
				result := &core.CallResult{Data: []byte("1"), Events: []core.Event{{Name: "pending"}}, GasConsumed: 20}
				if mode == "rollback" {
					return result, errors.New("runner failed")
				}
				return result, nil
			}}
			options := Options{}
			deps := Deps{State: store, Policy: d, ABI: d, Compiler: d, Deployer: d, Contracts: d, Runner: d, Estimator: d, Validator: d}
			if mode == "gas" {
				options.EnableGasMetering = true
				options.MaxGasLimit = 1
				deps.Meters = lowMeterFactory{}
			}
			e, err := New(options, deps)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "panic" {
				func() {
					defer func() {
						if recover() == nil {
							t.Error("panic missing")
						}
					}()
					_, _ = e.Execute(context.Background(), core.ExecutionRequest{Address: "real", Function: "F"})
				}()
			} else {
				out, err := e.Execute(context.Background(), core.ExecutionRequest{Address: "real", Function: "F"})
				if err == nil {
					t.Fatal("expected error")
				}
				if out != nil && (len(out.Data) > 0 || len(out.Events) > 0) {
					t.Fatal("failure published result")
				}
				if mode == "conflict" && !errors.Is(err, core.ErrStateConflict) {
					t.Fatal(err)
				}
				if (mode == "backend" || mode == "rollback") && !errors.Is(err, sentinel) {
					t.Fatal(err)
				}
			}
			if store.rollbacks != 1 {
				t.Fatalf("rollbacks=%d", store.rollbacks)
			}
			view, _ := base.Begin(context.Background())
			defer view.Rollback()
			if _, exists, _ := view.Get(context.Background(), core.StateKey{Contract: "real", Key: "x"}); exists {
				t.Fatal("failure committed")
			}
		})
	}
}
