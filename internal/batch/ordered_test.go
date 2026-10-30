package batch

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lengzhao/vm/core"
	"github.com/lengzhao/vm/internal/state"
)

type executorFunc func(context.Context, core.ExecutionRequest, core.StateTransaction) (*core.ExecuteResult, error)

func (f executorFunc) ExecuteTransaction(c context.Context, r core.ExecutionRequest, tx core.StateTransaction) (*core.ExecuteResult, error) {
	return f(c, r, tx)
}
func makeBatch(t *testing.T, workers int, s core.SnapshotStore, e core.TransactionExecutor) *Ordered {
	t.Helper()
	b, err := New(Options{Workers: workers}, Deps{State: s, Executor: e})
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func TestOrderedMatchesSerialAndReexecutesStaleFailure(t *testing.T) {
	for _, workers := range []int{1, 2, 4} {
		t.Run(strconv.Itoa(workers), func(t *testing.T) {
			s := state.NewMVCC()
			var calls atomic.Int32
			e := executorFunc(func(ctx context.Context, r core.ExecutionRequest, tx core.StateTransaction) (*core.ExecuteResult, error) {
				calls.Add(1)
				k := core.StateKey{Contract: "a", Key: "counter"}
				v, _, err := tx.Get(ctx, k)
				if err != nil {
					return nil, err
				}
				n, _ := strconv.Atoi(string(v))
				if r.Function == "NeedValue" && n == 0 {
					return &core.ExecuteResult{GasConsumed: 7}, &core.ExecutionError{Code: core.CodeContractError, Message: "missing"}
				}
				if r.Function == "Read" {
					return &core.ExecuteResult{Data: []byte(strconv.Itoa(n)), GasConsumed: uint64(n + 1)}, nil
				}
				if err := tx.Set(ctx, k, []byte(strconv.Itoa(n+1))); err != nil {
					return nil, err
				}
				return &core.ExecuteResult{Data: []byte(strconv.Itoa(n + 1)), Events: []core.Event{{Name: fmt.Sprint(n + 1)}}, GasConsumed: uint64(n + 10)}, nil
			})
			b := makeBatch(t, workers, s, e)
			requests := []core.ExecutionRequest{{Function: "Write"}, {Function: "NeedValue"}, {Function: "Read"}, {Function: "Write"}}
			got, err := b.ExecuteBatch(context.Background(), requests)
			if err != nil || len(got) != 4 {
				t.Fatal(got, err)
			}
			for i, want := range []string{"1", "2", "2", "3"} {
				if got[i].Err != nil || got[i].Result == nil || string(got[i].Result.Data) != want {
					t.Fatalf("item %d: %+v", i, got[i])
				}
			}
			for i, want := range []uint64{10, 11, 3, 12} {
				if got[i].Result.GasConsumed != want {
					t.Fatal("speculative gas leaked", got)
				}
			}
			if workers > 1 && calls.Load() <= 4 {
				t.Fatal("did not retry stale observations")
			}
		})
	}
}
func TestOrderedIndependentWritesAndFailureDoNotPublish(t *testing.T) {
	s := state.NewMVCC()
	var calls atomic.Int32
	e := executorFunc(func(ctx context.Context, r core.ExecutionRequest, tx core.StateTransaction) (*core.ExecuteResult, error) {
		calls.Add(1)
		tx.Set(ctx, core.StateKey{Key: r.Function}, []byte("v"))
		result := &core.ExecuteResult{Data: []byte(`1`), Events: []core.Event{{Name: "ok"}}, GasConsumed: 5}
		if r.Function == "fail" {
			return result, &core.ExecutionError{Code: core.CodeContractError, Message: "failure"}
		}
		return result, nil
	})
	got, err := makeBatch(t, 4, s, e).ExecuteBatch(context.Background(), []core.ExecutionRequest{{Function: "a"}, {Function: "fail"}, {Function: "b"}})
	if err != nil || len(got) != 3 || got[1].Err == nil || calls.Load() != 3 {
		t.Fatal(got, err, calls.Load())
	}
	if got[1].Result.Data != nil || got[1].Result.Events != nil {
		t.Fatal("failed output leaked")
	}
	tx, _ := s.Begin(context.Background())
	defer tx.Rollback()
	if _, ok, _ := tx.Get(context.Background(), core.StateKey{Key: "fail"}); ok {
		t.Fatal("failed writes committed")
	}
}
func TestOrderedBoundedConcurrencyAndCancellation(t *testing.T) {
	var active, maxActive atomic.Int32
	entered := make(chan struct{}, 2)
	e := executorFunc(func(ctx context.Context, _ core.ExecutionRequest, _ core.StateTransaction) (*core.ExecuteResult, error) {
		n := active.Add(1)
		defer active.Add(-1)
		for old := maxActive.Load(); n > old; old = maxActive.Load() {
			if maxActive.CompareAndSwap(old, n) {
				break
			}
		}
		entered <- struct{}{}
		<-ctx.Done()
		return nil, ctx.Err()
	})
	b := makeBatch(t, 2, state.NewMVCC(), e)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := b.ExecuteBatch(ctx, make([]core.ExecutionRequest, 4)); done <- err }()
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			t.Fatal("workers did not start")
		}
	}
	// A second batch cannot enter while the first owns this scheduler.
	waiting, stop := context.WithCancel(context.Background())
	stop()
	if _, err := b.ExecuteBatch(waiting, nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancellation stuck")
	}
	if maxActive.Load() != 2 || active.Load() != 0 {
		t.Fatal(maxActive.Load(), active.Load())
	}
}

