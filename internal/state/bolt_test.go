package state

import (
	"bytes"
	"context"
	"errors"
	bolt "go.etcd.io/bbolt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lengzhao/vm/core"
)

func openBolt(t *testing.T, path string) *Bolt {
	t.Helper()
	s, e := NewBolt(BoltOptions{Path: path})
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Start(context.Background()); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := s.Stop(context.Background()); e != nil {
			t.Error(e)
		}
	})
	return s
}
func boltSnap(t *testing.T, s *Bolt) core.SnapshotTransaction {
	t.Helper()
	tx, e := s.BeginSnapshot(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { tx.Rollback() })
	return tx
}
func TestBoltLifecycleHasNoConstructorIO(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	s, e := NewBolt(BoltOptions{Path: path})
	if e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(path); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("constructor accessed disk", e)
	}
	if _, e := s.Begin(ctx); !errors.Is(e, core.ErrStateUnavailable) {
		t.Fatal(e)
	}
	if e := s.Start(ctx); e != nil {
		t.Fatal(e)
	}
	if e := s.Start(ctx); e != nil {
		t.Fatal(e)
	}
	tx := boltSnap(t, s)
	if e := s.Stop(ctx); e != nil {
		t.Fatal(e)
	}
	if e := s.Stop(ctx); e != nil {
		t.Fatal(e)
	}
	if _, _, e := tx.Get(ctx, core.StateKey{}); !errors.Is(e, context.Canceled) {
		t.Fatal("old transaction survived close", e)
	}
	if e := s.Start(ctx); !errors.Is(e, core.ErrStateUnavailable) {
		t.Fatal("closed instance restarted", e)
	}
}
func TestBoltReopenBinaryKeysEmptyValuesAndDeletes(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	s := openBolt(t, path)
	keys := []core.StateKey{{}, {Contract: "a\x00b", Key: "x\xff"}, {Contract: "a", Key: "\x00bx\xff"}}
	tx := boltSnap(t, s)
	for i, k := range keys {
		put(t, tx, k, string([]byte{byte(i)}))
	}
	tx.Set(ctx, keys[0], nil)
	mvccCommit(t, tx)
	tx = boltSnap(t, s)
	tx.Delete(ctx, keys[2])
	mvccCommit(t, tx)
	if e := s.Stop(ctx); e != nil {
		t.Fatal(e)
	}
	s = openBolt(t, path)
	tx = boltSnap(t, s)
	for i, k := range keys {
		v, ok, e := tx.Get(ctx, k)
		if e != nil || ok != (i != 2) {
			t.Fatal(i, v, ok, e)
		}
		if i == 0 && len(v) != 0 {
			t.Fatal("empty value lost")
		}
		if i == 1 && string(v) != "\x01" {
			t.Fatal(v)
		}
	}
}
func TestBoltMVCCConflictsABAAndCanceledCommit(t *testing.T) {
	ctx := context.Background()
	s := openBolt(t, filepath.Join(t.TempDir(), "state.db"))
	k := core.StateKey{Key: "x"}
	a, b := boltSnap(t, s), boltSnap(t, s)
	a.Get(ctx, k)
	put(t, b, k, "x")
	mvccCommit(t, b)
	b = boltSnap(t, s)
	b.Delete(ctx, k)
	mvccCommit(t, b)
	if e := a.Validate(ctx); !errors.Is(e, core.ErrStateConflict) {
		t.Fatal(e)
	}
	a, b = boltSnap(t, s), boltSnap(t, s)
	put(t, a, k, "a")
	put(t, b, core.StateKey{Key: "y"}, "b")
	mvccCommit(t, a)
	mvccCommit(t, b)
	a, b = boltSnap(t, s), boltSnap(t, s)
	put(t, a, k, "a")
	put(t, b, k, "b")
	mvccCommit(t, a)
	if e := b.Commit(ctx); !errors.Is(e, core.ErrStateConflict) {
		t.Fatal(e)
	}
	cancelCtx, cancel := context.WithCancel(ctx)
	a, _ = s.BeginSnapshot(cancelCtx)
	put(t, a, k, "canceled")
	cancel()
	if e := a.Commit(ctx); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	a.Rollback()
}
func TestBoltExclusiveLockAndCanceledStart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	s := openBolt(t, path)
	other, e := NewBolt(BoltOptions{Path: path, LockTimeout: 100 * time.Millisecond})
	if e != nil {
		t.Fatal(e)
	}
	if e := other.Start(ctx); e == nil {
		t.Fatal("second writer accepted")
	}
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()
	if e := other.Start(cancelCtx); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if e := s.Stop(ctx); e != nil {
		t.Fatal(e)
	}
	if e := other.Start(ctx); e != nil {
		t.Fatal(e)
	}
	other.Stop(ctx)
}
func TestBoltRejectsInvalidOptionsAndCorruptFile(t *testing.T) {
	for _, o := range []BoltOptions{{}, {Path: "x", LockTimeout: -1}} {
		if _, e := NewBolt(o); e == nil {
			t.Fatal(o)
		}
	}
	path := filepath.Join(t.TempDir(), "bad.db")
	if e := os.WriteFile(path, []byte("not a database"), 0600); e != nil {
		t.Fatal(e)
	}
	s, e := NewBolt(BoltOptions{Path: path})
	if e != nil {
		t.Fatal(e)
	}
	if e := s.Start(context.Background()); e == nil {
		t.Fatal("corruption accepted")
	}
	v, _ := os.ReadFile(path)
	if string(v) != "not a database" {
		t.Fatal("corrupt file overwritten")
	}
}

