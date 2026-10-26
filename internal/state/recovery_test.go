package state

import (
	"context"
	"errors"
	"github.com/lengzhao/vm/core"
	bolt "go.etcd.io/bbolt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func batchIdentity(id string, n int) core.BatchIdentity {
	return core.BatchIdentity{ID: id, Fingerprint: strings.Repeat("a", 64), Count: n}
}
func acquire(t *testing.T, s *Bolt, id string, n int) core.BatchSession {
	t.Helper()
	session, e := s.AcquireBatch(context.Background(), batchIdentity(id, n))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { session.Close() })
	return session
}
func sessionSnap(t *testing.T, s core.BatchSession) core.SnapshotTransaction {
	t.Helper()
	tx, e := s.BeginSnapshot(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { tx.Rollback() })
	return tx
}
func TestBatchSessionBlocksNormalTransactionsAndWaitsForOldOnes(t *testing.T) {
	ctx := context.Background()
	s := openBolt(t, filepath.Join(t.TempDir(), "state.db"))
	old := boltSnap(t, s)
	limited, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	if _, e := s.AcquireBatch(limited, batchIdentity("b", 0)); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal(e)
	}
	old.Rollback()
	b := acquire(t, s, "b", 0)
	limited, cancel2 := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel2()
	if _, e := s.Begin(limited); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal("normal Begin bypassed session", e)
	}
	limited, cancel3 := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel3()
	if _, e := s.AcquireBatch(limited, batchIdentity("other", 1)); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal(e)
	}
	tx := sessionSnap(t, b)
	if e := tx.Commit(ctx); e == nil {
		t.Fatal("plain Commit bypassed receipt")
	}
	b.Close()
	if _, _, e := tx.Get(ctx, core.StateKey{}); !errors.Is(e, core.ErrStateClosed) {
		t.Fatal("closed session left transaction active", e)
	}
	tx = boltSnap(t, s)
	put(t, tx, core.StateKey{Key: "normal"}, "ok")
	mvccCommit(t, tx)
}
func TestBatchReceiptAtomicStateAndIdentityReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	s := openBolt(t, path)
	b := acquire(t, s, "b", 3)
	tx := sessionSnap(t, b)
	key := core.StateKey{Key: "x"}
	put(t, tx, key, "yes")
	if e := b.Finalize(ctx, tx, core.BatchReceipt{Index: 0, Result: &core.ExecuteResult{Data: []byte(`1`), GasConsumed: 9}}); e != nil {
		t.Fatal(e)
	}
	tx = sessionSnap(t, b)
	put(t, tx, key, "bad")
	if e := b.Finalize(ctx, tx, core.BatchReceipt{Index: 1, Result: &core.ExecuteResult{GasConsumed: 7}, Failure: &core.BatchFailure{Code: core.CodeContractError, Message: "business"}}); e != nil {
		t.Fatal(e)
	}
	tx = sessionSnap(t, b)
	if e := b.Finalize(ctx, tx, core.BatchReceipt{Index: 2, Result: &core.ExecuteResult{Data: []byte(`2`)}}); e != nil {
		t.Fatal("read-only receipt", e)
	}
	prefix, e := b.Prefix(ctx)
	if e != nil || len(prefix) != 3 {
		t.Fatal(prefix, e)
	}
	prefix[0].Result.Data[0] = '!'
	b.Close()
	s.Stop(ctx)
	s = openBolt(t, path)
	b = acquire(t, s, "b", 3)
	prefix, e = b.Prefix(ctx)
	if e != nil || len(prefix) != 3 || string(prefix[0].Result.Data) != "1" || prefix[1].Failure == nil {
		t.Fatal(prefix, e)
	}
	b.Close()
	value, _, _ := boltSnap(t, s).Get(ctx, key)
	if string(value) != "yes" {
		t.Fatal("failure writes published", string(value))
	}
	// Release that normal snapshot before acquisition.
	for tx := range s.active {
		tx.Rollback()
	}
	changed := batchIdentity("b", 3)
	changed.Fingerprint = strings.Repeat("b", 64)
	if _, e := s.AcquireBatch(ctx, changed); !errors.Is(e, core.ErrBatchIdentityConflict) {
		t.Fatal(e)
	}
}

