// Package engine composes VM business capabilities. It owns no compatibility
// state, filesystem deployment rules, lifecycle protocol or concrete policies.
package engine

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/lengzhao/vm/abi"
	"github.com/lengzhao/vm/core"
	"github.com/lengzhao/vm/internal/callcontext"
)

type Options struct {
	MaxCallDepth      int
	MaxGasLimit       uint64
	EnableGasMetering bool
	ExecutionTimeout  time.Duration
}
type Deps struct {
	Policy    core.SourcePolicy
	ABI       core.ABIGenerator
	Compiler  core.ContractCompiler
	Deployer  core.ContractDeployer
	Contracts core.ContractReader
	Runner    core.Runner
	Meters    core.GasMeterFactory
	Estimator core.GasEstimator
	State     core.StateStore
	Validator core.ArtifactValidator
}
type Engine struct {
	options Options
	deps    Deps
}

func New(o Options, d Deps) (*Engine, error) {
	if d.State == nil || d.Policy == nil || d.ABI == nil || d.Compiler == nil || d.Deployer == nil || d.Contracts == nil || d.Runner == nil || d.Estimator == nil || d.Validator == nil || (o.EnableGasMetering && d.Meters == nil) {
		return nil, fmt.Errorf("engine dependencies required")
	}
	if o.MaxCallDepth == 0 {
		o.MaxCallDepth = core.DefaultMaxCallDepth
	}
	if o.MaxCallDepth < 1 || o.MaxCallDepth > core.MaxCallDepth {
		return nil, fmt.Errorf("maxCallDepth must be between 1 and %d", core.MaxCallDepth)
	}
	if o.ExecutionTimeout <= 0 {
		o.ExecutionTimeout = 30 * time.Second
	}
	return &Engine{options: o, deps: d}, nil
}
func (e *Engine) Compile(ctx context.Context, source string) (*core.CompiledContract, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return e.deps.Compiler.Compile(ctx, source)
}
func (e *Engine) GenerateABI(ctx context.Context, source string) (*abi.ABI, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := e.deps.Policy.Review(source); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return e.deps.ABI.Generate(source)
}
func (e *Engine) Deploy(ctx context.Context, request core.DeploymentRequest) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return e.deps.Deployer.Deploy(ctx, request)
}
func (e *Engine) Get(ctx context.Context, address string) (*core.CompiledContract, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	contract, err := e.deps.Contracts.Get(ctx, address)
	if err != nil {
		return nil, err
	}
	if err := e.deps.Validator.ValidateArtifact(ctx, contract); err != nil {
		return nil, err
	}
	return contract, nil
}

func (e *Engine) Execute(ctx context.Context, req core.ExecutionRequest) (*core.ExecuteResult, error) {
	return e.execute(ctx, req, false)
}
func (e *Engine) execute(ctx context.Context, req core.ExecutionRequest, dryRun bool) (out *core.ExecuteResult, executionErr error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if req.Address == "" || req.Function == "" {
		return nil, &core.ExecutionError{Code: core.CodeInvalidRequest, Message: "contract address and function required"}
	}
	ctx, cancel := context.WithTimeout(ctx, e.options.ExecutionTimeout)
	defer cancel()
	contract, err := e.loadExecution(ctx, req.Address)
	if err != nil {
		return nil, err
	}
	tx, err := e.deps.State.Begin(ctx)
	if err != nil {
		return nil, stateError(err)
	}
	if tx == nil {
		return nil, stateError(fmt.Errorf("state store returned nil transaction"))
	}
	committed := false
	defer func() {
		if !committed {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				executionErr = errors.Join(executionErr, stateError(rollbackErr))
			}
		}
		if executionErr != nil && out != nil {
			out.Data = nil
			out.Events = nil
		}
	}()
	out, executionErr = e.runTransaction(ctx, req, contract, tx)
	if executionErr == nil && !dryRun {
		if commitErr := tx.Commit(ctx); commitErr != nil {
			executionErr = stateError(commitErr)
		} else {
			committed = true
		}
	}
	if dryRun && out != nil {
		out.Events = nil
	}
	return out, executionErr
}

// ExecuteTransaction leaves transaction ownership with its trusted caller.
func (e *Engine) ExecuteTransaction(ctx context.Context, req core.ExecutionRequest, tx core.StateTransaction) (*core.ExecuteResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if tx == nil {
		return nil, fmt.Errorf("state transaction required")
	}
	if req.Address == "" || req.Function == "" {
		return nil, &core.ExecutionError{Code: core.CodeInvalidRequest, Message: "contract address and function required"}
	}
	ctx, cancel := context.WithTimeout(ctx, e.options.ExecutionTimeout)
	defer cancel()
	contract, err := e.loadExecution(ctx, req.Address)
	if err != nil {
		return nil, err
	}
	return e.runTransaction(ctx, req, contract, tx)
}

