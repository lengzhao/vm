package assembly

import (
	"github.com/lengzhao/vm/core"
	"time"
)

// DefaultConfig returns a fresh, editable graph preserving legacy VM settings.
func DefaultConfig(c core.VMConfig) Config {
	if c.ContractStorageDir == "" {
		c.ContractStorageDir = "./contracts"
	}
	if c.ExecutionTimeout <= 0 {
		c.ExecutionTimeout = 30 * time.Second
	}
	node := func(use string, config, deps map[string]any) any {
		n := map[string]any{"use": use}
		if config != nil {
			n["config"] = config
		}
		if deps != nil {
			n["deps"] = deps
		}
		return n
	}
	return Config{Root: "engine", Plugins: map[string]any{
		"security":  node("security/go", nil, nil),
		"abi":       node("abi/go", nil, nil),
		"policy":    node("policy/go", map[string]any{"enableSecurity": c.EnableSecurityChecks, "deterministic": c.RequireDeterminism}, map[string]any{"reviewer": "security"}),
		"validator": node("policy/artifact", map[string]any{"requireDeterminism": c.RequireDeterminism}, map[string]any{"policy": "policy"}),
		"deployer":  node("deployment/default", nil, map[string]any{"store": "contracts", "validator": "validator"}),
		"injector":  node("gas/injector", nil, nil),
		"profiler":  node("gas/profiler", nil, nil),
		"meters":    node("gas/meters", nil, nil),
		"compiler":  node("compiler/go", map[string]any{"outputDir": c.ContractStorageDir, "requireDeterminism": c.RequireDeterminism}, map[string]any{"policy": "policy", "abi": "abi", "injector": "injector", "profiler": "profiler"}),
		"contracts": node("storage/filesystem", map[string]any{"directory": c.ContractStorageDir}, nil),
		"runner":    node("runner/process", map[string]any{"timeout": c.ExecutionTimeout.String()}, nil),
		"engine": node("engine/default", map[string]any{
			"maxGasLimit": c.MaxGasLimit, "enableGasMetering": c.EnableGasMetering, "executionTimeout": c.ExecutionTimeout.String(),
		}, map[string]any{"policy": "policy", "abi": "abi", "compiler": "compiler", "contracts": "contracts", "runner": "runner", "meters": "meters", "estimator": "profiler", "deployer": "deployer", "validator": "validator"}),
	}}
}
