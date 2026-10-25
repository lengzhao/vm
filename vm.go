// Package vm is the compatibility API. New plugins use only core contracts.
package vm

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/lengzhao/pluginkit/build"
	"github.com/lengzhao/vm/abi"
	"github.com/lengzhao/vm/assembly"
	"github.com/lengzhao/vm/core"
	"github.com/lengzhao/vm/internal/abigen"
	"github.com/lengzhao/vm/internal/callcontext"
	"github.com/lengzhao/vm/internal/compat"
	"github.com/lengzhao/vm/internal/deployment"
	"github.com/lengzhao/vm/internal/gas"
	"github.com/lengzhao/vm/internal/policy"
	"github.com/lengzhao/vm/internal/process"
	"github.com/lengzhao/vm/internal/security"
	_ "github.com/lengzhao/vm/plugins/all"
	storageplugin "github.com/lengzhao/vm/plugins/storage"
)

type ContractCompilerImpl = compat.Compiler
type ContractManagerImpl = compat.Manager
type SecurityReviewerImpl = security.Reviewer
type ABIGeneratorImpl = abigen.Generator
type GasMeteringImpl = gas.Meter
type ProcessRunner = process.ProcessRunner

const DeterministicPolicyVersion = policy.DeterministicPolicyVersion

// VMEngine keeps legacy signatures, diagnostics and last-result state outside
// the plugin contract. Engine exposes the context-aware business facade.
type VMEngine struct {
	Engine      core.Engine
	runtime     *assembly.Runtime
	config      VMConfig
	createdAt   time.Time
	mu          sync.Mutex
	lastEvents  []Event
	gasConsumed uint64
}

func normalize(c VMConfig) VMConfig {
	if c.ContractStorageDir == "" {
		c.ContractStorageDir = "./contracts"
	}
	if c.ExecutionTimeout <= 0 {
		c.ExecutionTimeout = 30 * time.Second
	}
	return c
}
func wrap(rt *assembly.Runtime, c VMConfig) *VMEngine {
	return &VMEngine{Engine: rt.Engine, runtime: rt, config: normalize(c), createdAt: time.Now()}
}
func OpenVMEngine(ctx context.Context, c VMConfig) (*VMEngine, error) {
	cfg := assembly.DefaultConfig(c)
	// Preserve the original development facade's permissive policy acceptance.
	if !c.RequireDeterminism {
		cfg.Plugins["validator"].(map[string]any)["config"].(map[string]any)["allowAnyPolicy"] = true
	}
	rt, err := assembly.Build(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return wrap(rt, c), nil
}
func NewVMEngine(c VMConfig) *VMEngine {
	e, err := OpenVMEngine(context.Background(), c)
	if err != nil {
		panic(err)
	}
	return e
}
func NewVMEngineWithRunner(c VMConfig, r Runner) *VMEngine {
	rt, err := assembly.BuildDefaultWithRunner(context.Background(), c, r)
	if err != nil {
		panic(err)
	}
	return wrap(rt, c)
}
func (e *VMEngine) Stop() error { return e.runtime.Stop(context.Background()) }
func (e *VMEngine) Compile(source string) (*CompiledContract, error) {
	return e.Engine.Compile(context.Background(), source)
}
func (e *VMEngine) CompileContext(ctx context.Context, source string) (*CompiledContract, error) {
	return e.Engine.Compile(ctx, source)
}
func (e *VMEngine) GenerateABI(source string) (*abi.ABI, error) {
	return e.Engine.GenerateABI(context.Background(), source)
}
func (e *VMEngine) Deploy(c *CompiledContract) (string, error) {
	return e.DeployWithInput(c, compat.LocalDeploymentInput())
}
func (e *VMEngine) DeployWithInput(c *CompiledContract, input DeploymentInput) (string, error) {
	return e.DeployContext(context.Background(), c, input)
}
func (e *VMEngine) DeployContext(ctx context.Context, c *CompiledContract, input DeploymentInput) (string, error) {
	address, err := e.Engine.Deploy(ctx, core.DeploymentRequest{Contract: c, Input: input})
	if err != nil {
		return "", err
	}
	loaded, err := e.Engine.Get(ctx, address)
	if err != nil {
		return "", err
	}
	c.Address = address
	c.ExecutablePath = loaded.ExecutablePath
	c.Deployment = loaded.Deployment
	return address, nil
}

// DeriveDeploymentAddress predicts the versioned address without accessing storage.
func DeriveDeploymentAddress(input DeploymentInput, manifestSHA256 string) (string, error) {
	return deployment.DeriveAddress(input, manifestSHA256)
}

func (e *VMEngine) Execute(address, function string, args ...interface{}) ([]byte, error) {
	out, err := e.ExecuteWithContext(address, function, nil, args...)
	if err != nil {
		return nil, err
	}
	return out.Data, nil
}
func (e *VMEngine) ExecuteWithContext(address, function string, cc *CallContext, args ...interface{}) (*ExecuteResult, error) {
	return e.ExecuteContext(context.Background(), address, function, cc, args...)
}
func (e *VMEngine) ExecuteContext(ctx context.Context, address, function string, cc *CallContext, args ...interface{}) (*ExecuteResult, error) {
	out, err := e.Engine.Execute(ctx, core.ExecutionRequest{Address: address, Function: function, Args: args, CallContext: cc})
	e.mu.Lock()
	e.lastEvents = nil
	e.gasConsumed = 0
	if out != nil {
		e.lastEvents = append([]Event(nil), out.Events...)
		if e.config.EnableGasMetering {
			var executionErr *ExecutionError
			if !errors.As(err, &executionErr) || executionErr.Code != CodeGasLimitExceeded {
				e.gasConsumed = out.GasConsumed
				if e.gasConsumed == 0 {
					e.gasConsumed = 10
				}
			}
		}
	}
	e.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return out, nil
}
func (e *VMEngine) EstimateGas(a, f string, o *EstimateOptions) (*GasEstimate, error) {
	if o != nil && o.DryRun {
		out, err := e.ExecuteWithContext(a, f, o.CallCtx, o.Args...)
		if err != nil {
			return nil, err
		}
		return &GasEstimate{Estimated: out.GasConsumed, Mode: "dry_run"}, nil
	}
	return e.Engine.EstimateGas(context.Background(), a, f, o)
}
func (e *VMEngine) GetContract(a string) (*CompiledContract, error) {
	return e.Engine.Get(context.Background(), a)
}
func (e *VMEngine) GetContractABI(a string) (*abi.ABI, error) {
	c, err := e.GetContract(a)
	if err != nil {
		return nil, err
	}
	return c.ABI, nil
}
func (e *VMEngine) GetLastEvents() []Event {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]Event(nil), e.lastEvents...)
}
func (e *VMEngine) GetGasConsumed() uint64 { e.mu.Lock(); defer e.mu.Unlock(); return e.gasConsumed }
func (e *VMEngine) GetVersion() string     { return "1.0.0" }
func (e *VMEngine) GetConfig() VMConfig    { return e.config }
func (e *VMEngine) String() string {
	return fmt.Sprintf("VMEngine{version: %s, createdAt: %s}", e.GetVersion(), e.createdAt.Format(time.RFC3339))
}