type trackedStore struct {
	base           *state.MVCC
	mu             sync.Mutex
	opened, closed int
	rollbackErr    error
}
type trackedTx struct {
	core.SnapshotTransaction
	owner *trackedStore
	once  sync.Once
}

func (s *trackedStore) BeginSnapshot(ctx context.Context) (core.SnapshotTransaction, error) {
	tx, err := s.base.BeginSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.opened++
	s.mu.Unlock()
	return &trackedTx{SnapshotTransaction: tx, owner: s}, nil
}
func (t *trackedTx) Rollback() error {
	t.once.Do(func() { t.owner.mu.Lock(); t.owner.closed++; t.owner.mu.Unlock() })
	return errors.Join(t.SnapshotTransaction.Rollback(), t.owner.rollbackErr)
}
func TestOrderedRollbackFailureStopsAndCleansWindow(t *testing.T) {
	boom := errors.New("rollback failed")
	s := &trackedStore{base: state.NewMVCC(), rollbackErr: boom}
	e := executorFunc(func(context.Context, core.ExecutionRequest, core.StateTransaction) (*core.ExecuteResult, error) {
		return nil, &core.ExecutionError{Code: core.CodeContractError, Message: "business"}
	})
	got, err := makeBatch(t, 3, s, e).ExecuteBatch(context.Background(), make([]core.ExecutionRequest, 6))
	if !errors.Is(err, boom) || len(got) != 0 {
		t.Fatal(got, err)
	}
	if s.opened != 3 || s.closed != 3 {
		t.Fatal(s.opened, s.closed)
	}
}
func TestOrderedRejectsLimitsAndNilDependencies(t *testing.T) {
	for _, o := range []Options{{Workers: -1}, {Workers: 65}, {MaxBatch: -1}, {MaxBatch: 65537}} {
		if _, err := New(o, Deps{}); err == nil {
			t.Fatal(o)
		}
	}
	if _, err := New(Options{}, Deps{}); err == nil {
		t.Fatal("nil deps")
	}
	e := executorFunc(func(context.Context, core.ExecutionRequest, core.StateTransaction) (*core.ExecuteResult, error) {
		t.Fatal("executed oversized batch")
		return nil, nil
	})
	b, err := New(Options{MaxBatch: 1}, Deps{State: state.NewMVCC(), Executor: e})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.ExecuteBatch(context.Background(), make([]core.ExecutionRequest, 2)); err == nil {
		t.Fatal("batch limit ignored")
	}
}

