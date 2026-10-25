// Package compiler registers the standalone Go compiler pipeline.
package compiler

import (
	"fmt"
	"github.com/lengzhao/pluginkit"
	"github.com/lengzhao/vm/core"
	implementation "github.com/lengzhao/vm/internal/compiler"
	"time"
)

type Config struct {
	OutputDir          string `json:"outputDir"`
	BuildTimeout       string `json:"buildTimeout"`
	RequireDeterminism bool   `json:"requireDeterminism"`
}
type Deps struct {
	Policy   core.SourcePolicy      `json:"policy"`
	ABI      core.ABIGenerator      `json:"abi"`
	Injector core.GasInjector       `json:"injector"`
	Profiler core.GasProfileBuilder `json:"profiler"`
}

func New(c Config, d Deps) (core.ContractCompiler, error) {
	var timeout time.Duration
	var err error
	if c.BuildTimeout != "" {
		timeout, err = time.ParseDuration(c.BuildTimeout)
		if err != nil {
			return nil, err
		}
		if timeout <= 0 {
			return nil, fmt.Errorf("buildTimeout must be positive")
		}
	}
	return implementation.New(implementation.Options{OutputDir: c.OutputDir, BuildTimeout: timeout, RequireDeterminism: c.RequireDeterminism}, implementation.Deps{Policy: d.Policy, ABI: d.ABI, Injector: d.Injector, Profiler: d.Profiler})
}
func init() { pluginkit.Register("compiler/go", New) }
