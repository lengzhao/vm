package process

import (
	"context"
	"errors"
	"github.com/lengzhao/vm/core"
	"github.com/lengzhao/vm/internal/callcontext"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type observedArgument struct {
	calls  *int
	cancel context.CancelFunc
}

func (a observedArgument) MarshalJSON() ([]byte, error) {
	*a.calls++
	if a.cancel != nil {
		a.cancel()
	}
	return []byte(`1`), nil
}

type observedHost struct{ calls int }

func (h *observedHost) BlockHeight() uint64           { h.calls++; return 0 }
func (h *observedHost) BlockTime() uint64             { h.calls++; return 0 }
func (h *observedHost) Sender() core.Address          { h.calls++; return "" }
func (h *observedHost) ContractAddress() core.Address { h.calls++; return "" }
func (h *observedHost) Log(string, ...any)            {}
func TestCanceledRunAvoidsCallbacks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "contract")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, deadline := range []bool{false, true} {
		t.Run(map[bool]string{false: "canceled", true: "deadline"}[deadline], func(t *testing.T) {
			var ctx context.Context
			var cancel context.CancelFunc
			want := core.CodeCanceled
			cause := context.Canceled
			if deadline {
				ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
				want = core.CodeTimeout
				cause = context.DeadlineExceeded
			} else {
				ctx, cancel = context.WithCancel(context.Background())
				cancel()
			}
			defer cancel()
			host := &observedHost{}
			ctx = callcontext.WithCallContext(ctx, &core.CallContext{Host: host})
			calls := 0
			_, err := New(time.Second).Run(ctx, &core.CompiledContract{ExecutablePath: path, ProtocolVersion: core.ProtocolVersion}, core.CallRequest{Function: "Run", Args: []any{observedArgument{calls: &calls}}})
			var ee *core.ExecutionError
			if !errors.As(err, &ee) || ee.Code != want || !errors.Is(err, cause) {
				t.Fatalf("unexpected cancellation: %v", err)
			}
			if calls != 0 || host.calls != 0 {
				t.Fatalf("canceled operation invoked callbacks: marshal=%d host=%d", calls, host.calls)
			}
		})
	}
}
func TestCancellationDuringMarshalAvoidsHostCallbacks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "contract")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	host := &observedHost{}
	ctx = callcontext.WithCallContext(ctx, &core.CallContext{Host: host})
	calls := 0
	_, err := New(time.Second).Run(ctx, &core.CompiledContract{ExecutablePath: path, ProtocolVersion: core.ProtocolVersion}, core.CallRequest{Function: "Run", Args: []any{observedArgument{calls: &calls, cancel: cancel}}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 || host.calls != 0 {
		t.Fatalf("marshal=%d host=%d", calls, host.calls)
	}
}