func TestOrderedInfrastructureErrorStopsAtCommittedPrefix(t *testing.T) {
	business := &core.ExecutionError{Code: core.CodeContractError, Message: "business"}
	process := &core.ExecutionError{Code: core.CodeProcessFailure, Message: "cleanup failed"}
	for _, failure := range []error{process, errors.Join(business, process), &core.ExecutionError{Code: core.CodeStateFailure, Message: "state failed"}, context.Canceled, errors.New("unknown infrastructure")} {
		t.Run(failure.Error(), func(t *testing.T) {
			s := state.NewMVCC()
			e := executorFunc(func(ctx context.Context, r core.ExecutionRequest, tx core.StateTransaction) (*core.ExecuteResult, error) {
				tx.Set(ctx, core.StateKey{Key: r.Function}, []byte("v"))
				if r.Function == "stop" {
					return nil, failure
				}
				return &core.ExecuteResult{Data: []byte(`1`)}, nil
			})
			got, err := makeBatch(t, 3, s, e).ExecuteBatch(context.Background(), []core.ExecutionRequest{{Function: "first"}, {Function: "stop"}, {Function: "later"}})
			if !errors.Is(err, failure) || len(got) != 1 {
				t.Fatalf("prefix=%d err=%v", len(got), err)
			}
			tx, _ := s.Begin(context.Background())
			defer tx.Rollback()
			for _, key := range []string{"first", "stop", "later"} {
				_, exists, _ := tx.Get(context.Background(), core.StateKey{Key: key})
				if exists != (key == "first") {
					t.Fatal("published wrong prefix", key)
				}
			}
		})
	}
}

type failingSnapshotStore struct {
	core.SnapshotStore
	begins int
	failAt int
	err    error
}

func (s *failingSnapshotStore) BeginSnapshot(ctx context.Context) (core.SnapshotTransaction, error) {
	s.begins++
	if s.begins == s.failAt {
		return nil, s.err
	}
	return s.SnapshotStore.BeginSnapshot(ctx)
}
func TestOrderedBeginFailureCleansUnexecutedSnapshots(t *testing.T) {
	tracked := &trackedStore{base: state.NewMVCC()}
	boom := errors.New("snapshot unavailable")
	s := &failingSnapshotStore{SnapshotStore: tracked, failAt: 2, err: boom}
	e := executorFunc(func(context.Context, core.ExecutionRequest, core.StateTransaction) (*core.ExecuteResult, error) {
		t.Fatal("partial window executed")
		return nil, nil
	})
	got, err := makeBatch(t, 3, s, e).ExecuteBatch(context.Background(), make([]core.ExecutionRequest, 3))
	if !errors.Is(err, boom) || len(got) != 0 || tracked.closed != 1 {
		t.Fatal(got, err, tracked.closed)
	}
}

type conflictingStore struct {
	core.SnapshotStore
	commits atomic.Int32
}
type conflictingTx struct {
	core.SnapshotTransaction
	owner *conflictingStore
}

func (s *conflictingStore) BeginSnapshot(ctx context.Context) (core.SnapshotTransaction, error) {
	tx, e := s.SnapshotStore.BeginSnapshot(ctx)
	if e != nil {
		return nil, e
	}
	return &conflictingTx{SnapshotTransaction: tx, owner: s}, nil
}
func (t *conflictingTx) Commit(context.Context) error {
	t.owner.commits.Add(1)
	return core.ErrStateConflict
}
func TestOrderedSecondConflictStopsWithoutInfiniteRetry(t *testing.T) {
	s := &conflictingStore{SnapshotStore: state.NewMVCC()}
	var calls atomic.Int32
	e := executorFunc(func(context.Context, core.ExecutionRequest, core.StateTransaction) (*core.ExecuteResult, error) {
		calls.Add(1)
		return &core.ExecuteResult{}, nil
	})
	got, err := makeBatch(t, 1, s, e).ExecuteBatch(context.Background(), make([]core.ExecutionRequest, 2))
	if !errors.Is(err, core.ErrStateConflict) || len(got) != 0 || calls.Load() != 2 || s.commits.Load() != 2 {
		t.Fatal(got, err, calls.Load(), s.commits.Load())
	}
}
