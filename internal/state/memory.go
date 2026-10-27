// Package state implements in-memory snapshot transactions.
package state

import (
	"context"
	"slices"
	"strings"
	"sync"

	"github.com/lengzhao/vm/core"
)

type Memory struct {
	mu sync.Mutex
	// Identity tokens cannot overflow and remain alive while snapshots refer to them.
	revision *revision
	values   *stateTable[[]byte]
}
type revision struct{ marker byte }

var _ core.StateStore = (*Memory)(nil)
var _ core.StateTransaction = (*transaction)(nil)

func New() *Memory { return &Memory{revision: &revision{}, values: newStateTable[[]byte]()} }

func (s *Memory) Begin(ctx context.Context) (core.StateTransaction, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Commit publishes a new table and never mutates committed values. Keep this
	// immutable generation alive; Get and Set still copy at external boundaries.
	return &transaction{store: s, parent: ctx, base: s.revision, snapshot: s.values, reads: make(map[core.StateKey]struct{}), writes: make(map[core.StateKey]core.StateWrite)}, nil
}

type transaction struct {
	mu       sync.Mutex
	store    *Memory
	parent   context.Context
	base     *revision
	snapshot *stateTable[[]byte]
	reads    map[core.StateKey]struct{}
	writes   map[core.StateKey]core.StateWrite
	closed   bool
}

func (t *transaction) check(ctx context.Context) error {
	if t.closed {
		return core.ErrStateClosed
	}
	if err := t.parent.Err(); err != nil {
		return err
	}
	return ctx.Err()
}
func (t *transaction) Get(ctx context.Context, k core.StateKey) ([]byte, bool, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if err := t.check(ctx); err != nil {
		return nil, false, err
	}
	t.reads[k] = struct{}{}
	if w, ok := t.writes[k]; ok {
		return slices.Clone(w.Value), !w.Deleted, nil
	}
	v, ok := t.snapshot.get(k)
	return slices.Clone(v), ok, nil
}
func (t *transaction) Set(ctx context.Context, k core.StateKey, v []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if err := t.check(ctx); err != nil {
		return err
	}
	t.writes[k] = core.StateWrite{Key: k, Value: slices.Clone(v)}
	return nil
}
func (t *transaction) Delete(ctx context.Context, k core.StateKey) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if err := t.check(ctx); err != nil {
		return err
	}
	t.writes[k] = core.StateWrite{Key: k, Deleted: true}
	return nil
}
func compare(a, b core.StateKey) int {
	if c := strings.Compare(string(a.Contract), string(b.Contract)); c != 0 {
		return c
	}
	return strings.Compare(a.Key, b.Key)
}
func (t *transaction) ReadSet() []core.StateKey {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]core.StateKey, 0, len(t.reads))
	for k := range t.reads {
		out = append(out, k)
	}
	slices.SortFunc(out, compare)
	return out
}
func (t *transaction) WriteSet() []core.StateWrite {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]core.StateWrite, 0, len(t.writes))
	for _, w := range t.writes {
		w.Value = slices.Clone(w.Value)
		out = append(out, w)
	}
	slices.SortFunc(out, func(a, b core.StateWrite) int { return compare(a.Key, b.Key) })
	return out
}
func (t *transaction) Commit(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if err := t.check(ctx); err != nil {
		return err
	}
	t.store.mu.Lock()
	defer t.store.mu.Unlock()
	if err := t.check(ctx); err != nil {
		return err
	}
	if len(t.writes) > 0 {
		if t.base != t.store.revision {
			return core.ErrStateConflict
		}
		next, err := t.store.values.apply(func() error { return t.check(ctx) }, t.writes, stateValue)
		if err != nil {
			return err
		}
		if err := t.check(ctx); err != nil {
			return err
		}
		t.store.values = next
		t.store.revision = &revision{}
	}
	t.closed = true
	t.snapshot = nil
	return nil
}
func (t *transaction) Rollback() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closed = true
	t.snapshot = nil
	return nil
}
