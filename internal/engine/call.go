package engine

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/lengzhao/vm/contractapi"
	"github.com/lengzhao/vm/core"
	"github.com/lengzhao/vm/internal/callcontext"
	"github.com/lengzhao/vm/internal/state"
)

// callSession owns one temporary store for the entire tree. Only execute can
// publish it. There are no child commits, savepoints or compensating writes.
type callSession struct {
	engine     *Engine
	ctx        context.Context
	tx         core.StateTransaction
	mu         sync.Mutex
	err        error
	calls      int
	eventBytes int
}

func callError(code core.ErrorCode, message string) error {
	return &core.ExecutionError{Code: code, Message: message}
}
func (s *callSession) fail(err error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err == nil {
		s.err = err
	}
	return s.err
}
func (s *callSession) failure() error { s.mu.Lock(); defer s.mu.Unlock(); return s.err }
func (s *callSession) events(events []core.Event) error {
	if len(events) == 0 {
		return nil
	}
	encoded, err := json.Marshal(events)
	if err != nil {
		return s.fail(callError(core.CodeInvalidResult, err.Error()))
	}
	s.mu.Lock()
	s.eventBytes += len(encoded)
	exceeded := s.eventBytes > core.MaxCallEventBytes
	s.mu.Unlock()
	if exceeded {
		return s.fail(callError(core.CodeOutputLimit, "call tree events exceed 1 MiB"))
	}
	return nil
}

type callFrame struct {
	session *callSession
	address string
	stack   []string
	cc      *core.CallContext
	limit   uint64
	mu      sync.Mutex // serializes child calls; each child has its own frame lock
	floor   uint64
	events  []core.Event
	closed  bool
}

func (s *callSession) run(ctx context.Context, contract *core.CompiledContract, address, function string, args []any, cc *core.CallContext, stack []string) (*core.CallResult, error) {
	if err := s.failure(); err != nil {
		return nil, err
	}
	f := &callFrame{session: s, address: address, stack: stack, cc: cc}
	if cc.GasLimit != nil {
		f.limit = *cc.GasLimit
	}
	object := state.Bind(s.tx, core.Address(address))
	ctx = core.WithObjectStore(ctx, object)
	ctx = core.WithContractCaller(ctx, f)
	ctx = callcontext.WithCallContext(ctx, cc)
	ctx = callcontext.WithGasLimit(ctx, f.limit)
	result, err := s.engine.deps.Runner.Run(ctx, contract, core.CallRequest{Function: function, Args: args})
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	if err == nil && ctx.Err() != nil {
		err = stateError(ctx.Err())
	}
	if err == nil && object.Err() != nil {
		err = stateError(object.Err())
	}
	if result == nil {
		if err == nil {
			err = callError(core.CodeInvalidResponse, "runner returned nil result")
		}
		if f.floor > 0 {
			result = &core.CallResult{GasConsumed: f.floor}
		}
	} else {
		if err == nil && (result.GasConsumed < f.floor || (f.limit > 0 && result.GasConsumed > f.limit)) {
			err = callError(core.CodeGasLimitExceeded, "call frame gas exceeds budget or omits child gas")
		}
		if result.GasConsumed < f.floor {
			result.GasConsumed = f.floor
		}
		if err == nil {
			err = s.events(result.Events)
			f.events = append(f.events, result.Events...)
		}
		result = &core.CallResult{Data: result.Data, RawResult: result.RawResult, GasConsumed: result.GasConsumed, Events: f.events, Stdout: result.Stdout, Stderr: result.Stderr}
	}
	if err != nil {
		err = s.fail(err)
	} else {
		err = s.failure()
	}
	return result, err
}
func (f *callFrame) Call(ctx context.Context, req core.CrossCallRequest) (*core.CallResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s := f.session
	if f.closed {
		return nil, callError(core.CodeCallFailure, "call frame already finished")
	}
	reject := func(err error) (*core.CallResult, error) { return nil, s.fail(err) }
	if err := s.failure(); err != nil {
		return nil, err
	}
	if err := s.ctx.Err(); err != nil {
		return reject(stateError(err))
	}
	if err := ctx.Err(); err != nil {
		return reject(stateError(err))
	}
	if req.Address == "" || req.Function == "" {
		return reject(callError(core.CodeCallFailure, "target address and function required"))
	}
	if req.GasConsumed < f.floor || req.GasConsumed-f.floor < contractapi.GasCrossCall {
		return reject(callError(core.CodeInvalidResponse, "undercharged cross-contract call"))
	}
	f.floor = req.GasConsumed
	if len(f.stack) >= s.engine.options.MaxCallDepth {
		return reject(callError(core.CodeCallDepth, "maximum call depth exceeded"))
	}
	for _, address := range f.stack {
		if req.Address == address {
			return reject(callError(core.CodeReentrancy, "contract already active in call stack"))
		}
	}
	remaining := uint64(0)
	if f.limit > 0 {
		if req.GasConsumed >= f.limit || f.limit-req.GasConsumed < contractapi.GasEntryBase {
			return reject(callError(core.CodeGasLimitExceeded, "insufficient remaining gas for child"))
		}
		remaining = f.limit - req.GasConsumed
	}
	s.mu.Lock()
	s.calls++
	exceeded := s.calls > core.MaxCrossCalls
	s.mu.Unlock()
	if exceeded {
		return reject(callError(core.CodeCallFailure, "cross-contract call limit exceeded"))
	}
	if err := s.events(req.Events); err != nil {
		return nil, err
	}
	f.events = append(f.events, req.Events...)
	// An operation cannot detach the nested call from the root deadline, even if
	// a custom runner supplies a fresh context.
	childCtx, cancel := context.WithCancel(s.ctx)
	defer cancel()
	stop := context.AfterFunc(ctx, cancel)
	defer stop()
	contract, err := s.engine.Get(childCtx, req.Address)
	if err != nil {
		return reject(err)
	}
	childCC := *f.cc
	sender, address := core.Address(f.address), core.Address(req.Address)
	childCC.Sender = &sender
	childCC.ContractAddress = &address
	childCC.GasLimit = &remaining
	stack := append(append([]string(nil), f.stack...), req.Address)
	result, err := s.run(childCtx, contract, req.Address, req.Function, req.Args, &childCC, stack)
	if result != nil {
		if result.GasConsumed > ^uint64(0)-req.GasConsumed {
			return reject(callError(core.CodeGasLimitExceeded, "cross-contract gas overflow"))
		}
		f.floor = req.GasConsumed + result.GasConsumed
		if f.limit > 0 && f.floor > f.limit {
			return reject(callError(core.CodeGasLimitExceeded, "child exceeds shared gas budget"))
		}
	}
	if err != nil {
		return nil, s.fail(err)
	}
	if result == nil {
		return reject(callError(core.CodeInvalidResponse, "nil child result"))
	}
	if len(result.Data) > core.MaxCrossResultBytes || (len(result.Data) > 0 && !json.Valid(result.Data)) {
		return reject(callError(core.CodeInvalidResponse, "invalid or oversized child JSON result"))
	}
	f.events = append(f.events, result.Events...)
	return &core.CallResult{Data: result.Data, GasConsumed: result.GasConsumed}, nil
}

var _ core.ContractCaller = (*callFrame)(nil)
