package process

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/lengzhao/vm/core"
)

type crossProbe struct {
	request core.CrossCallRequest
	result  *core.CallResult
	err     error
	calls   int
}

func (p *crossProbe) Call(_ context.Context, req core.CrossCallRequest) (*core.CallResult, error) {
	p.calls++
	p.request = req
	return p.result, p.err
}
func TestCrossRPCRejectsInvalidFramesBeforeDispatch(t *testing.T) {
	for _, frame := range []string{
		`{"version":2,"type":"call","address":"child","function":"F","args":[],"gas":20}`,
		`{"version":3,"type":"call","address":"child","function":"F","args":[],"gas":19}`,
		`{"version":3,"type":"call","address":"child","function":"F","args":[]}`,
		`{"version":3,"type":"call","address":"child","function":"F","args":null,"gas":20}`,
		`{"version":3,"type":"call","address":"child","function":"F","args":[],"gas":20,"sender":"spoof"}`,
	} {
		t.Run(frame, func(t *testing.T) {
			probe := &crossProbe{}
			ctx := core.WithContractCaller(context.Background(), probe)
			o := objectOutput{ctx: ctx, reply: io.Discard, gasFloor: 10, boundedOutput: boundedOutput{cancel: func() {}}}
			err := o.handleCall([]byte(frame))
			var ee *core.ExecutionError
			if !errors.As(o.err, &ee) || ee.Code != core.CodeInvalidResponse || probe.calls != 0 {
				t.Fatalf("err=%v calls=%d", err, probe.calls)
			}
		})
	}
}
func TestCrossRPCResultGasErrorsAndReplyBudget(t *testing.T) {
	for _, mode := range []string{"success", "backend", "overflow", "budget", "reply-bytes", "bad-result"} {
		t.Run(mode, func(t *testing.T) {
			sentinel := errors.New("child failed")
			probe := &crossProbe{result: &core.CallResult{Data: []byte("7"), GasConsumed: 11}}
			ctx := core.WithContractCaller(context.Background(), probe)
			o := objectOutput{ctx: ctx, reply: io.Discard, gasFloor: 10, boundedOutput: boundedOutput{cancel: func() {}}}
			if mode == "backend" {
				probe.err = sentinel
			}
			if mode == "overflow" {
				probe.result.GasConsumed = ^uint64(0)
			}
			if mode == "budget" {
				o.gasLimit = 30
			}
			if mode == "reply-bytes" {
				o.replyBytes = 1 << 20
			}
			if mode == "bad-result" {
				probe.result.Data = []byte(strings.Repeat("x", core.MaxCrossResultBytes+1))
			}
			frame := `{"version":3,"type":"call","address":"child","function":"F","args":[18446744073709551615],"gas":20,"events":[{"name":"before","fields":{"n":18446744073709551615}}]}`
			err := o.handleCall([]byte(frame))
			if mode == "success" {
				if err != nil || o.gasFloor != 31 || probe.request.Args[0].(json.Number).String() != "18446744073709551615" || probe.request.Events[0].Fields["n"].(json.Number).String() != "18446744073709551615" {
					t.Fatalf("err=%v floor=%d request=%+v", err, o.gasFloor, probe.request)
				}
				return
			}
			if err == nil {
				t.Fatal("accepted invalid child result")
			}
			if mode == "backend" && !errors.Is(err, sentinel) {
				t.Fatal("lost cause")
			}
		})
	}
}
