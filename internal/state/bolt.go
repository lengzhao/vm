package state

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/lengzhao/vm/core"
	"github.com/lengzhao/vm/internal/statefile"
	bolt "go.etcd.io/bbolt"
)

var boltRoot = []byte(statefile.RootBucket)
var boltValues = []byte(statefile.ValuesBucket)
var boltSchema = []byte(statefile.SchemaKey)

type BoltOptions struct {
	Path              string
	LockTimeout       time.Duration
	MaxBatchReceiptMB int
}

// Bolt owns one exclusive database session. Snapshots are in-memory generations,
// not long-lived mmap readers. Constructing does not open files or goroutines.
type Bolt struct {
	mu         sync.Mutex
	options    BoltOptions
	db         *bolt.DB
	memory     *MVCC
	active     map[*boltTransaction]struct{}
	closed     bool
	fault      error
	commitDisk func(*bolt.Tx) error
	session    *boltBatchSession
	pendingID  string
	waiting    int
	changed    chan struct{}
}

var _ core.StateStore = (*Bolt)(nil)
var _ core.SnapshotStore = (*Bolt)(nil)

func NewBolt(o BoltOptions) (*Bolt, error) {
	if o.Path == "" || o.LockTimeout < 0 {
		return nil, fmt.Errorf("state database path and nonnegative lockTimeout required")
	}
	if o.MaxBatchReceiptMB == 0 {
		o.MaxBatchReceiptMB = 64
	}
	if o.MaxBatchReceiptMB < 4 || o.MaxBatchReceiptMB > 1024 {
		return nil, fmt.Errorf("maxBatchReceiptMB must be 4–1024")
	}
	if o.LockTimeout == 0 {
		o.LockTimeout = time.Second
	}
	path, err := filepath.Abs(o.Path)
	if err != nil {
		return nil, err
	}
	o.Path = path
	return &Bolt{options: o, active: make(map[*boltTransaction]struct{}), changed: make(chan struct{}), commitDisk: func(tx *bolt.Tx) error { return tx.Commit() }}, nil
}
func (s *Bolt) Start(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.closed || s.fault != nil {
		return errors.Join(core.ErrStateUnavailable, s.fault)
	}
	if s.db != nil {
		return nil
	}
	// bbolt's file-lock API has no context. Short bounded attempts observe cancellation.
	deadline := time.Now().Add(s.options.LockTimeout)
	var db *bolt.DB
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return fmt.Errorf("open state database: lock timeout")
		}
		var err error
		db, err = bolt.Open(s.options.Path, 0600, &bolt.Options{Timeout: min(remaining, 50*time.Millisecond)})
		if err == nil {
			break
		}
		if !errors.Is(err, bolt.ErrTimeout) {
			return err
		}
	}
	fail := func(err error) error { return errors.Join(err, db.Close()) }
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	memory := NewMVCC()
	var pending string
	err := db.Update(func(tx *bolt.Tx) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		root := tx.Bucket(boltRoot)
		if root == nil {
			if key, _ := tx.Cursor().First(); key != nil {
				return fmt.Errorf("unrecognized state database schema")
			}
			var err error
			root, err = tx.CreateBucket(boltRoot)
			if err != nil {
				return err
			}
			if err = root.Put(boltSchema, []byte{1}); err != nil {
				return err
			}
			if _, err = root.CreateBucket(boltValues); err != nil {
				return err
			}
		}
		if err := statefile.ValidateSchema(tx); err != nil {
			return err
		}
		pending = string(root.Get(boltPending))
		if pending != "" {
			batches := root.Bucket(boltBatches)
			if batches == nil || batches.Bucket([]byte(pending)) == nil {
				return fmt.Errorf("missing pending batch metadata")
			}
			var header batchHeader
			if err := decodeBatchJSON(batches.Bucket([]byte(pending)).Get(boltBatchHeader), &header); err != nil {
				return err
			}
			if !validBatchIdentity(header.Identity) || header.Identity.ID != pending || header.Identity.Count == 0 {
				return fmt.Errorf("invalid pending batch metadata")
			}
		}
		return root.Bucket(boltValues).ForEach(func(k, v []byte) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if v == nil {
				return fmt.Errorf("unexpected nested state bucket")
			}
			key, err := decodeStateKey(k)
			if err != nil {
				return err
			}
			memory.values.set(key, bytes.Clone(v))
			memory.versions.set(key, &revision{})
			return nil
		})
	})
	if err != nil {
		return fail(err)
	}
	// The DB's sync covers its contents; sync the parent for its initial directory entry.
	dir, err := os.Open(filepath.Dir(s.options.Path))
	if err != nil {
		return fail(err)
	}
	err = errors.Join(dir.Sync(), dir.Close())
	if err != nil {
		return fail(err)
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	s.db, s.memory = db, memory
	s.pendingID = pending
	return nil
}
func (s *Bolt) Stop(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.closed {
		return nil
	}
	s.closed = true
	for tx := range s.active {
		tx.cancel()
	}
	clear(s.active)
	if s.session != nil {
		s.session.cancel()
	}
	s.notify()
	if s.db == nil {
		return nil
	}
	err := s.db.Close()
	s.db = nil
	s.memory = nil
	return err
}
func (s *Bolt) available() error {
	if s.db == nil || s.closed || s.fault != nil {
		return errors.Join(core.ErrStateUnavailable, s.fault)
	}
	return nil
}
func (s *Bolt) Begin(ctx context.Context) (core.StateTransaction, error) { return s.BeginSnapshot(ctx) }
func (s *Bolt) BeginSnapshot(ctx context.Context) (core.SnapshotTransaction, error) {
	return s.beginSnapshot(ctx, nil)
}
func (s *Bolt) notify() { close(s.changed); s.changed = make(chan struct{}) }
func (s *Bolt) beginSnapshot(ctx context.Context, session *boltBatchSession) (core.SnapshotTransaction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := s.available(); err != nil {
			return nil, err
		}
		if session != nil {
			if session != s.session {
				return nil, core.ErrStateClosed
			}
			if err := session.ctx.Err(); err != nil {
				return nil, err
			}
			break
		}
		if s.session == nil && s.pendingID != "" {
			return nil, fmt.Errorf("%w: %s", core.ErrBatchPending, s.pendingID)
		}
		if s.session == nil && s.waiting == 0 && s.pendingID == "" {
			break
		}
		changed := s.changed
		s.mu.Unlock()
		select {
		case <-ctx.Done():
		case <-changed:
		}
		s.mu.Lock()
	}
	parent, cancel := context.WithCancel(ctx)
	if session != nil {
		stop := context.AfterFunc(session.ctx, cancel)
		baseCancel := cancel
		cancel = func() { stop(); baseCancel() }
	}
	tx, err := s.memory.BeginSnapshot(parent)
	if err != nil {
		cancel()
		return nil, err
	}
	out := &boltTransaction{mvccTransaction: tx.(*mvccTransaction), owner: s, cancel: cancel, session: session}
	s.active[out] = struct{}{}
	return out, nil
}

