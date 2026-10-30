package state_test

import (
	"context"
	"testing"

	"github.com/lengzhao/pluginkit"
	"github.com/lengzhao/pluginkit/build"
	"github.com/lengzhao/vm/core"
	_ "github.com/lengzhao/vm/plugins/all"
)

type config struct{}
type deps struct {
	State core.StateStore `json:"state"`
}
type consumer struct{ store core.StateStore }

func init() {
	pluginkit.Register("test/state-consumer", func(_ config, d deps) (*consumer, error) { return &consumer{store: d.State}, nil })
}
func TestStatePluginGraph(t *testing.T) {
	ctx := context.Background()
	cfg := map[string]any{
		"state":    map[string]any{"use": "state/memory"},
		"consumer": map[string]any{"use": "test/state-consumer", "deps": map[string]any{"state": "state"}},
	}
	c, _, err := build.Build[*consumer](ctx, cfg, "consumer")
	if err != nil {
		t.Fatal(err)
	}
	tx, err := c.store.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	key := core.StateKey{Contract: "contract", Key: "count"}
	if err := tx.Set(ctx, key, []byte("1")); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	next, err := c.store.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Rollback()
	value, ok, err := next.Get(ctx, key)
	if err != nil || !ok || string(value) != "1" {
		t.Fatalf("Get=%q %v %v", value, ok, err)
	}
	// Distinct graph builds do not share a global mutable store.
	other, _, err := build.Build[core.StateStore](ctx, cfg, "state")
	if err != nil {
		t.Fatal(err)
	}
	empty, err := other.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer empty.Rollback()
	if _, ok, err := empty.Get(ctx, key); err != nil || ok {
		t.Fatalf("shared state: %v %v", ok, err)
	}
}