func TestIncompleteBatchFencesWritersAcrossRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	s := openBolt(t, path)
	b := acquire(t, s, "pending", 1)
	b.Close()
	s.Stop(ctx)
	s = openBolt(t, path)
	limited, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	if _, e := s.Begin(limited); !errors.Is(e, core.ErrBatchPending) {
		t.Fatal("restart lost pending fence", e)
	}
	if _, e := s.AcquireBatch(ctx, batchIdentity("different", 1)); !errors.Is(e, core.ErrBatchPending) {
		t.Fatal(e)
	}
	b = acquire(t, s, "pending", 1)
	tx := sessionSnap(t, b)
	if e := b.Finalize(ctx, tx, core.BatchReceipt{Index: 0, Result: &core.ExecuteResult{}}); e != nil {
		t.Fatal(e)
	}
	b.Close()
	tx = boltSnap(t, s)
	put(t, tx, core.StateKey{Key: "released"}, "yes")
	mvccCommit(t, tx)
}

func TestBatchReceiptUncertainCommitReopensWithoutDuplicates(t *testing.T) {
	for _, publish := range []bool{false, true} {
		t.Run(map[bool]string{false: "before", true: "after"}[publish], func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "state.db")
			s := openBolt(t, path)
			b := acquire(t, s, "fault", 1)
			tx := sessionSnap(t, b)
			key := core.StateKey{Key: "x"}
			put(t, tx, key, "once")
			s.commitDisk = func(disk *bolt.Tx) error {
				if publish {
					if e := disk.Commit(); e != nil {
						return e
					}
				}
				return errors.New("injected commit error")
			}
			if e := b.Finalize(ctx, tx, core.BatchReceipt{Index: 0, Result: &core.ExecuteResult{Data: []byte(`1`)}}); !errors.Is(e, core.ErrStateCommitUncertain) {
				t.Fatal(e)
			}
			b.Close()
			s.Stop(ctx)
			s = openBolt(t, path)
			b = acquire(t, s, "fault", 1)
			prefix, e := b.Prefix(ctx)
			want := 0
			if publish {
				want = 1
			}
			if e != nil || len(prefix) != want {
				t.Fatal(prefix, e)
			}
			read := sessionSnap(t, b)
			v, ok, e := read.Get(ctx, key)
			read.Rollback()
			if e != nil || ok != publish || (publish && string(v) != "once") {
				t.Fatal(v, ok, e)
			}
			if !publish {
				tx = sessionSnap(t, b)
				put(t, tx, key, "once")
				if e := b.Finalize(ctx, tx, core.BatchReceipt{Index: 0, Result: &core.ExecuteResult{Data: []byte(`1`)}}); e != nil {
					t.Fatal(e)
				}
			}
		})
	}
}
func TestBatchFailureReceiptValidatesReadsAndLimits(t *testing.T) {
	ctx := context.Background()
	s := openBolt(t, filepath.Join(t.TempDir(), "state.db"))
	b := acquire(t, s, "conflict", 2)
	key := core.StateKey{Key: "x"}
	first, stale := sessionSnap(t, b), sessionSnap(t, b)
	stale.Get(ctx, key)
	put(t, first, key, "v")
	if e := b.Finalize(ctx, first, core.BatchReceipt{Index: 0, Result: &core.ExecuteResult{}}); e != nil {
		t.Fatal(e)
	}
	failure := core.BatchReceipt{Index: 1, Failure: &core.BatchFailure{Code: core.CodeContractError, Message: "old observation"}}
	if e := b.Finalize(ctx, stale, failure); !errors.Is(e, core.ErrStateConflict) {
		t.Fatal(e)
	}
	stale.Rollback()
	fresh := sessionSnap(t, b)
	put(t, fresh, key, "must not publish")
	if e := b.Finalize(ctx, fresh, core.BatchReceipt{Index: 1, Result: &core.ExecuteResult{Data: make([]byte, 4<<20)}}); e == nil {
		t.Fatal("receipt size ignored")
	}
	prefix, e := b.Prefix(ctx)
	if e != nil || len(prefix) != 1 {
		t.Fatal(prefix, e)
	}
	if e := b.Finalize(ctx, fresh, failure); e != nil {
		t.Fatal(e)
	}
	read := sessionSnap(t, b)
	v, _, _ := read.Get(ctx, key)
	if string(v) != "v" {
		t.Fatal("failure writes committed", string(v))
	}
}
func TestRecoverableBatchAbruptExitRestoresFenceAndPrefix(t *testing.T) {
	if path := os.Getenv("VM_RECOVERY_CHILD_PATH"); path != "" {
		s, e := NewBolt(BoltOptions{Path: path})
		if e != nil {
			panic(e)
		}
		if e = s.Start(context.Background()); e != nil {
			panic(e)
		}
		b, e := s.AcquireBatch(context.Background(), batchIdentity("crash", 2))
		if e != nil {
			panic(e)
		}
		tx, e := b.BeginSnapshot(context.Background())
		if e != nil {
			panic(e)
		}
		tx.Set(context.Background(), core.StateKey{Key: "first"}, []byte("yes"))
		if e = b.Finalize(context.Background(), tx, core.BatchReceipt{Index: 0, Result: &core.ExecuteResult{Data: []byte(`1`)}}); e != nil {
			panic(e)
		}
		tx, e = b.BeginSnapshot(context.Background())
		if e != nil {
			panic(e)
		}
		tx.Set(context.Background(), core.StateKey{Key: "uncommitted"}, []byte("no"))
		os.Exit(0)
	}
	path := filepath.Join(t.TempDir(), "state.db")
	cmd := exec.Command(os.Args[0], "-test.run=^TestRecoverableBatchAbruptExitRestoresFenceAndPrefix$")
	cmd.Env = append(os.Environ(), "VM_RECOVERY_CHILD_PATH="+path)
	if v, e := cmd.CombinedOutput(); e != nil {
		t.Fatal(e, string(v))
	}
	ctx := context.Background()
	s := openBolt(t, path)
	limited, cancel := context.WithTimeout(ctx, 30*time.Millisecond)
	defer cancel()
	if _, e := s.Begin(limited); !errors.Is(e, core.ErrBatchPending) {
		t.Fatal(e)
	}
	b := acquire(t, s, "crash", 2)
	prefix, e := b.Prefix(ctx)
	if e != nil || len(prefix) != 1 {
		t.Fatal(prefix, e)
	}
	tx := sessionSnap(t, b)
	if _, ok, _ := tx.Get(ctx, core.StateKey{Key: "uncommitted"}); ok {
		t.Fatal("uncommitted survived crash")
	}
	v, _, _ := tx.Get(ctx, core.StateKey{Key: "first"})
	if string(v) != "yes" {
		t.Fatal(string(v))
	}
	if e := b.Finalize(ctx, tx, core.BatchReceipt{Index: 1, Result: &core.ExecuteResult{}}); e != nil {
		t.Fatal(e)
	}
}