func TestBoltCommitFailureIsUncertainAndQuarantines(t *testing.T) {
	for _, publish := range []bool{false, true} {
		t.Run(map[bool]string{false: "before", true: "after"}[publish], func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "state.db")
			s := openBolt(t, path)
			tx := boltSnap(t, s)
			key := core.StateKey{Key: "x"}
			put(t, tx, key, "value")
			boom := errors.New("injected sync failure")
			s.commitDisk = func(disk *bolt.Tx) error {
				if publish {
					if e := disk.Commit(); e != nil {
						t.Fatal(e)
					}
				}
				return boom
			}
			if e := tx.Commit(ctx); !errors.Is(e, core.ErrStateCommitUncertain) || !errors.Is(e, boom) {
				t.Fatal(e)
			}
			if _, e := s.Begin(ctx); !errors.Is(e, core.ErrStateUnavailable) {
				t.Fatal("faulted backend accepted work", e)
			}
			if _, _, e := tx.Get(ctx, key); !errors.Is(e, context.Canceled) {
				t.Fatal("faulted transaction usable", e)
			}
			tx.Rollback()
			s.Stop(ctx)
			fresh := openBolt(t, path)
			v, ok, e := boltSnap(t, fresh).Get(ctx, key)
			if e != nil || ok != publish || (publish && string(v) != "value") {
				t.Fatal(v, ok, e)
			}
		})
	}
}
func TestBoltPrecommitFailureDoesNotPublishOrPoison(t *testing.T) {
	ctx := context.Background()
	s := openBolt(t, filepath.Join(t.TempDir(), "state.db"))
	tx := boltSnap(t, s)
	put(t, tx, core.StateKey{Key: "good"}, "no")
	put(t, tx, core.StateKey{Key: strings.Repeat("x", 40000)}, "too large")
	if e := tx.Commit(ctx); e == nil || errors.Is(e, core.ErrStateCommitUncertain) {
		t.Fatal(e)
	}
	tx.Rollback()
	if _, ok, e := boltSnap(t, s).Get(ctx, core.StateKey{Key: "good"}); e != nil || ok {
		t.Fatal("partial commit", ok, e)
	}
	tx = boltSnap(t, s)
	put(t, tx, core.StateKey{Key: "next"}, "yes")
	mvccCommit(t, tx)
}
func TestBoltRejectsUnknownSchemaAndReleasesLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	db, e := bolt.Open(path, 0600, nil)
	if e != nil {
		t.Fatal(e)
	}
	if e = db.Update(func(tx *bolt.Tx) error {
		r, e := tx.CreateBucket(boltRoot)
		if e != nil {
			return e
		}
		return r.Put(boltSchema, []byte{99})
	}); e != nil {
		t.Fatal(e)
	}
	db.Close()
	s, _ := NewBolt(BoltOptions{Path: path})
	if e := s.Start(context.Background()); e == nil {
		t.Fatal("unknown schema accepted")
	}
	db, e = bolt.Open(path, 0600, &bolt.Options{Timeout: time.Second})
	if e != nil {
		t.Fatal("failed Start leaked lock", e)
	}
	defer db.Close()
	db.View(func(tx *bolt.Tx) error {
		if !bytes.Equal(tx.Bucket(boltRoot).Get(boltSchema), []byte{99}) {
			t.Fatal("schema overwritten")
		}
		return nil
	})
}
func TestBoltAbruptProcessRecovery(t *testing.T) {
	if path := os.Getenv("VM_BOLT_CHILD_PATH"); path != "" {
		s, e := NewBolt(BoltOptions{Path: path})
		if e != nil {
			panic(e)
		}
		if e = s.Start(context.Background()); e != nil {
			panic(e)
		}
		tx, e := s.Begin(context.Background())
		if e != nil {
			panic(e)
		}
		if e = tx.Set(context.Background(), core.StateKey{Key: "committed"}, []byte("yes")); e != nil {
			panic(e)
		}
		if e = tx.Commit(context.Background()); e != nil {
			panic(e)
		}
		tx, e = s.Begin(context.Background())
		if e != nil {
			panic(e)
		}
		tx.Set(context.Background(), core.StateKey{Key: "uncommitted"}, []byte("no"))
		os.Exit(0) // Deliberately omit Stop, Rollback, and Go defers.
	}
	path := filepath.Join(t.TempDir(), "state.db")
	cmd := exec.Command(os.Args[0], "-test.run=^TestBoltAbruptProcessRecovery$")
	cmd.Env = append(os.Environ(), "VM_BOLT_CHILD_PATH="+path)
	if output, e := cmd.CombinedOutput(); e != nil {
		t.Fatal(e, string(output))
	}
	s := openBolt(t, path)
	tx := boltSnap(t, s)
	v, ok, e := tx.Get(context.Background(), core.StateKey{Key: "committed"})
	if e != nil || !ok || string(v) != "yes" {
		t.Fatal(v, ok, e)
	}
	if _, ok, e := tx.Get(context.Background(), core.StateKey{Key: "uncommitted"}); e != nil || ok {
		t.Fatal(ok, e)
	}
}

func TestBoltCancellationAfterDurableBoundaryReportsSuccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := openBolt(t, filepath.Join(t.TempDir(), "state.db"))
	tx, e := s.BeginSnapshot(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback()
	key := core.StateKey{Key: "x"}
	put(t, tx, key, "committed")
	s.commitDisk = func(disk *bolt.Tx) error { err := disk.Commit(); cancel(); return err }
	if e := tx.Commit(ctx); e != nil {
		t.Fatal("durable commit reported cancellation", e)
	}
	value, ok, e := boltSnap(t, s).Get(context.Background(), key)
	if e != nil || !ok || string(value) != "committed" {
		t.Fatal(value, ok, e)
	}
}
