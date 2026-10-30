package state

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/lengzhao/vm/core"
)

type failedBackupWriter struct{}

func (failedBackupWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

type cancelBackupWriter struct{ cancel context.CancelFunc }

func (w cancelBackupWriter) Write(p []byte) (int, error) { w.cancel(); return len(p), nil }

func TestBoltBackupRestoresCommittedStateAndPendingReceipts(t *testing.T) {
	ctx := context.Background()
	s := openBolt(t, filepath.Join(t.TempDir(), "live.db"))
	tx := boltSnap(t, s)
	put(t, tx, core.StateKey{Contract: "a", Key: "k"}, "committed")
	mvccCommit(t, tx)
	id := core.BatchIdentity{ID: "pending", Fingerprint: strings.Repeat("a", 64), Count: 2}
	session, e := s.AcquireBatch(ctx, id)
	if e != nil {
		t.Fatal(e)
	}
	st, e := session.BeginSnapshot(ctx)
	if e != nil {
		t.Fatal(e)
	}
	put(t, st, core.StateKey{Contract: "a", Key: "k"}, "receipt-state")
	if e = session.Finalize(ctx, st, core.BatchReceipt{Index: 0, Result: &core.ExecuteResult{Data: []byte(`1`), GasConsumed: 11}}); e != nil {
		t.Fatal(e)
	}
	if e = session.Close(); e != nil {
		t.Fatal(e)
	}
	var buf bytes.Buffer
	if e = s.Backup(ctx, &buf); e != nil {
		t.Fatal(e)
	}
	stats, e := s.StateStats(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if stats.StateKeys != 1 || stats.StateValueBytes != 13 || stats.Batches != 1 || stats.Receipts != 1 || stats.ReceiptBytes == 0 || stats.PendingBatchID != "pending" || stats.DatabaseBytes == 0 {
		t.Fatalf("stats: %+v", stats)
	}
	path := filepath.Join(t.TempDir(), "restored.db")
	if e = os.WriteFile(path, buf.Bytes(), 0600); e != nil {
		t.Fatal(e)
	}
	restored := openBolt(t, path)
	if _, e = restored.Begin(ctx); !errors.Is(e, core.ErrBatchPending) {
		t.Fatalf("pending lost: %v", e)
	}
	resumed, e := restored.AcquireBatch(ctx, id)
	if e != nil {
		t.Fatal(e)
	}
	defer resumed.Close()
	prefix, e := resumed.Prefix(ctx)
	if e != nil || len(prefix) != 1 || string(prefix[0].Result.Data) != "1" {
		t.Fatalf("prefix=%v err=%v", prefix, e)
	}
	rt, e := resumed.BeginSnapshot(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer rt.Rollback()
	v, ok, e := rt.Get(ctx, core.StateKey{Contract: "a", Key: "k"})
	if e != nil || !ok || string(v) != "receipt-state" {
		t.Fatalf("state=%q exists=%v err=%v", v, ok, e)
	}
}
func TestBoltBackupExcludesUncommittedWritesAndHandlesErrors(t *testing.T) {
	ctx := context.Background()
	s := openBolt(t, filepath.Join(t.TempDir(), "live.db"))
	tx := boltSnap(t, s)
	put(t, tx, core.StateKey{Key: "not-committed"}, "hidden")
	var b bytes.Buffer
	if e := s.Backup(ctx, &b); e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(t.TempDir(), "copy.db")
	if e := os.WriteFile(path, b.Bytes(), 0600); e != nil {
		t.Fatal(e)
	}
	restored := openBolt(t, path)
	r := boltSnap(t, restored)
	if _, ok, e := r.Get(ctx, core.StateKey{Key: "not-committed"}); e != nil || ok {
		t.Fatal("uncommitted write backed up", e)
	}
	if e := s.Backup(ctx, failedBackupWriter{}); !errors.Is(e, io.ErrClosedPipe) {
		t.Fatal(e)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	b.Reset()
	if e := s.Backup(canceled, &b); !errors.Is(e, context.Canceled) || b.Len() != 0 {
		t.Fatal("canceled backup wrote", e)
	}
	during, stop := context.WithCancel(ctx)
	defer stop()
	if e := s.Backup(during, cancelBackupWriter{stop}); !errors.Is(e, context.Canceled) {
		t.Fatal("mid-copy cancellation lost", e)
	}
	if e := s.Stop(ctx); e != nil {
		t.Fatal(e)
	}
	if e := s.Backup(ctx, &b); !errors.Is(e, core.ErrStateUnavailable) {
		t.Fatal(e)
	}
	if _, e := s.StateStats(ctx); !errors.Is(e, core.ErrStateUnavailable) {
		t.Fatal(e)
	}
}

type shortBackupWriter struct{}

func (shortBackupWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }
func TestBoltBackupPreservesShortWriteAndCanceledStats(t *testing.T) {
	s := openBolt(t, filepath.Join(t.TempDir(), "live.db"))
	if e := s.Backup(context.Background(), shortBackupWriter{}); !errors.Is(e, io.ErrShortWrite) {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := s.StateStats(ctx); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}

type pausedSnapshotWriter struct {
	bytes.Buffer
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (w *pausedSnapshotWriter) Write(p []byte) (int, error) {
	w.once.Do(func() { close(w.entered); <-w.release })
	return w.Buffer.Write(p)
}
func TestBoltBackupSerializesConcurrentCommit(t *testing.T) {
	ctx := context.Background()
	s := openBolt(t, filepath.Join(t.TempDir(), "live.db"))
	key := core.StateKey{Key: "k"}
	initial := boltSnap(t, s)
	put(t, initial, key, "before")
	mvccCommit(t, initial)
	next := boltSnap(t, s)
	put(t, next, key, "after")
	w := &pausedSnapshotWriter{entered: make(chan struct{}), release: make(chan struct{})}
	var release sync.Once
	defer release.Do(func() { close(w.release) })
	copied := make(chan error, 1)
	go func() { copied <- s.Backup(ctx, w) }()
	<-w.entered
	committed := make(chan error, 1)
	go func() { committed <- next.Commit(ctx) }()
	release.Do(func() { close(w.release) })
	if e := <-copied; e != nil {
		t.Fatal(e)
	}
	if e := <-committed; e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(t.TempDir(), "copy.db")
	if e := os.WriteFile(path, w.Bytes(), 0600); e != nil {
		t.Fatal(e)
	}
	restored := openBolt(t, path)
	read := boltSnap(t, restored)
	v, ok, e := read.Get(ctx, key)
	if e != nil || !ok || string(v) != "before" {
		t.Fatal(string(v), ok, e)
	}
	live := boltSnap(t, s)
	v, ok, e = live.Get(ctx, key)
	if e != nil || !ok || string(v) != "after" {
		t.Fatal(string(v), ok, e)
	}
}
