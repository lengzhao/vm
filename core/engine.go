package core

import (
	"github.com/lengzhao/vm/abi"
	"time"
)

// VMConfig represents the configuration for the VM
type VMConfig struct {
	MaxGasLimit          uint64
	EnableSecurityChecks bool
	// RequireDeterminism enforces the versioned source profile, independently of EnableSecurityChecks.
	RequireDeterminism bool
	EnableGasMetering  bool
	ExecutionTimeout   time.Duration
	ContractStorageDir string
}

// ExecuteResult 是带事件的执行结果（推荐使用，结果自包含）
type ExecuteResult struct {
	Data        []byte
	Events      []Event
	GasConsumed uint64
}

// EstimateOptions 控制 EstimateGas 的估算方式。
// opts == nil 时等价于静态估算 + 默认 LoopBound。
type EstimateOptions struct {
	DryRun    bool
	LoopBound uint64
	Args      []any
	CallCtx   *CallContext
}

// GasEstimate 是 EstimateGas 的返回结果。
type GasEstimate struct {
	Estimated   uint64
	Mode        string // "static" | "dry_run"
	ProfileUsed bool
	Breakdown   map[string]uint64
}

// ABIGenerator ABI生成模块接口
type ABIGenerator interface {
	Generate(sourceCode string) (*abi.ABI, error)
}
