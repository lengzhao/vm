package state

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/lengzhao/vm/core"
)

var bg = context.Background()
var key = core.StateKey{Contract: "alice", Key: "balance"}

func BenchmarkBeginSnapshot(b *testing.B) {
	for _, size := range []int{0, 1024} {
		b.Run(fmt.Sprintf("keys=%d", size), func(b *testing.B) {
			s := New()
			tx, err := s.Begin(bg)
			if err != nil {
				b.Fatal(err)
			}
			for i := range size {
				if err := tx.Set(bg, core.StateKey{Contract: "alice", Key: fmt.Sprint(i)}, make([]byte, 1024)); err != nil {
					b.Fatal(err)
				}
			}
			if err := tx.Commit(bg); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				tx, err := s.Begin(bg)
				if err != nil {
					b.Fatal(err)
				}
				if err := tx.Rollback(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func begin(t *testing.T, s core.StateStore) core.StateTransaction {
	t.Helper()
	tx, err := s.Begin(bg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback() })
	return tx
}
func set(t *testing.T, tx core.StateTransaction, k core.StateKey, v []byte) {
	t.Helper()
	if err := tx.Set(bg, k, v); err != nil {
		t.Fatal(err)
	}
}
func get(t *testing.T, tx core.StateTransaction, k core.StateKey, want string, exists bool) {
	t.Helper()
	v, ok, err := tx.Get(bg, k)
	if err != nil || ok != exists || string(v) != want {
		t.Fatalf("Get=%q,%v,%v want %q,%v", v, ok, err, want, exists)
	}
}
func commit(t *testing.T, tx core.StateTransaction) {
	t.Helper()
	if err := tx.Commit(bg); err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotCommitRollback(t *testing.T) {
	s := New()
	old := begin(t, s)
	writer := begin(t, s)
	set(t, writer, key, []byte("10"))
	get(t, old, key, "", false)
	get(t, writer, key, "10", true)
	commit(t, writer)
	get(t, old, key, "", false)
	get(t, begin(t, s), key, "10", true)
	tx := begin(t, s)
	if err := tx.Delete(bg, key); err != nil {
		t.Fatal(err)
	}
	get(t, tx, key, "", false)
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	get(t, begin(t, s), key, "10", true)
	tx = begin(t, s)
	if err := tx.Delete(bg, key); err != nil {
		t.Fatal(err)
	}
	commit(t, tx)
	get(t, begin(t, s), key, "", false)
}

func TestCopiesNamespacesAndAccessSets(t *testing.T) {
	s := New()
	tx := begin(t, s)
	other := core.StateKey{Contract: "bob", Key: key.Key}
	value := []byte("abc")
	set(t, tx, key, value)
	value[0] = 'X'
	got, _, _ := tx.Get(bg, key)
	got[0] = 'Y'
	get(t, tx, key, "abc", true)
	set(t, tx, other, nil)
	get(t, tx, other, "", true)
	missing := core.StateKey{Contract: "bob", Key: "missing"}
	get(t, tx, missing, "", false)
	if err := tx.Delete(bg, missing); err != nil {
		t.Fatal(err)
	}
	reads := tx.ReadSet()
	want := []core.StateKey{key, other, missing}
	if !reflect.DeepEqual(reads, want) {
		t.Fatalf("reads=%v", reads)
	}
	reads[0] = missing
	writes := tx.WriteSet()
	if len(writes) != 3 || writes[0].Key != key || string(writes[0].Value) != "abc" || !writes[2].Deleted {
		t.Fatalf("writes=%v", writes)
	}
	writes[0].Value[0] = 'Z'
	writes[0].Deleted = true
	if string(tx.WriteSet()[0].Value) != "abc" || tx.ReadSet()[0] != key {
		t.Fatal("sets alias transaction")
	}
	commit(t, tx)
	get(t, begin(t, s), key, "abc", true)
	if _, _, err := tx.Get(bg, key); !errors.Is(err, core.ErrStateClosed) {
		t.Fatal(err)
	}
	if err := tx.Set(bg, key, nil); !errors.Is(err, core.ErrStateClosed) {
		t.Fatal(err)
	}
	if err := tx.Delete(bg, key); !errors.Is(err, core.ErrStateClosed) {
		t.Fatal(err)
	}
	if err := tx.Commit(bg); !errors.Is(err, core.ErrStateClosed) {
		t.Fatal(err)
	}
}

func TestConflictDoesNotPartiallyPublish(t *testing.T) {
	s := New()
	a, b := begin(t, s), begin(t, s)
	second := core.StateKey{Contract: "bob", Key: "x"}
	set(t, a, key, []byte("winner"))
	set(t, b, key, []byte("loser"))
	set(t, b, second, []byte("partial"))
	commit(t, a)
	if err := b.Commit(bg); !errors.Is(err, core.ErrStateConflict) {
		t.Fatal(err)
	}
	view := begin(t, s)
	get(t, view, key, "winner", true)
	get(t, view, second, "", false)
	// Read-only snapshots can finish without changing the store revision.
	ro := begin(t, s)
	w := begin(t, s)
	commit(t, ro)
	set(t, w, second, nil)
	commit(t, w)
}

func TestCancellationNeverPublishes(t *testing.T) {
	s := New()
	canceled, cancel := context.WithCancel(bg)
	cancel()
	if _, err := s.Begin(canceled); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	tx := begin(t, s)
	set(t, tx, key, []byte("pending"))
	if err := tx.Commit(canceled); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, _, err := tx.Get(canceled, key); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := tx.Set(canceled, key, nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := tx.Delete(canceled, key); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	get(t, begin(t, s), key, "", false)
	parent, stop := context.WithCancel(bg)
	bound, err := s.Begin(parent)
	if err != nil {
		t.Fatal(err)
	}
	set(t, bound, key, []byte("bound"))
	stop()
	if err := bound.Commit(bg); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, _, err := bound.Get(bg, key); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := bound.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	get(t, begin(t, s), key, "", false)
}

func TestConcurrentCommitHasOneWinner(t *testing.T) {
	s := New()
	const n = 16
	txs := make([]core.StateTransaction, n)
	for i := range txs {
		txs[i] = begin(t, s)
		set(t, txs[i], key, []byte{byte(i)})
	}
	var wg sync.WaitGroup
	outcomes := make(chan error, n)
	for _, tx := range txs {
		wg.Go(func() { outcomes <- tx.Commit(bg) })
	}
	wg.Wait()
	close(outcomes)
	wins := 0
	for err := range outcomes {
		if err == nil {
			wins++
		} else if !errors.Is(err, core.ErrStateConflict) {
			t.Fatal(err)
		}
	}
	if wins != 1 {
		t.Fatalf("winners=%d", wins)
	}
}

func TestConcurrentTransactionAccess(t *testing.T) {
	tx := begin(t, New())
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			for range 100 {
				_ = tx.Set(bg, key, []byte("x"))
				_, _, _ = tx.Get(bg, key)
				_ = tx.ReadSet()
				_ = tx.WriteSet()
				_ = tx.Delete(bg, key)
			}
		})
	}
	wg.Wait()
	commit(t, tx)
}

func TestLastWriteAndClosedRollback(t *testing.T) {
	tx := begin(t, New())
	set(t, tx, key, []byte("first"))
	if err := tx.Delete(bg, key); err != nil {
		t.Fatal(err)
	}
	set(t, tx, key, []byte{})
	writes := tx.WriteSet()
	if len(writes) != 1 || writes[0].Deleted || len(writes[0].Value) != 0 {
		t.Fatalf("writes=%v", writes)
	}
	get(t, tx, key, "", true)
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := tx.Get(bg, key); !errors.Is(err, core.ErrStateClosed) {
		t.Fatal(err)
	}
	if err := tx.Commit(bg); !errors.Is(err, core.ErrStateClosed) {
		t.Fatal(err)
	}
}

func TestCanceledCommitWhileWaitingDoesNotPublish(t *testing.T) {
	s := New()
	tx := begin(t, s)
	set(t, tx, key, []byte("pending"))
	ctx, cancel := context.WithCancel(bg)
	s.mu.Lock()
	done := make(chan error, 1)
	go func() { done <- tx.Commit(ctx) }()
	cancel()
	s.mu.Unlock()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	get(t, begin(t, s), key, "", false)
	// Cancellation of one operation does not invalidate the Begin context.
	commit(t, tx)
	get(t, begin(t, s), key, "pending", true)
}

func TestConcurrentCommitAndRollback(t *testing.T) {
	for range 50 {
		s := New()
		tx := begin(t, s)
		set(t, tx, key, []byte("complete"))
		var wg sync.WaitGroup
		var err error
		wg.Go(func() { err = tx.Commit(bg) })
		wg.Go(func() { _ = tx.Rollback() })
		wg.Wait()
		v, ok, readErr := begin(t, s).Get(bg, key)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if err == nil {
			if !ok || string(v) != "complete" {
				t.Fatal("successful commit lost")
			}
		} else if !errors.Is(err, core.ErrStateClosed) || ok {
			t.Fatalf("commit=%v value=%q exists=%v", err, v, ok)
		}
	}
}

func TestStaleReadOnlyAndDisjointWrites(t *testing.T) {
	s := New()
	ro, stale, winner := begin(t, s), begin(t, s), begin(t, s)
	get(t, ro, key, "", false)
	set(t, winner, key, []byte("new"))
	commit(t, winner)
	get(t, ro, key, "", false)
	commit(t, ro)
	set(t, stale, core.StateKey{Contract: "other", Key: "key"}, nil)
	if err := stale.Commit(bg); !errors.Is(err, core.ErrStateConflict) {
		t.Fatal(err)
	}
}

func TestExpiredDeadline(t *testing.T) {
	ctx, cancel := context.WithDeadline(bg, time.Unix(0, 0))
	defer cancel()
	s := New()
	if _, err := s.Begin(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	tx := begin(t, s)
	set(t, tx, key, []byte("pending"))
	if err := tx.Commit(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	get(t, begin(t, s), key, "", false)
}

func TestSnapshotsNeverSeePartialCommit(t *testing.T) {
	s := New()
	second := core.StateKey{Contract: "alice", Key: "second"}
	var wg sync.WaitGroup
	wg.Go(func() {
		for i := range 100 {
			tx, err := s.Begin(bg)
			if err != nil {
				t.Error(err)
				return
			}
			value := []byte{byte(i)}
			if err := tx.Set(bg, key, value); err != nil {
				t.Error(err)
				return
			}
			if err := tx.Set(bg, second, value); err != nil {
				t.Error(err)
				return
			}
			if err := tx.Commit(bg); err != nil {
				t.Error(err)
				return
			}
		}
	})
	for range 4 {
		wg.Go(func() {
			for range 100 {
				tx, err := s.Begin(bg)
				if err != nil {
					t.Error(err)
					return
				}
				a, aok, ae := tx.Get(bg, key)
				b, bok, be := tx.Get(bg, second)
				_ = tx.Rollback()
				if ae != nil || be != nil || aok != bok || !bytes.Equal(a, b) {
					t.Errorf("partial snapshot: %v/%v %v/%v", a, aok, b, bok)
					return
				}
			}
		})
	}
	wg.Wait()
}
