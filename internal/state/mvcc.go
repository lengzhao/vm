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
	values   *stateTable[[]byte]
	versions *stateTable[*revision]
}

var _ core.StateStore = (*MVCC)(nil)
var _ core.SnapshotStore = (*MVCC)(nil)

func NewMVCC() *MVCC {
	return &MVCC{values: newStateTable[[]byte](), versions: newStateTable[*revision]()}
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
	versions *stateTable[*revision]
}

func (t *mvccTransaction) validate(ctx context.Context) error {
	if err := t.check(ctx); err != nil {
		return err
	}
	for k := range t.reads {
		if err := t.check(ctx); err != nil {
			return err
		}
		before, _ := t.versions.get(k)
		current, _ := t.owner.versions.get(k)
		if before != current {
			return core.ErrStateConflict
		}
	}
	for k := range t.writes {
		if err := t.check(ctx); err != nil {
			return err
		}
		before, _ := t.versions.get(k)
		current, _ := t.owner.versions.get(k)
		if before != current {
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
func (t *mvccTransaction) Commit(ctx context.Context) error { return t.commit(ctx, nil) }

// commit prepares everything before the optional durable publication boundary.
// After persist succeeds, publication must not fail, even if cancellation races.
func (t *mvccTransaction) commit(ctx context.Context, persist func(map[core.StateKey]core.StateWrite) error) error {
	return t.finalize(ctx, true, func(w map[core.StateKey]core.StateWrite) error {
		if persist != nil && len(w) > 0 {
			return persist(w)
		}
		return nil
	})
}

// finalize may discard guest writes while atomically recording a failure receipt.
// All allocations/checks precede durable publication, including read-only receipts.
func (t *mvccTransaction) finalize(ctx context.Context, apply bool, persist func(map[core.StateKey]core.StateWrite) error) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.owner.mu.Lock()
	defer t.owner.mu.Unlock()
	if err := t.validate(ctx); err != nil {
		return err
	}
	writes := t.writes
	if !apply {
		writes = nil
	}
	values, versions := t.owner.values, t.owner.versions
	if len(writes) > 0 {
		check := func() error { return t.check(ctx) }
		var err error
		values, err = values.apply(check, writes, stateValue)
		if err != nil {
			return err
		}
		versions, err = versions.apply(check, writes, stateVersion)
		if err != nil {
			return err
		}
	}
	if err := t.check(ctx); err != nil {
		return err
	}
	if persist != nil {
		if err := persist(writes); err != nil {
			return err
		}
	}
	t.owner.values, t.owner.versions = values, versions
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
