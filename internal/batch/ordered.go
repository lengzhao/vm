// Package batch implements bounded speculative execution with ordered commits.
package batch

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/lengzhao/vm/core"
)

type Options struct{ Workers, MaxBatch int }
type Deps struct {
	State    core.SnapshotStore
	Executor core.TransactionExecutor
}
type Ordered struct {
	options Options
	deps    Deps
	gate    chan struct{}
}

var _ core.BatchExecutor = (*Ordered)(nil)

func New(o Options, d Deps) (*Ordered, error) {
	var err error
	o, err = normalizeOptions(o)
	if err != nil {
		return nil, err
	}
	if d.State == nil || d.Executor == nil {
		return nil, fmt.Errorf("batch state and transaction executor required")
	}
	return &Ordered{options: o, deps: d, gate: make(chan struct{}, 1)}, nil
}
func normalizeOptions(o Options) (Options, error) {
	if o.Workers == 0 {
		o.Workers = 4
	}
	if o.MaxBatch == 0 {
		o.MaxBatch = 1024
	}
	if o.Workers < 1 || o.Workers > 64 || o.MaxBatch < 1 || o.MaxBatch > 65536 {
		return o, fmt.Errorf("workers must be 1–64 and maxBatch 1–65536")
	}
	return o, nil
}
func (b *Ordered) ExecuteBatch(ctx context.Context, requests []core.ExecutionRequest) ([]core.BatchItemResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(requests) > b.options.MaxBatch {
		return nil, fmt.Errorf("batch exceeds maxBatch %d", b.options.MaxBatch)
	}
	select {
	case b.gate <- struct{}{}:
		defer func() { <-b.gate }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	results := make([]core.BatchItemResult, 0, len(requests))
	for start := 0; start < len(requests); start += b.options.Workers {
		end := min(start+b.options.Workers, len(requests))
		window, err := b.window(ctx, requests[start:end])
		results = append(results, window...)
		if err != nil {
			return results, err
		}
	}
	return results, nil
}

type attempt struct {
	tx     core.SnapshotTransaction
	result *core.ExecuteResult
	err    error
}

func (b *Ordered) begin(ctx context.Context) (*attempt, error) {
	return b.beginWith(ctx, b.deps.State)
}
func (b *Ordered) beginWith(ctx context.Context, state core.SnapshotStore) (*attempt, error) {
	tx, err := state.BeginSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	if tx == nil {
		return nil, fmt.Errorf("snapshot store returned nil transaction")
	}
	return &attempt{tx: tx}, nil
}
func (b *Ordered) run(ctx context.Context, req core.ExecutionRequest, a *attempt) {
	a.result, a.err = b.deps.Executor.ExecuteTransaction(ctx, req, a.tx)
	if a.err == nil && a.result == nil {
		a.err = &core.ExecutionError{Code: core.CodeInvalidResponse, Message: "transaction executor returned nil result"}
	}
}
func (a *attempt) resolve(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if a.err != nil {
		return a.tx.Validate(ctx)
	}
	return a.tx.Commit(ctx)
}
func (a *attempt) discard() error {
	if a == nil || a.tx == nil {
		return nil
	}
	err := a.tx.Rollback()
	a.tx = nil
	return err
}
func (b *Ordered) window(ctx context.Context, requests []core.ExecutionRequest) ([]core.BatchItemResult, error) {
	return b.windowWith(ctx, requests, b.deps.State, func(ctx context.Context, _ int, a *attempt) error { return a.resolve(ctx) })
}
func (b *Ordered) windowWith(ctx context.Context, requests []core.ExecutionRequest, state core.SnapshotStore, resolve func(context.Context, int, *attempt) error) (out []core.BatchItemResult, returnErr error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	attempts := make([]*attempt, len(requests))
	defer func() {
		for _, a := range attempts {
			returnErr = errors.Join(returnErr, a.discard())
		}
	}()
	// Begin the entire bounded window before executing or committing any member.
	for i := range requests {
		a, err := b.beginWith(ctx, state)
		if err != nil {
			return out, err
		}
		attempts[i] = a
	}
	var wg sync.WaitGroup
	for i, a := range attempts {
		wg.Add(1)
		go func(i int, a *attempt) { defer wg.Done(); b.run(ctx, requests[i], a) }(i, a)
	}
	wg.Wait()
	for i, a := range attempts {
		if a.err != nil && !transactionFailure(a.err) {
			return out, a.err
		}
		err := resolve(ctx, i, a)
		if errors.Is(err, core.ErrStateConflict) {
			if cleanupErr := a.discard(); cleanupErr != nil {
				return out, errors.Join(err, cleanupErr)
			}
			a, err = b.beginWith(ctx, state)
			if err != nil {
				return out, err
			}
			attempts[i] = a
			b.run(ctx, requests[i], a)
			if a.err != nil && !transactionFailure(a.err) {
				return out, a.err
			}
			err = resolve(ctx, i, a)
		}
		// A second conflict signals an external writer or incompatible state plugin.
		// Do not spin, publish stale output, or advance past an unresolved item.
		if err != nil {
			return out, err
		}
		if a.err != nil {
			if err := a.discard(); err != nil {
				return out, err
			}
			if a.result != nil {
				a.result.Data = nil
				a.result.Events = nil
			}
		} else {
			a.tx = nil
		} // Successful Commit owns closing the transaction.
		out = append(out, core.BatchItemResult{Result: a.result, Err: a.err})
	}
	return out, nil
}

// Only explicit contract-level failures are item results. In particular, a
// joined business error and sandbox cleanup error must stop the batch: checking
// only the first errors.As match would hide the infrastructure failure.
func transactionFailure(err error) bool {
	if err == nil {
		return true
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		for _, child := range joined.Unwrap() {
			if !transactionFailure(child) {
				return false
			}
		}
		return true
	}
	if e, ok := err.(*core.ExecutionError); ok {
		switch e.Code {
		case core.CodeInvalidRequest, core.CodeInvalidArguments, core.CodeUnknownFunction,
			core.CodeContractError, core.CodeContractPanic, core.CodeGasLimitExceeded,
			core.CodeInvalidResult, core.CodeCallDepth, core.CodeReentrancy:
			return !fatalCause(e.Cause)
		case core.CodeCallFailure:
			return e.Cause == nil || transactionFailure(e.Cause)
		default:
			return false
		}
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		return transactionFailure(wrapped.Unwrap())
	}
	return false
}
func fatalCause(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	switch e := err.(type) {
	case *core.ExecutionError:
		return !transactionFailure(e)
	case interface{ Unwrap() []error }:
		return !transactionFailure(err)
	case interface{ Unwrap() error }:
		return fatalCause(e.Unwrap())
	}
	return false // Diagnostic causes beneath a classified contract error are allowed.
}
