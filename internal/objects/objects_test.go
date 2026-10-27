package objects

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/lengzhao/vm/core"
	"github.com/lengzhao/vm/internal/state"
)

func TestOwnershipLifecycleAndIsolation(t *testing.T) {
	ctx := context.Background()
	db := state.NewMVCC()
	svc := New(OwnerPolicy{})
	apply := func(contract, sender string, cmd core.ObjectCommand, wantErr bool) core.ObjectReply {
		t.Helper()
		tx, _ := db.Begin(ctx)
		defer tx.Rollback()
		s := svc.Bind(tx, core.ObjectScope{Contract: core.Address(contract), Sender: core.Address(sender)})
		r, e := s.Apply(ctx, cmd)
		if (e != nil) != wantErr {
			t.Fatalf("%+v: %v", cmd, e)
		}
		if wantErr {
			if s.Err() == nil {
				t.Fatal("failure not latched")
			}
			return r
		}
		if e = tx.Commit(ctx); e != nil {
			t.Fatal(e)
		}
		return r
	}
	apply("a", "alice", core.ObjectCommand{ID: "x", Op: "create"}, false)
	apply("a", "alice", core.ObjectCommand{ID: "x", Op: "set", Key: "k", Value: []byte("v")}, false)
	if r := apply("a", "bob", core.ObjectCommand{ID: "x", Op: "get", Key: "k"}, false); !r.Exists || string(r.Value) != "v" {
		t.Fatal(r)
	}
	apply("a", "bob", core.ObjectCommand{ID: "x", Op: "set", Key: "k"}, true)
	apply("b", "alice", core.ObjectCommand{ID: "x", Op: "get", Key: "k"}, true)
	apply("a", "alice", core.ObjectCommand{ID: "x", Op: "transfer", Owner: "bob"}, false)
	apply("a", "alice", core.ObjectCommand{ID: "x", Op: "delete", Key: "k"}, true)
	apply("a", "bob", core.ObjectCommand{ID: "x", Op: "delete", Key: "k"}, false)
	if r := apply("a", "alice", core.ObjectCommand{ID: "x", Op: "owner"}, false); r.Owner != "bob" {
		t.Fatal(r)
	}
	apply("a", "bob", core.ObjectCommand{ID: "x", Op: "destroy"}, false)
	apply("a", "bob", core.ObjectCommand{ID: "x", Op: "create"}, true)
	apply("a", "bob", core.ObjectCommand{ID: "x", Op: "owner"}, true)
}

func TestObjectConflictRollbackAndDefaultNamespace(t *testing.T) {
	ctx := context.Background()
	db := state.NewMVCC()
	svc := New(OwnerPolicy{})
	scope := core.ObjectScope{Contract: "a", Sender: "alice"}
	tx, _ := db.Begin(ctx)
	s := svc.Bind(tx, scope)
	if _, e := s.Apply(ctx, core.ObjectCommand{ID: "x", Op: "create"}); e != nil {
		t.Fatal(e)
	}
	if e := tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	a, _ := db.Begin(ctx)
	b, _ := db.Begin(ctx)
	defer a.Rollback()
	defer b.Rollback()
	for _, tx := range []core.StateTransaction{a, b} {
		if _, e := svc.Bind(tx, scope).Apply(ctx, core.ObjectCommand{ID: "x", Op: "transfer", Owner: "bob"}); e != nil {
			t.Fatal(e)
		}
	}
	if e := a.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	if e := b.Commit(ctx); !errors.Is(e, core.ErrStateConflict) {
		t.Fatal(e)
	}
	tx, _ = db.Begin(ctx)
	defer tx.Rollback()
	s = svc.Bind(tx, core.ObjectScope{Contract: "a", Sender: "bob"})
	if _, e := s.Apply(ctx, core.ObjectCommand{ID: "x", Op: "set", Key: "", Value: []byte("secret")}); e != nil {
		t.Fatal(e)
	}
	for _, w := range tx.WriteSet() {
		if e := state.Bind(tx, "a").Set(ctx, w.Key.Key, []byte("overwrite")); e == nil {
			t.Fatal("default object reached reserved key")
		}
	}
	tx.Rollback()
	tx, _ = db.Begin(ctx)
	defer tx.Rollback()
	r, e := svc.Bind(tx, scope).Apply(ctx, core.ObjectCommand{ID: "x", Op: "get", Key: ""})
	if e != nil || r.Exists {
		t.Fatal(r, e)
	}
}

