package core

import (
	"context"
	"github.com/lengzhao/vm/abi"
)

// SourcePolicy is one versioned source-acceptance policy. Its ID identifies
// semantics, rather than encoding an implementation or a security guarantee.
type SourcePolicy interface {
	Review(string) error
	ID() string
}

// PolicyGuarantees records execution properties promised by a trusted policy.
// This declaration is not a proof; extension implementations remain trusted.
type PolicyGuarantees struct {
	Deterministic bool `json:"deterministic,omitempty"`
}
type PolicyGuarantor interface{ Guarantees() PolicyGuarantees }

func GuaranteesOf(policy SourcePolicy) PolicyGuarantees {
	if p, ok := policy.(PolicyGuarantor); ok {
		return p.Guarantees()
	}
	return PolicyGuarantees{}
}

// ArtifactValidator decides which compiled artifacts a runtime may accept.
type ArtifactValidator interface {
	ValidateArtifact(context.Context, *CompiledContract) error
}
type GasInjector interface{ InjectGas(string) (string, error) }
type GasProfileBuilder interface {
	BuildProfile(string) (*GasProfile, error)
}
type GasEstimator interface {
	Estimate(*GasProfile, string, uint64) (uint64, map[string]uint64, error)
}

// GasProfiler keeps profile production and interpretation in one coherent model.
// Consumers depend on the appropriate narrow view; one plugin can implement both.
type GasProfiler interface {
	GasProfileBuilder
	GasEstimator
}
type GasMeterFactory interface{ NewMeter() GasMetering }

// ExecutionRequest contains call data; cancellation belongs to context.Context.
type ExecutionRequest struct {
	Address     string
	Function    string
	Args        []any
	CallContext *CallContext
}
type Executor interface {
	Execute(context.Context, ExecutionRequest) (*ExecuteResult, error)
}
type ABIService interface {
	GenerateABI(context.Context, string) (*abi.ABI, error)
}
type GasEstimationService interface {
	EstimateGas(context.Context, string, string, *EstimateOptions) (*GasEstimate, error)
}

// Engine is an application facade composed from business capabilities. State,
// diagnostics and lifecycle are optional concerns, never plugin requirements.
type Engine interface {
	ContractCompiler
	ContractDeployer
	ContractReader
	Executor
	ABIService
	GasEstimationService
}