func NewContractCompiler() ContractCompiler { return NewContractCompilerWithPolicy("", true, false) }
func NewContractCompilerWithOptions(dir string, secure bool) ContractCompiler {
	return NewContractCompilerWithPolicy(dir, secure, false)
}
func NewContractCompilerWithPolicy(dir string, secure, deterministic bool) ContractCompiler {
	cfg := assembly.DefaultConfig(VMConfig{EnableSecurityChecks: secure, RequireDeterminism: deterministic})
	cfg.Plugins["compiler"].(map[string]any)["config"].(map[string]any)["outputDir"] = dir
	c, result, err := build.Build[core.ContractCompiler](context.Background(), cfg.Plugins, "compiler")
	if err != nil {
		panic(err)
	}
	p, err := build.RequireByID[core.SourcePolicy](result, "compiler.policy", "policy")
	if err != nil {
		panic(err)
	}
	i, err := build.RequireByID[core.GasInjector](result, "compiler.injector", "injector")
	if err != nil {
		panic(err)
	}
	return compat.NewCompiler(c, p, i)
}
func NewSecurityReviewer() SecurityReviewer         { return security.New() }
func NewABIGenerator() ABIGenerator                 { return abigen.New() }
func NewGasMetering() GasMetering                   { return gas.NewMeter() }
func NewProcessRunner(timeout time.Duration) Runner { return process.New(timeout) }
func NewContractManager(dir string, _ SecurityReviewer, _ ABIGenerator) ContractManager {
	s, err := storageplugin.New(storageplugin.Config{Directory: dir})
	if err != nil {
		panic(err)
	}
	d, err := deployment.New(s.(core.ContractCreator), compat.AcceptArtifacts{})
	if err != nil {
		panic(err)
	}
	return compat.NewManager(s, d)
}
func ReviewDeterministic(source string) error {
	p, err := policy.NewSource(policy.Options{Deterministic: true}, security.New())
	if err != nil {
		return err
	}
	return p.Review(source)
}
func BuildGasProfile(s string) (*GasProfile, error) { return gas.BuildGasProfile(s) }
func EstimateFromProfile(p *GasProfile, f string, b uint64) (uint64, map[string]uint64, error) {
	// Older callers built profiles without a version; normalization belongs here.
	if p != nil && p.Version == 0 {
		copy := *p
		copy.Version = 1
		p = &copy
	}
	return gas.EstimateFromProfile(p, f, b)
}
func WithGasLimit(ctx context.Context, l uint64) context.Context {
	return callcontext.WithGasLimit(ctx, l)
}
func WithCallContext(ctx context.Context, c *CallContext) context.Context {
	return callcontext.WithCallContext(ctx, c)
}
func CallContextFrom(ctx context.Context) (*CallContext, bool) {
	return callcontext.CallContextFrom(ctx)
}
