// Package compiler registers the standalone Go compiler pipeline.
package compiler

import (
	"fmt"
	"github.com/lengzhao/pluginkit"
	"github.com/lengzhao/vm/core"
	implementation "github.com/lengzhao/vm/internal/compiler"
	"github.com/lengzhao/vm/internal/sandbox"
	"time"
)

type SandboxConfig = sandbox.Config

type Config struct {
	Sandbox            *SandboxConfig `json:"sandbox"`
	GoBinary           string         `json:"goBinary"`
	GoVersion          string         `json:"goVersion"`
	OutputDir          string         `json:"outputDir"`
	BuildTimeout       string         `json:"buildTimeout"`
	RequireDeterminism bool           `json:"requireDeterminism"`
}
type Deps struct {
	Policy   core.SourcePolicy      `json:"policy"`
	ABI      core.ABIGenerator      `json:"abi"`
	Injector core.GasInjector       `json:"injector"`
	Profiler core.GasProfileBuilder `json:"profiler"`
}

func New(c Config, d Deps) (core.ContractCompiler, error)     { return newCompiler(c, d, false) }
func NewWASI(c Config, d Deps) (core.ContractCompiler, error) { return newCompiler(c, d, true) }
func newCompiler(c Config, d Deps, wasi bool) (core.ContractCompiler, error) {
	var docker *sandbox.Docker
	if c.Sandbox != nil {
		sc := *c.Sandbox
		if sc.MemoryMB == 0 {
			sc.MemoryMB = 1024
		}
		if sc.TmpMB == 0 {
			sc.TmpMB = 512
		}
		if sc.PIDs == 0 {
			sc.PIDs = 256
		}
		var err error
		docker, err = sandbox.New(sc)
		if err != nil {
			return nil, err
		}
		if c.BuildTimeout == "" {
			c.BuildTimeout = "2m"
		}
	}
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
	return implementation.New(implementation.Options{WASI: wasi, Sandbox: docker, GoBinary: c.GoBinary, GoVersion: c.GoVersion, OutputDir: c.OutputDir, BuildTimeout: timeout, RequireDeterminism: c.RequireDeterminism}, implementation.Deps{Policy: d.Policy, ABI: d.ABI, Injector: d.Injector, Profiler: d.Profiler})
}
func init() { pluginkit.Register("compiler/go", New); pluginkit.Register("compiler/wasip1", NewWASI) }
