// Package engine composes VM business capabilities. It owns no compatibility
// state, filesystem deployment rules, lifecycle protocol or concrete policies.
package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/lengzhao/vm/abi"
	"github.com/lengzhao/vm/core"
	"github.com/lengzhao/vm/internal/callcontext"
)

type Options struct {
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
	Validator core.ArtifactValidator
}
type Engine struct {
	options Options
	deps    Deps
}

func New(o Options, d Deps) (*Engine, error) {
	if d.Policy == nil || d.ABI == nil || d.Compiler == nil || d.Deployer == nil || d.Contracts == nil || d.Runner == nil || d.Estimator == nil || d.Validator == nil || (o.EnableGasMetering && d.Meters == nil) {
		return nil, fmt.Errorf("engine dependencies required")
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
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if req.Address == "" || req.Function == "" {
		return nil, fmt.Errorf("contract address and function required")
	}
	ctx, cancel := context.WithTimeout(ctx, e.options.ExecutionTimeout)
	defer cancel()
	contract, err := e.deps.Contracts.Get(ctx, req.Address)
	if err != nil {
		return nil, fmt.Errorf("failed to load contract: %w", err)
	}
	if err := e.deps.Validator.ValidateArtifact(ctx, contract); err != nil {
		return nil, err
	}
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
	result, err := e.deps.Runner.Run(ctx, contract, core.CallRequest{Function: req.Function, Args: req.Args})
	if result == nil {
		if err != nil {
			return nil, err
		}
		return nil, &core.ExecutionError{Code: core.CodeInvalidResponse, Message: "runner returned nil result"}
	}
	out := &core.ExecuteResult{Data: result.Data, Events: result.Events, GasConsumed: result.GasConsumed}
	if meter != nil {
		consumed := result.GasConsumed
		if consumed == 0 {
			consumed = 10
		}
		if gasErr := meter.ConsumeGas(consumed); gasErr != nil && err == nil {
			err = &core.ExecutionError{Code: core.CodeGasLimitExceeded, Message: gasErr.Error(), Cause: gasErr}
		}
	}
	return out, err
}

func (e *Engine) EstimateGas(ctx context.Context, address, function string, opts *core.EstimateOptions) (*core.GasEstimate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if opts != nil && opts.DryRun {
		out, err := e.Execute(ctx, core.ExecutionRequest{Address: address, Function: function, Args: opts.Args, CallContext: opts.CallCtx})
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
