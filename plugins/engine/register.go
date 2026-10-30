// Package engine registers the VM business facade with narrow dependencies.
package engine

import (
	"fmt"
	"github.com/lengzhao/pluginkit"
	"github.com/lengzhao/vm/core"
	impl "github.com/lengzhao/vm/internal/engine"
	"time"
)

type Config struct {
	MaxCallDepth      int    `json:"maxCallDepth"`
	MaxGasLimit       uint64 `json:"maxGasLimit"`
	EnableGasMetering bool   `json:"enableGasMetering"`
	ExecutionTimeout  string `json:"executionTimeout"`
}
type Deps struct {
	Policy    core.SourcePolicy      `json:"policy"`
	ABI       core.ABIGenerator      `json:"abi"`
	Compiler  core.ContractCompiler  `json:"compiler"`
	Deployer  core.ContractDeployer  `json:"deployer"`
	Contracts core.ContractReader    `json:"contracts"`
	Runner    core.Runner            `json:"runner"`
	Meters    core.GasMeterFactory   `json:"meters"`
	Estimator core.GasEstimator      `json:"estimator"`
	State     core.StateStore        `json:"state"`
	Validator core.ArtifactValidator `json:"validator"`
}

func NewComponents(_ struct{}, d Deps) (*Deps, error) { return &d, nil }
func New(c Config, d Deps) (core.Engine, error) {
	var timeout time.Duration
	var err error
	if c.ExecutionTimeout != "" {
		timeout, err = time.ParseDuration(c.ExecutionTimeout)
		if err != nil {
			return nil, err
		}
		if timeout <= 0 {
			return nil, fmt.Errorf("executionTimeout must be positive")
		}
	}
	return impl.New(impl.Options{MaxCallDepth: c.MaxCallDepth, MaxGasLimit: c.MaxGasLimit, EnableGasMetering: c.EnableGasMetering, ExecutionTimeout: timeout}, impl.Deps{State: d.State, Policy: d.Policy, ABI: d.ABI, Compiler: d.Compiler, Deployer: d.Deployer, Contracts: d.Contracts, Runner: d.Runner, Meters: d.Meters, Estimator: d.Estimator, Validator: d.Validator})
}

// NewWithConfig adapts the legacy configuration only at the compatibility boundary.
func NewWithConfig(c core.VMConfig, d Deps) (core.Engine, error) {
	return New(Config{MaxCallDepth: c.MaxCallDepth, MaxGasLimit: c.MaxGasLimit, EnableGasMetering: c.EnableGasMetering, ExecutionTimeout: normalizedTimeout(c.ExecutionTimeout).String()}, d)
}
func normalizedTimeout(d time.Duration) time.Duration {
	if d <= 0 {
		return 30 * time.Second
	}
	return d
}
func init() {
	pluginkit.Register("engine/default", New)
	pluginkit.Register("engine/components", NewComponents)
}