type boltTransaction struct {
	*mvccTransaction
	owner   *Bolt
	cancel  context.CancelFunc
	session *boltBatchSession
}

func (t *boltTransaction) Validate(ctx context.Context) error {
	t.owner.mu.Lock()
	defer t.owner.mu.Unlock()
	if err := t.owner.available(); err != nil {
		return err
	}
	return t.mvccTransaction.Validate(ctx)
}
func (t *boltTransaction) Commit(ctx context.Context) error {
	if t.session != nil {
		return fmt.Errorf("session transactions require Finalize with a receipt")
	}
	s := t.owner
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.available(); err != nil {
		return err
	}
	err := t.mvccTransaction.commit(ctx, func(writes map[core.StateKey]core.StateWrite) error {
		return s.persist(writes, func() error { return t.check(ctx) }, nil)
	})
	if err == nil {
		delete(s.active, t)
		t.cancel()
		s.notify()
	}
	return err
}
func (t *boltTransaction) Rollback() error {
	t.owner.mu.Lock()
	defer t.owner.mu.Unlock()
	err := t.mvccTransaction.Rollback()
	delete(t.owner.active, t)
	t.cancel()
	t.owner.notify()
	return err
}
func encodeStateKey(k core.StateKey) []byte {
	out := make([]byte, 8+len(k.Contract)+len(k.Key))
	binary.BigEndian.PutUint64(out, uint64(len(k.Contract)))
	copy(out[8:], k.Contract)
	copy(out[8+len(k.Contract):], k.Key)
	return out
}
func decodeStateKey(k []byte) (core.StateKey, error) {
	if len(k) < 8 {
		return core.StateKey{}, fmt.Errorf("invalid state key encoding")
	}
	n := binary.BigEndian.Uint64(k[:8])
	if n > uint64(len(k)-8) {
		return core.StateKey{}, fmt.Errorf("invalid state key length")
	}
	return core.StateKey{Contract: core.Address(string(k[8 : 8+int(n)])), Key: string(k[8+int(n):])}, nil
}
