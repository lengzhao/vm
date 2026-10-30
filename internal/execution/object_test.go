package execution

import (
	"context"
	"errors"
	"fmt"
	"github.com/lengzhao/vm/core"
	"io"
	"testing"
)

type objectProbe struct {
	value []byte
	calls int
	err   error
}

func (p *objectProbe) Get(context.Context, string) ([]byte, bool, error) {
	p.calls++
	return p.value, true, p.err
}
func (p *objectProbe) Set(context.Context, string, []byte) error { p.calls++; return p.err }
func (p *objectProbe) Delete(context.Context, string) error      { p.calls++; return p.err }
func TestObjectRPCBudgetsAndBackendFailure(t *testing.T) {
	for _, mode := range []string{"calls", "reply-bytes", "backend", "unavailable", "gas-limit"} {
		t.Run(mode, func(t *testing.T) {
			probe := &objectProbe{}
			sentinel := errors.New("backend down")
			output := &objectOutput{boundedOutput: boundedOutput{limit: 1 << 20, cancel: func() {}}, ctx: context.Background(), reply: io.Discard, store: probe, gasFloor: 10}
			if mode == "reply-bytes" {
				probe.value = make([]byte, 64<<10)
			}
			if mode == "backend" {
				probe.err = sentinel
			}
			if mode == "unavailable" {
				output.store = nil
			}
			if mode == "gas-limit" {
				output.gasLimit = 14
			}
			want := core.CodeInvalidResponse
			if mode == "reply-bytes" {
				want = core.CodeOutputLimit
			}
			if mode == "backend" || mode == "unavailable" {
				want = core.CodeStateFailure
			}
			if mode == "gas-limit" {
				want = core.CodeGasLimitExceeded
			}
			for i := 1; i <= core.MaxObjectCalls+1; i++ {
				frame := fmt.Sprintf("{\"version\":3,\"type\":\"object\",\"op\":\"get\",\"key\":\"x\",\"gas\":%d}\n", 10+i*5)
				if err := output.handle([]byte(frame)); err != nil {
					break
				}
			}
			var ee *core.ExecutionError
			if !errors.As(output.err, &ee) || ee.Code != want {
				t.Fatalf("err=%v", output.err)
			}
			if mode == "backend" && !errors.Is(output.err, sentinel) {
				t.Fatal("lost backend cause")
			}
			if mode == "calls" && probe.calls != core.MaxObjectCalls {
				t.Fatalf("calls=%d", probe.calls)
			}
			if mode == "gas-limit" && probe.calls != 0 {
				t.Fatal("over-budget callback invoked")
			}
		})
	}
}
