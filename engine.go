// Package vm implements a smart contract virtual machine that executes Go code
// with security restrictions and resource limitations.
package vm

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/lengzhao/vm/abi"
)

const defaultEntryGas uint64 = 10

// VMEngine represents the virtual machine engine
type VMEngine struct {
	config           VMConfig
	version          string
	createdAt        time.Time
	securityReviewer SecurityReviewer
	abiGenerator     ABIGenerator
	gasMetering      GasMetering
	contractManager  ContractManager
	compiler         ContractCompiler
	runner           Runner

	mu         sync.Mutex
	lastEvents []Event
}

// VMConfig represents the configuration for the VM
type VMConfig struct {
	MaxGasLimit          uint64
	EnableSecurityChecks bool
	EnableGasMetering    bool
	ExecutionTimeout     time.Duration
	ContractStorageDir   string
}

// ExecuteResult 是带事件的执行结果（推荐使用，结果自包含）
type ExecuteResult struct {
	Data        []byte
	Events      []Event
	GasConsumed uint64
}

// ABIGenerator ABI生成模块接口
type ABIGenerator interface {
	Generate(sourceCode string) (*abi.ABI, error)
}

// ABIGeneratorImpl ABI生成器实现
type ABIGeneratorImpl struct{}

// NewABIGenerator 创建新的ABI生成器实例
func NewABIGenerator() ABIGenerator {
	return &ABIGeneratorImpl{}
}

// Generate 从源代码生成ABI
func (a *ABIGeneratorImpl) Generate(sourceCode string) (*abi.ABI, error) {
	return abi.ExtractABI([]byte(sourceCode))
}

// NewVMEngine creates a new VM engine with the given configuration
func NewVMEngine(config VMConfig) *VMEngine {
	if config.ContractStorageDir == "" {
		config.ContractStorageDir = "./contracts"
	}
	if config.ExecutionTimeout <= 0 {
		config.ExecutionTimeout = 30 * time.Second
	}

	if err := os.MkdirAll(config.ContractStorageDir, 0755); err != nil {
		slog.Warn("failed to create contract storage directory", "dir", config.ContractStorageDir, "err", err)
	}

	gasMetering := NewGasMetering()
	if config.EnableGasMetering && config.MaxGasLimit > 0 {
		gasMetering.SetGasLimit(config.MaxGasLimit)
	}

	securityReviewer := NewSecurityReviewer()
	abiGenerator := NewABIGenerator()
	contractManager := NewContractManager(config.ContractStorageDir, securityReviewer, abiGenerator)
	compiler := NewContractCompilerWithOptions(config.ContractStorageDir, config.EnableSecurityChecks)
	runner := NewProcessRunner(config.ExecutionTimeout)

	return &VMEngine{
		config:           config,
		version:          "1.0.0",
		createdAt:        time.Now(),
		securityReviewer: securityReviewer,
		abiGenerator:     abiGenerator,
		gasMetering:      gasMetering,
		contractManager:  contractManager,
		compiler:         compiler,
		runner:           runner,
	}
}

// Compile compiles the given source code into an executable file
func (vm *VMEngine) Compile(sourceCode string) (*CompiledContract, error) {
	return vm.compiler.Compile(sourceCode)
}

// GenerateABI generates the ABI for the given source code
func (vm *VMEngine) GenerateABI(sourceCode string) (*abi.ABI, error) {
	if vm.config.EnableSecurityChecks {
		if err := vm.securityReviewer.Review(sourceCode); err != nil {
			return nil, fmt.Errorf("security review failed: %w", err)
		}
	}

	return vm.abiGenerator.Generate(sourceCode)
}

// Deploy deploys the compiled contract
func (vm *VMEngine) Deploy(contract *CompiledContract) (string, error) {
	if contract == nil {
		return "", fmt.Errorf("contract cannot be nil")
	}

	if contract.ExecutablePath == "" {
		return "", fmt.Errorf("executable path cannot be empty")
	}

	if _, err := os.Stat(contract.ExecutablePath); os.IsNotExist(err) {
		return "", fmt.Errorf("executable file does not exist: %s", contract.ExecutablePath)
	}

	address, err := vm.contractManager.Deploy(contract)
	if err != nil {
		return "", fmt.Errorf("failed to deploy contract: %w", err)
	}

	contract.Address = address
	return address, nil
}

