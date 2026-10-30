package process

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lengzhao/vm/core"
	"github.com/lengzhao/vm/internal/artifact/artifacttest"
	"github.com/lengzhao/vm/internal/state"
)

func objectScript(t *testing.T, script string) *core.CompiledContract {
	t.Helper()
	path := filepath.Join(t.TempDir(), "contract")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0755); err != nil {
		t.Fatal(err)
	}
	c := &core.CompiledContract{ExecutablePath: path, ProtocolVersion: core.ProtocolVersion}
	artifacttest.Seal(t, c)
	return c
}
func TestObjectRPCWritesScopedTransaction(t *testing.T) {
	c := objectScript(t, "read request\necho '{\"version\":3,\"type\":\"object\",\"op\":\"set\",\"key\":\"x\",\"value\":\"eQ==\",\"gas\":20}'\nread reply\necho '{\"version\":3,\"ok\":true,\"gas\":20,\"result\":1}'\n")
	s := state.New()
	tx, _ := s.Begin(context.Background())
	defer tx.Rollback()
	ctx := core.WithObjectStore(context.Background(), state.Bind(tx, "real"))
	result, err := New(time.Second*5).Run(ctx, c, core.CallRequest{Function: "F"})
	if err != nil {
		t.Fatal(err)
	}
	if string(result.Data) != "1" {
		t.Fatal(string(result.Data))
	}
	value, ok, err := tx.Get(ctx, core.StateKey{Contract: "real", Key: "x"})
	if err != nil || !ok || string(value) != "y" {
		t.Fatalf("state=%q %v %v", value, ok, err)
	}
}

func TestObjectRPCFailurePreservesConfirmedGas(t *testing.T) {
	for _, test := range []struct {
		name, script string
		code         core.ErrorCode
	}{
		{"invalid_response", "echo invalid", core.CodeInvalidResponse},
		{"underreported_gas", `echo '{"version":3,"ok":true,"gas":1,"result":1}'`, core.CodeInvalidResponse},
		{"process_exit", "exit 1", core.CodeProcessFailure},
		{"timeout", "while :; do :; done", core.CodeTimeout},
		{"output_limit", "while :; do printf '%01024d' 0; done", core.CodeOutputLimit},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := objectScript(t, "read request\necho '{\"version\":3,\"type\":\"object\",\"op\":\"set\",\"key\":\"x\",\"value\":\"eQ==\",\"gas\":20}'\nread reply\n"+test.script+"\n")
			tx, err := state.New().Begin(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			ctx := core.WithObjectStore(context.Background(), state.Bind(tx, "real"))
			result, err := New(5*time.Second).Run(ctx, c, core.CallRequest{Function: "F"})
			var ee *core.ExecutionError
			if !errors.As(err, &ee) || ee.Code != test.code {
				t.Fatalf("err=%v, want %s", err, test.code)
			}
			if result == nil {
				t.Fatal("missing failure result")
			}
			if result.GasConsumed != 20 {
				t.Fatalf("gas=%d, want confirmed gas 20", result.GasConsumed)
			}
		})
	}
}
func TestObjectRPCRejectsMalformedAndUnderchargedFrames(t *testing.T) {
	for _, frame := range []string{
		`{"version":1,"type":"object","op":"get","key":"x","gas":15}`,
		`{"version":3,"type":"object","op":"set","key":"x","gas":19}`,
		`{"version":3,"type":"object","op":"get","key":"x"}`,
		`{"version":3,"type":"object","op":"get","key":"x","gas":15,"address":"other"}`,
		`{"version":3,"type":"object","op":"commit","key":"x","gas":15}`,
	} {
		t.Run(frame, func(t *testing.T) {
			c := objectScript(t, "read request\necho '"+frame+"'\nread reply\n")
			s := state.New()
			tx, _ := s.Begin(context.Background())
			defer tx.Rollback()
			_, err := New(5*time.Second).Run(core.WithObjectStore(context.Background(), state.Bind(tx, "real")), c, core.CallRequest{Function: "F"})
			var ee *core.ExecutionError
			if !errors.As(err, &ee) || ee.Code != core.CodeInvalidResponse {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func TestCancellationClosesInputWithInheritedReadEnd(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "started")
	c := objectScript(t, "sleep 2 <&0 >/dev/null 2>&1 &\necho ready > '"+marker+"'\nwait\n")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := New(5*time.Second).Run(ctx, c, core.CallRequest{Function: "F", Args: []any{strings.Repeat("x", 256<<10)}})
		done <- err
	}()
	deadline := time.After(5 * time.Second)
	for {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		select {
		case <-deadline:
			t.Fatal("child did not start")
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("stdin write ignored cancellation while descendant retained read end")
	}
}

func TestCancellationClosesLargeObjectReply(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "started")
	c := objectScript(t, "read request\nsleep 2 <&0 >/dev/null 2>&1 &\necho ready > '"+marker+"'\necho '{\"version\":3,\"type\":\"object\",\"op\":\"get\",\"key\":\"x\",\"gas\":15}'\nwait\n")
	s := state.New()
	tx, _ := s.Begin(context.Background())
	defer tx.Rollback()
	_ = tx.Set(context.Background(), core.StateKey{Contract: "real", Key: "x"}, make([]byte, 64<<10))
	ctx, cancel := context.WithCancel(core.WithObjectStore(context.Background(), state.Bind(tx, "real")))
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := New(5*time.Second).Run(ctx, c, core.CallRequest{Function: "F"}); done <- err }()
	deadline := time.After(5 * time.Second)
	for {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		select {
		case <-deadline:
			t.Fatal("child did not start")
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("object reply write ignored cancellation")
	}
}

func TestObjectRPCFinalGasCannotDecrease(t *testing.T) {
	c := objectScript(t, "read request\necho '{\"version\":3,\"type\":\"object\",\"op\":\"get\",\"key\":\"x\",\"gas\":15}'\nread reply\necho '{\"version\":3,\"ok\":true,\"gas\":14,\"result\":1}'\n")
	tx, _ := state.New().Begin(context.Background())
	defer tx.Rollback()
	_, err := New(5*time.Second).Run(core.WithObjectStore(context.Background(), state.Bind(tx, "contract")), c, core.CallRequest{Function: "F"})
	var ee *core.ExecutionError
	if !errors.As(err, &ee) || ee.Code != core.CodeInvalidResponse {
		t.Fatal(err)
	}
}