func (e *Engine) loadExecution(ctx context.Context, address string) (*core.CompiledContract, error) {
	contract, err := e.deps.Contracts.Get(ctx, address)
	if err != nil {
		return nil, fmt.Errorf("failed to load contract: %w", err)
	}
	if err := e.deps.Validator.ValidateArtifact(ctx, contract); err != nil {
		return nil, err
	}
	return contract, nil
}

func (e *Engine) runTransaction(ctx context.Context, req core.ExecutionRequest, contract *core.CompiledContract, tx core.StateTransaction) (out *core.ExecuteResult, executionErr error) {
	defer func() {
		if executionErr != nil && out != nil {
			out.Data = nil
			out.Events = nil
		}
	}()
	cc := req.CallContext
	if cc == nil {
		cc = &core.CallContext{}
	} else {
		snapshot := *cc
		cc = &snapshot
	}
	if cc.ContractAddress == nil {
		a := core.Address(req.Address)
		cc.ContractAddress = &a
	}
	resolved := callcontext.Resolve(cc)
	var meter core.GasMetering
	if e.options.EnableGasMetering {
		meter = e.deps.Meters.NewMeter()
		if meter == nil {
			return nil, fmt.Errorf("gas factory returned nil meter")
		}
		ceiling := e.options.MaxGasLimit
		if ceiling > 0 && (!resolved.GasLimitSet || resolved.GasLimit == 0 || resolved.GasLimit > ceiling) {
			resolved.GasLimit = ceiling
			resolved.GasLimitSet = true
		}
		if resolved.GasLimitSet {
			meter.SetGasLimit(resolved.GasLimit)
		}
		if ceiling > 0 {
			ctx = callcontext.WithGasLimit(ctx, ceiling)
		}
	}
	// Resolve Host exactly once and pass only isolated per-call values to Runner.
	cc = &core.CallContext{}
	if resolved.BlockHeightSet {
		cc.BlockHeight = &resolved.BlockHeight
	}
	if resolved.BlockTimeSet {
		cc.BlockTime = &resolved.BlockTime
	}
	if resolved.SenderSet {
		cc.Sender = &resolved.Sender
	}
	if resolved.ContractSet {
		cc.ContractAddress = &resolved.Contract
	}
	if resolved.GasLimitSet {
		cc.GasLimit = &resolved.GasLimit
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ctx = callcontext.WithCallContext(ctx, cc)
	session := &callSession{engine: e, ctx: ctx, tx: tx}
	result, err := session.run(ctx, contract, req.Address, req.Function, req.Args, cc, []string{req.Address})
	if result == nil {
		if err != nil {
			return nil, err
		}
		return nil, &core.ExecutionError{Code: core.CodeInvalidResponse, Message: "runner returned nil result"}
	}
	out = &core.ExecuteResult{Data: result.Data, Events: result.Events, GasConsumed: result.GasConsumed}
	if meter != nil {
		consumed := result.GasConsumed
		if consumed == 0 {
			consumed = 10
		}
		if gasErr := meter.ConsumeGas(consumed); gasErr != nil && err == nil {
			err = &core.ExecutionError{Code: core.CodeGasLimitExceeded, Message: gasErr.Error(), Cause: gasErr}
		}
	}
	if err == nil && ctx.Err() != nil {
		err = stateError(ctx.Err())
	}
	return out, err
}

func (e *Engine) EstimateGas(ctx context.Context, address, function string, opts *core.EstimateOptions) (*core.GasEstimate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if opts != nil && opts.DryRun {
		out, err := e.execute(ctx, core.ExecutionRequest{Address: address, Function: function, Args: opts.Args, CallContext: opts.CallCtx}, true)
		if err != nil {
			return nil, err
		}
		return &core.GasEstimate{Estimated: out.GasConsumed, Mode: "dry_run"}, nil
	}
	c, err := e.deps.Contracts.Get(ctx, address)
	if err != nil {
		return nil, err
	}
	if err := e.deps.Validator.ValidateArtifact(ctx, c); err != nil {
		return nil, err
	}
	if c.GasProfile == nil {
		return nil, fmt.Errorf("gas profile not found for contract %s", address)
	}
	var bound uint64
	if opts != nil {
		bound = opts.LoopBound
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	estimated, breakdown, err := e.deps.Estimator.Estimate(c.GasProfile, function, bound)
	if err != nil {
		return nil, err
	}
	return &core.GasEstimate{Estimated: estimated, Mode: "static", ProfileUsed: true, Breakdown: breakdown}, nil
}

func stateError(err error) error {
	code := core.CodeStateFailure
	if errors.Is(err, core.ErrStateConflict) {
		code = core.CodeStateConflict
	}
	if errors.Is(err, context.Canceled) {
		code = core.CodeCanceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		code = core.CodeTimeout
	}
	return &core.ExecutionError{Code: code, Message: err.Error(), Cause: err}
}