// Execute executes a function on the deployed contract
func (vm *VMEngine) Execute(contractAddress, function string, args ...interface{}) ([]byte, error) {
	result, err := vm.ExecuteWithContext(contractAddress, function, nil, args...)
	if err != nil {
		return nil, err
	}
	return result.Data, nil
}

// ExecuteWithContext 在指定 Host 调用上下文中执行合约函数。
// 事件与 Gas 以返回的 ExecuteResult 为准；GetLastEvents/GetGasConsumed 仅为兼容保留。
func (vm *VMEngine) ExecuteWithContext(contractAddress, function string, callCtx *CallContext, args ...interface{}) (*ExecuteResult, error) {
	vm.mu.Lock()
	vm.lastEvents = nil
	vm.mu.Unlock()

	if contractAddress == "" {
		return nil, fmt.Errorf("contract address cannot be empty")
	}
	if function == "" {
		return nil, fmt.Errorf("function name cannot be empty")
	}

	if vm.config.EnableGasMetering {
		vm.gasMetering.Reset()
		if vm.config.MaxGasLimit > 0 {
			vm.gasMetering.SetGasLimit(vm.config.MaxGasLimit)
		}
	}

	contract, err := vm.contractManager.GetContract(contractAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to load contract: %w", err)
	}

	if callCtx == nil {
		callCtx = &CallContext{}
	}
	if callCtx.ContractAddress == nil {
		addr := Address(contractAddress)
		callCtx.ContractAddress = &addr
	}
	if callCtx.GasLimit == nil && vm.config.EnableGasMetering && vm.config.MaxGasLimit > 0 {
		limit := vm.config.MaxGasLimit
		callCtx.GasLimit = &limit
	}

	ctx := context.Background()
	if vm.config.ExecutionTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, vm.config.ExecutionTimeout)
		defer cancel()
	}
	if vm.config.EnableGasMetering && vm.config.MaxGasLimit > 0 {
		ctx = WithGasLimit(ctx, vm.config.MaxGasLimit)
	}
	ctx = WithCallContext(ctx, callCtx)

	result, err := vm.runner.Run(ctx, contract, CallRequest{
		Function: function,
		Args:     args,
	})
	if result != nil {
		vm.mu.Lock()
		vm.lastEvents = append([]Event(nil), result.Events...)
		vm.mu.Unlock()

		if vm.config.EnableGasMetering {
			consumed := result.GasConsumed
			if consumed == 0 {
				consumed = defaultEntryGas
			}
			if consumeErr := vm.gasMetering.ConsumeGas(consumed); consumeErr != nil {
				if err == nil {
					err = consumeErr
				}
			}
		}
	}
	if err != nil {
		return nil, err
	}

	return &ExecuteResult{
		Data:        result.Data,
		Events:      result.Events,
		GasConsumed: result.GasConsumed,
	}, nil
}

// GetLastEvents 返回最近一次执行产生的事件（兼容接口；优先使用 ExecuteResult.Events）
func (vm *VMEngine) GetLastEvents() []Event {
	vm.mu.Lock()
	defer vm.mu.Unlock()
	return append([]Event(nil), vm.lastEvents...)
}

// GetContract 获取合约
func (vm *VMEngine) GetContract(address string) (*CompiledContract, error) {
	return vm.contractManager.GetContract(address)
}

// GetContractABI 获取合约ABI
func (vm *VMEngine) GetContractABI(address string) (*abi.ABI, error) {
	return vm.contractManager.GetContractABI(address)
}

// GetVersion returns the version of the VM
func (vm *VMEngine) GetVersion() string {
	return vm.version
}

// GetConfig returns the configuration of the VM
func (vm *VMEngine) GetConfig() VMConfig {
	return vm.config
}

// GetGasConsumed returns the amount of gas consumed in the last execution
func (vm *VMEngine) GetGasConsumed() uint64 {
	return vm.gasMetering.GetConsumedGas()
}

// Stop stops the VM engine
func (vm *VMEngine) Stop() error {
	return nil
}

// String returns a string representation of the VM engine
func (vm *VMEngine) String() string {
	return fmt.Sprintf("VMEngine{version: %s, createdAt: %s}", vm.version, vm.createdAt.Format(time.RFC3339))
}
