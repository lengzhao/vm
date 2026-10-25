package assembly_test

import (
	"context"
	"errors"
	"github.com/lengzhao/pluginkit"
	"github.com/lengzhao/vm/assembly"
	"github.com/lengzhao/vm/core"
	"reflect"
	"sync"
	"testing"
	"time"
)

var rollbackMu sync.Mutex
var rollbackEvents []string
var rollbackCancel context.CancelFunc
var rollbackFailure = errors.New("rollback startup failure")

type rollbackConfig struct {
	Name   string `json:"name"`
	Fail   bool   `json:"fail"`
	Cancel bool   `json:"cancel"`
}
type rollbackNode struct{ config rollbackConfig }

func (n *rollbackNode) Start(context.Context) error {
	rollbackEvents = append(rollbackEvents, "start:"+n.config.Name)
	if n.config.Cancel {
		rollbackCancel()
	}
	if n.config.Fail {
		return rollbackFailure
	}
	return nil
}
func (n *rollbackNode) Stop(context.Context) error {
	rollbackEvents = append(rollbackEvents, "stop:"+n.config.Name)
	return nil
}

type rollbackLazy struct{}

func (*rollbackLazy) Stop(context.Context) error {
	rollbackEvents = append(rollbackEvents, "stop:lazy")
	return nil
}

type rollbackEngine struct{ core.Engine }

func init() {
	pluginkit.Register("test/rollback-first", func(c rollbackConfig) (*rollbackNode, error) { return &rollbackNode{config: c}, nil })
	pluginkit.Register("test/rollback-next", func(c rollbackConfig, _ struct {
		Previous *rollbackNode `json:"previous"`
	}) (*rollbackNode, error) {
		return &rollbackNode{config: c}, nil
	})
	pluginkit.Register("test/rollback-lazy", func(struct{}) (*rollbackLazy, error) { return &rollbackLazy{}, nil })
	pluginkit.Register("test/rollback-root", func(_ struct{}, _ struct {
		Last *rollbackNode `json:"last"`
		Lazy *rollbackLazy `json:"lazy"`
	}) (core.Engine, error) {
		return &rollbackEngine{}, nil
	})
}
func rollbackGraph(fail, cancel bool) assembly.Config {
	node := func(use string, c map[string]any, d map[string]any) any {
		return map[string]any{"use": use, "config": c, "deps": d}
	}
	return assembly.Config{Root: "root", Plugins: map[string]any{
		"prior": node("test/rollback-first", map[string]any{"name": "prior"}, nil),
		"a":     node("test/rollback-next", map[string]any{"name": "a", "fail": fail, "cancel": cancel}, map[string]any{"previous": "prior"}),
		"b":     node("test/rollback-next", map[string]any{"name": "b"}, map[string]any{"previous": "a"}),
		"lazy":  map[string]any{"use": "test/rollback-lazy"},
		"root":  node("test/rollback-root", nil, map[string]any{"last": "b", "lazy": "lazy"}),
	}}
}
func TestRollbackStopsOnlyAttemptedStarters(t *testing.T) {
	rollbackMu.Lock()
	defer rollbackMu.Unlock()
	for _, canceled := range []bool{false, true} {
		t.Run(map[bool]string{false: "start_failure", true: "cancellation"}[canceled], func(t *testing.T) {
			rollbackEvents = nil
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			rollbackCancel = cancel
			rt, err := assembly.Build(ctx, rollbackGraph(!canceled, canceled))
			wantErr := rollbackFailure
			if canceled {
				wantErr = context.Canceled
			}
			if rt != nil || !errors.Is(err, wantErr) {
				t.Fatalf("runtime=%v error=%v", rt, err)
			}
			want := []string{"start:prior", "start:a", "stop:a", "stop:prior"}
			if !reflect.DeepEqual(rollbackEvents, want) {
				t.Fatalf("events=%v want=%v", rollbackEvents, want)
			}
		})
	}
}
func TestSuccessfulRuntimeStopsLazyPlugins(t *testing.T) {
	rollbackMu.Lock()
	defer rollbackMu.Unlock()
	rollbackEvents = nil
	rt, err := assembly.Build(context.Background(), rollbackGraph(false, false))
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, event := range rollbackEvents {
		if event == "stop:lazy" {
			found = true
		}
	}
	if !found {
		t.Fatalf("lazy plugin was not stopped: %v", rollbackEvents)
	}
	count := len(rollbackEvents)
	if err := rt.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(rollbackEvents) != count {
		t.Fatal("duplicate shutdown")
	}
}

type retryShutdownState struct {
	calls []string
	first bool
}
type retryShutdownNode struct {
	state *retryShutdownState
	name  string
	block bool
}

func (n *retryShutdownNode) Start(context.Context) error { return nil }
func (n *retryShutdownNode) Stop(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	n.state.calls = append(n.state.calls, n.name)
	if n.block && !n.state.first {
		n.state.first = true
		<-ctx.Done()
		return ctx.Err()
	}
	return nil
}

var retryState *retryShutdownState

func init() {
	pluginkit.Register("test/retry-first", func(struct{}) (*retryShutdownNode, error) {
		return &retryShutdownNode{state: retryState, name: "first"}, nil
	})
	pluginkit.Register("test/retry-blocking", func(_ struct{}, _ struct {
		Previous *retryShutdownNode `json:"previous"`
	}) (*retryShutdownNode, error) { return &retryShutdownNode{state: retryState, name: "blocking", block: true}, nil })
	pluginkit.Register("test/retry-last", func(_ struct{}, _ struct {
		Previous *retryShutdownNode `json:"previous"`
	}) (*retryShutdownNode, error) { return &retryShutdownNode{state: retryState, name: "last"}, nil })
	pluginkit.Register("test/retry-root", func(_ struct{}, _ struct {
		Last *retryShutdownNode `json:"last"`
	}) (core.Engine, error) { return &rollbackEngine{}, nil })
}
func TestShutdownDeadlineCanRetryPendingPlugins(t *testing.T) {
	rollbackMu.Lock()
	defer rollbackMu.Unlock()
	retryState = &retryShutdownState{}
	cfg := assembly.Config{Root: "root", Plugins: map[string]any{
		"first":    map[string]any{"use": "test/retry-first"},
		"blocking": map[string]any{"use": "test/retry-blocking", "deps": map[string]any{"previous": "first"}},
		"last":     map[string]any{"use": "test/retry-last", "deps": map[string]any{"previous": "blocking"}},
		"root":     map[string]any{"use": "test/retry-root", "deps": map[string]any{"last": "last"}},
	}}
	rt, err := assembly.Build(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := rt.Stop(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline, got %v", err)
	}
	if !reflect.DeepEqual(retryState.calls, []string{"last", "blocking"}) {
		t.Fatalf("first shutdown: %v", retryState.calls)
	}
	if err := rt.Stop(context.Background()); err != nil {
		t.Fatalf("retry retained transient error: %v", err)
	}
	want := []string{"last", "blocking", "blocking", "first"}
	if !reflect.DeepEqual(retryState.calls, want) {
		t.Fatalf("retry events=%v want=%v", retryState.calls, want)
	}
	if err := rt.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(retryState.calls, want) {
		t.Fatalf("successful plugins stopped repeatedly: %v", retryState.calls)
	}
}