func TestPendingBatchReportsRecoveryRequirement(t *testing.T) {
	s := openBolt(t, filepath.Join(t.TempDir(), "state.db"))
	b := acquire(t, s, "pending-report", 1)
	b.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, e := s.Begin(ctx); !errors.Is(e, core.ErrBatchPending) {
		t.Fatal("pending batch should be reported without waiting", e)
	}
}

func TestBatchReceiptBudgetCanBeRaisedForRecovery(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	s, e := NewBolt(BoltOptions{Path: path, MaxBatchReceiptMB: 4})
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Start(ctx); e != nil {
		t.Fatal(e)
	}
	defer s.Stop(ctx)
	b := acquire(t, s, "budget", 2)
	r := core.BatchReceipt{Index: 0, Result: &core.ExecuteResult{Data: make([]byte, 2<<20)}}
	if e = b.Finalize(ctx, sessionSnap(t, b), r); e != nil {
		t.Fatal(e)
	}
	r.Index = 1
	if e = b.Finalize(ctx, sessionSnap(t, b), r); e == nil {
		t.Fatal("budget ignored")
	}
	b.Close()
	s.Stop(ctx)
	s, e = NewBolt(BoltOptions{Path: path, MaxBatchReceiptMB: 8})
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Start(ctx); e != nil {
		t.Fatal(e)
	}
	defer s.Stop(ctx)
	b = acquire(t, s, "budget", 2)
	prefix, e := b.Prefix(ctx)
	if e != nil || len(prefix) != 1 {
		t.Fatal(e)
	}
	if e = b.Finalize(ctx, sessionSnap(t, b), r); e != nil {
		t.Fatal("raising quota did not allow recovery", e)
	}
}
func TestBatchLargeBusinessDiagnosticRemainsRecoverable(t *testing.T) {
	s := openBolt(t, filepath.Join(t.TempDir(), "state.db"))
	b := acquire(t, s, "diagnostic", 1)
	if e := b.Finalize(context.Background(), sessionSnap(t, b), core.BatchReceipt{Index: 0, Failure: &core.BatchFailure{Code: core.CodeContractError, Message: strings.Repeat("x", 20<<10)}}); e != nil {
		t.Fatal(e)
	}
}