func TestObjectValidationCancellationAndCopies(t *testing.T) {
	ctx := context.Background()
	db := state.NewMVCC()
	svc := New(OwnerPolicy{})
	scope := core.ObjectScope{Contract: "a", Sender: "alice"}
	for _, cmd := range []core.ObjectCommand{{ID: "", Op: "create"}, {ID: strings.Repeat("x", 129), Op: "create"}, {ID: "x", Op: "create", Owner: "other"}, {ID: "x", Op: "unknown"}} {
		tx, _ := db.Begin(ctx)
		s := svc.Bind(tx, scope)
		if _, e := s.Apply(ctx, cmd); e == nil {
			t.Fatal(cmd)
		}
		tx.Rollback()
	}
	tx, _ := db.Begin(ctx)
	defer tx.Rollback()
	s := svc.Bind(tx, scope)
	if _, e := s.Apply(ctx, core.ObjectCommand{ID: "x", Op: "create"}); e != nil {
		t.Fatal(e)
	}
	v := []byte("value")
	if _, e := s.Apply(ctx, core.ObjectCommand{ID: "x", Op: "set", Value: v}); e != nil {
		t.Fatal(e)
	}
	v[0] = 'X'
	r, e := s.Apply(ctx, core.ObjectCommand{ID: "x", Op: "get"})
	if e != nil || string(r.Value) != "value" {
		t.Fatal(r, e)
	}
	r.Value[0] = 'X'
	r, e = s.Apply(ctx, core.ObjectCommand{ID: "x", Op: "get"})
	if e != nil || string(r.Value) != "value" {
		t.Fatal(r, e)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e = s.Apply(canceled, core.ObjectCommand{ID: "x", Op: "owner"}); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if _, e = s.Apply(ctx, core.ObjectCommand{ID: "x", Op: "owner"}); !errors.Is(e, context.Canceled) {
		t.Fatal("unlatched cancel", e)
	}
}

func TestObjectLimitsCloseAndCorruption(t *testing.T) {
	ctx := context.Background()
	svc := New(OwnerPolicy{})
	scope := core.ObjectScope{Contract: "a", Sender: "alice"}
	t.Run("fields", func(t *testing.T) {
		db := state.NewMVCC()
		tx, _ := db.Begin(ctx)
		defer tx.Rollback()
		s := svc.Bind(tx, scope)
		s.Apply(ctx, core.ObjectCommand{ID: "x", Op: "create"})
		for i := 0; i < MaxFields; i++ {
			if _, e := s.Apply(ctx, core.ObjectCommand{ID: "x", Op: "set", Key: fmt.Sprint(i)}); e != nil {
				t.Fatal(e)
			}
		}
		if _, e := s.Apply(ctx, core.ObjectCommand{ID: "x", Op: "set", Key: "overflow"}); e == nil {
			t.Fatal("field quota ignored")
		}
	})
	t.Run("bytes", func(t *testing.T) {
		db := state.NewMVCC()
		tx, _ := db.Begin(ctx)
		defer tx.Rollback()
		s := svc.Bind(tx, scope)
		s.Apply(ctx, core.ObjectCommand{ID: "x", Op: "create"})
		hit := false
		for i := 0; i < MaxFields; i++ {
			if _, e := s.Apply(ctx, core.ObjectCommand{ID: "x", Op: "set", Key: fmt.Sprint(i), Value: make([]byte, core.MaxObjectValueBytes)}); e != nil {
				hit = true
				break
			}
		}
		if !hit {
			t.Fatal("record quota ignored")
		}
	})
	t.Run("closed", func(t *testing.T) {
		tx, _ := state.NewMVCC().Begin(ctx)
		defer tx.Rollback()
		s := svc.Bind(tx, scope)
		s.Close()
		if _, e := s.Apply(ctx, core.ObjectCommand{ID: "x", Op: "create"}); e == nil {
			t.Fatal("closed frame retained authority")
		}
	})
	t.Run("corruption", func(t *testing.T) {
		tx, _ := state.NewMVCC().Begin(ctx)
		defer tx.Rollback()
		tx.Set(ctx, core.StateKey{Contract: "a", Key: keyPrefix + "x"}, []byte(`{"version":99}`))
		s := svc.Bind(tx, scope)
		_, e := s.Apply(ctx, core.ObjectCommand{ID: "x", Op: "get"})
		var business *core.ExecutionError
		if e == nil || errors.As(e, &business) {
			t.Fatal("corruption must be infrastructure failure", e)
		}
	})
	t.Run("missing-sender", func(t *testing.T) {
		tx, _ := state.NewMVCC().Begin(ctx)
		defer tx.Rollback()
		s := svc.Bind(tx, core.ObjectScope{Contract: "a"})
		if _, e := s.Apply(ctx, core.ObjectCommand{ID: "x", Op: "create"}); e == nil {
			t.Fatal("anonymous creator")
		}
	})
}
