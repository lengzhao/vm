package wasm

import (
	"context"
	"errors"
	"github.com/lengzhao/vm/core"
	"github.com/lengzhao/vm/internal/artifact"
	"github.com/lengzhao/vm/internal/artifact/artifacttest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestCanceledCacheAcquisitionClassification(t *testing.T) {
	r, _ := New(Options{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := r.acquireCache(ctx, "artifact")
	var executionErr *core.ExecutionError
	if !errors.As(err, &executionErr) || executionErr.Code != core.CodeCanceled || !errors.Is(err, context.Canceled) {
		t.Fatalf("cache cancellation lost category: %v", err)
	}
}

func TestRunnerOwnsCacheLifecycle(t *testing.T) {
	r, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	stopper, ok := any(r).(interface{ Stop(context.Context) error })
	if !ok {
		t.Fatal("runner must close its retained compiled code through Stop(ctx)")
	}
	if err := stopper.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := stopper.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

// Closing a retired cache must wait for its last execution lease. Shutdown may
// time out and be retried; it must never free code still in use by a guest.
func TestCacheRetirementAndRetryableStop(t *testing.T) {
	r, _ := New(Options{})
	cache, release, err := r.acquireCache(context.Background(), "first")
	if err != nil {
		t.Fatal(err)
	}
	same, releaseSame, err := r.acquireCache(context.Background(), "first")
	if err != nil || same != cache {
		t.Fatal("same artifact did not share compiled code")
	}
	other, releaseOther, err := r.acquireCache(context.Background(), "second")
	if err != nil || other == cache {
		t.Fatal("different artifact reused wrong generation")
	}
	if err := releaseOther(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := r.Stop(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	if _, _, err := r.acquireCache(context.Background(), "third"); err == nil {
		t.Fatal("shutdown accepted new execution")
	}
	if err := releaseSame(); err != nil {
		t.Fatal(err)
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
	if err := r.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.acquireCache(context.Background(), "first"); err == nil {
		t.Fatal("stopped runner accepted execution")
	}
}

func TestConcurrentGuestsAndStop(t *testing.T) {
	data := []byte{0, 97, 115, 109, 1, 0, 0, 0, 1, 4, 1, 96, 0, 0, 3, 2, 1, 0, 7, 10, 1, 6, 95, 115, 116, 97, 114, 116, 0, 0, 10, 9, 1, 7, 0, 3, 64, 12, 0, 11, 11}
	path := filepath.Join(t.TempDir(), "loop.wasm")
	if err := os.WriteFile(path, data, 0755); err != nil {
		t.Fatal(err)
	}
	contract := artifacttest.Seal(t, &core.CompiledContract{ExecutablePath: path, ProtocolVersion: core.ProtocolVersion})
	build := contract.Manifest.Build
	build.Toolchain.GOOS = "wasip1"
	build.Toolchain.GOARCH = "wasm"
	if err := artifact.Seal(context.Background(), contract, build); err != nil {
		t.Fatal(err)
	}
	r, _ := New(Options{Timeout: 5 * time.Second})
	t.Cleanup(func() { _ = r.Stop(context.Background()) })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 2)
	for range 2 {
		go func() { _, err := r.Run(ctx, contract, core.CallRequest{Function: "F"}); done <- err }()
	}
	startup, stopStartup := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopStartup()
	for {
		r.code.mu.Lock()
		ready := r.code.current != nil && r.code.current.refs == 2
		r.code.mu.Unlock()
		if ready {
			break
		}
		if err := startup.Err(); err != nil {
			t.Fatal("guests did not acquire leases", err)
		}
		runtime.Gosched()
	}
	stopping, stopStopping := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer stopStopping()
	if err := r.Stop(stopping); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("active guests were not retained: %v", err)
	}
	cancel()
	for range 2 {
		select {
		case err := <-done:
			var ee *core.ExecutionError
			if !errors.As(err, &ee) || ee.Code != core.CodeCanceled {
				t.Fatalf("guest cancellation: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("guest failed to cancel")
		}
	}
	if err := r.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(r.code.entries) != 0 {
		t.Fatal("compiled code retained after Stop")
	}
}
