// Package compat adapts legacy APIs without expanding plugin requirements.
package compat

import (
	"context"
	"github.com/lengzhao/vm/core"
)

type ContractCompiler interface {
	Compile(string) (*core.CompiledContract, error)
	Validate(string) error
	InjectGas(string) (string, error)
}
type Compiler struct {
	runtime  core.ContractCompiler
	policy   core.SourcePolicy
	injector core.GasInjector
}

func NewCompiler(c core.ContractCompiler, p core.SourcePolicy, i core.GasInjector) *Compiler {
	return &Compiler{runtime: c, policy: p, injector: i}
}
func (c *Compiler) Compile(s string) (*core.CompiledContract, error) {
	return c.runtime.Compile(context.Background(), s)
}
func (c *Compiler) CompileContext(ctx context.Context, s string) (*core.CompiledContract, error) {
	return c.runtime.Compile(ctx, s)
}
func (c *Compiler) Validate(s string) error            { return c.policy.Review(s) }
func (c *Compiler) InjectGas(s string) (string, error) { return c.injector.InjectGas(s) }

// SecurityReviewer retains allowlist queries only for callers of the legacy API.
type SecurityReviewer interface {
	core.SecurityReviewer
	IsKeywordAllowed(string) bool
	IsImportAllowed(string) bool
}
