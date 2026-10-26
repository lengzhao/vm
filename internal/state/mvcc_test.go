package state

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/lengzhao/vm/core"
)

func snap(t *testing.T, s *MVCC) core.SnapshotTransaction {
	t.Helper()
	tx, e := s.BeginSnapshot(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { tx.Rollback() })
	return tx
}
func put(t *testing.T, tx core.StateTransaction, k core.StateKey, v string) {
	t.Helper()
	if e := tx.Set(context.Background(), k, []byte(v)); e != nil {
		t.Fatal(e)
	}
}
func mvccCommit(t *testing.T, tx core.StateTransaction) {
	t.Helper()
	if e := tx.Commit(context.Background()); e != nil {
		t.Fatal(e)
	}
}
func TestMVCCIndependentAndConflictingWrites(t *testing.T) {
	s := NewMVCC()
	ctx := context.Background()
	a := core.StateKey{Contract: "a", Key: "x"}
	b := core.StateKey{Contract: "a", Key: "y"}
	x, y := snap(t, s), snap(t, s)
	put(t, x, a, "one")
	put(t, y, b, "two")
	mvccCommit(t, x)
	mvccCommit(t, y)
	x, y = snap(t, s), snap(t, s)
	put(t, x, a, "new")
	put(t, y, a, "lost")
	mvccCommit(t, x)
	if e := y.Commit(ctx); !errors.Is(e, core.ErrStateConflict) {
		t.Fatalf("blind write conflict: %v", e)
	}
	v, _, _ := snap(t, s).Get(ctx, a)
	if string(v) != "new" {
		t.Fatal(string(v))
	}
}
func TestMVCCMissingReadAndABA(t *testing.T) {
	ctx := context.Background()
	s := NewMVCC()
	k := core.StateKey{Key: "absent"}
	reader := snap(t, s)
	if _, exists, e := reader.Get(ctx, k); exists || e != nil {
		t.Fatal(exists, e)
	}
	w := snap(t, s)
	put(t, w, k, "x")
	mvccCommit(t, w)
	w = snap(t, s)
	if e := w.Delete(ctx, k); e != nil {
		t.Fatal(e)
	}
	mvccCommit(t, w)
	if e := reader.Validate(ctx); !errors.Is(e, core.ErrStateConflict) {
		t.Fatalf("missing ABA not detected: %v", e)
	}
	if e := reader.Commit(ctx); !errors.Is(e, core.ErrStateConflict) {
		t.Fatalf("read-only commit accepted stale read: %v", e)
	}
	// Even deleting an already missing key is a conflicting write.
	reader = snap(t, s)
	reader.Get(ctx, k)
	w = snap(t, s)
	w.Delete(ctx, k)
	mvccCommit(t, w)
	if e := reader.Validate(ctx); !errors.Is(e, core.ErrStateConflict) {
		t.Fatal(e)
	}
}
func TestMVCCReadWriteSkewAndAtomicFailure(t *testing.T) {
	ctx := context.Background()
	s := NewMVCC()
	a, b := core.StateKey{Key: "a"}, core.StateKey{Key: "b"}
	x, y := snap(t, s), snap(t, s)
	x.Get(ctx, b)
	y.Get(ctx, a)
	put(t, x, a, "x")
	put(t, y, b, "y")
	mvccCommit(t, x)
	if e := y.Commit(ctx); !errors.Is(e, core.ErrStateConflict) {
		t.Fatal(e)
	}
	if _, ok, _ := snap(t, s).Get(ctx, b); ok {
		t.Fatal("failed commit published")
	}
	if len(y.ReadSet()) != 1 || len(y.WriteSet()) != 1 {
		t.Fatal("sets lost")
	}
	if e := y.Rollback(); e != nil {
		t.Fatal(e)
	}
	if e := y.Rollback(); e != nil {
		t.Fatal(e)
	}
	if e := y.Validate(ctx); !errors.Is(e, core.ErrStateClosed) {
		t.Fatal(e)
	}
}
func TestMVCCCopiesContextsAndConcurrency(t *testing.T) {
	s := NewMVCC()
	ctx := context.Background()
	k := core.StateKey{Key: "k"}
	tx := snap(t, s)
	data := []byte("x")
	tx.Set(ctx, k, data)
	data[0] = '!'
	v, _, _ := tx.Get(ctx, k)
	v[0] = '!'
	ws := tx.WriteSet()
	ws[0].Value[0] = '!'
	mvccCommit(t, tx)
	v, _, _ = snap(t, s).Get(ctx, k)
	if string(v) != "x" {
		t.Fatal(string(v))
	}
	cancelCtx, cancel := context.WithCancel(ctx)
	c, e := s.BeginSnapshot(cancelCtx)
	if e != nil {
		t.Fatal(e)
	}
	cancel()
	if e := c.Commit(ctx); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	c.Rollback()
	if _, e := s.BeginSnapshot(cancelCtx); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			x, _ := s.BeginSnapshot(ctx)
			defer x.Rollback()
			x.Set(ctx, core.StateKey{Key: string(rune('A' + i))}, []byte("ok"))
			errs <- x.Commit(ctx)
		}(i)
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
}
