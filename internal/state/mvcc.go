package state

import (
	"context"
	"sync"

	"github.com/lengzhao/vm/core"
)

// MVCC keeps immutable generations and per-key identity tokens. Tombstones retain
// a version even for absent keys, so delete/recreate and missing-key ABA conflict.
// Like Memory, it is in-process storage; it does not persist or reclaim key history.
type MVCC struct {
	mu       sync.Mutex
	values   map[core.StateKey][]byte
	versions map[core.StateKey]*revision
}

var _ core.StateStore = (*MVCC)(nil)
var _ core.SnapshotStore = (*MVCC)(nil)

func NewMVCC() *MVCC {
	return &MVCC{values: make(map[core.StateKey][]byte), versions: make(map[core.StateKey]*revision)}
}
func (s *MVCC) Begin(ctx context.Context) (core.StateTransaction, error) { return s.BeginSnapshot(ctx) }
func (s *MVCC) BeginSnapshot(ctx context.Context) (core.SnapshotTransaction, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &mvccTransaction{transaction: &transaction{parent: ctx, snapshot: s.values, reads: make(map[core.StateKey]struct{}), writes: make(map[core.StateKey]core.StateWrite)}, owner: s, versions: s.versions}, nil
}

type mvccTransaction struct {
	*transaction
	owner    *MVCC
	versions map[core.StateKey]*revision
}

func (t *mvccTransaction) validate(ctx context.Context) error {
	if err := t.check(ctx); err != nil {
		return err
	}
	for k := range t.reads {
		if err := t.check(ctx); err != nil {
			return err
		}
		if t.versions[k] != t.owner.versions[k] {
			return core.ErrStateConflict
		}
	}
	for k := range t.writes {
		if err := t.check(ctx); err != nil {
			return err
		}
		if t.versions[k] != t.owner.versions[k] {
			return core.ErrStateConflict
		}
	}
	return nil
}
func (t *mvccTransaction) Validate(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.owner.mu.Lock()
	defer t.owner.mu.Unlock()
	return t.validate(ctx)
}
func (t *mvccTransaction) Commit(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.owner.mu.Lock()
	defer t.owner.mu.Unlock()
	if err := t.validate(ctx); err != nil {
		return err
	}
	if len(t.writes) > 0 {
		values := make(map[core.StateKey][]byte, len(t.owner.values)+len(t.writes))
		versions := make(map[core.StateKey]*revision, len(t.owner.versions)+len(t.writes))
		for k, v := range t.owner.values {
			if err := t.check(ctx); err != nil {
				return err
			}
			values[k] = v
		}
		for k, v := range t.owner.versions {
			if err := t.check(ctx); err != nil {
				return err
			}
			versions[k] = v
		}
		for k, w := range t.writes {
			if err := t.check(ctx); err != nil {
				return err
			}
			if w.Deleted {
				delete(values, k)
			} else {
				values[k] = w.Value
			}
			versions[k] = &revision{}
		}
		if err := t.check(ctx); err != nil {
			return err
		}
		t.owner.values, t.owner.versions = values, versions
	}
	t.closed = true
	t.snapshot = nil
	t.versions = nil
	return nil
}

// Rollback additionally releases the version generation retained by this snapshot.
func (t *mvccTransaction) Rollback() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closed = true
	t.snapshot = nil
	t.versions = nil
	return nil
}
