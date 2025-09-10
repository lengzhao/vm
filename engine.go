// Package vm implements a smart contract virtual machine that executes Go code
// with security restrictions and resource limitations.
package vm

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/lengzhao/vm/abi"
)

// VMEngine represents the virtual machine engine
type VMEngine struct {
	// Configuration for the VM
	config VMConfig

	// Version of the VM
	version string

	// Creation time
	createdAt time.Time

	// Security reviewer for contract code
	securityReviewer SecurityReviewer

	// ABI generator for contract code
	abiGenerator ABIGenerator

	// Gas metering for contract execution
	gasMetering GasMetering

	// Contract manager for contract lifecycle
	contractManager ContractManager
}

// VMConfig represents the configuration for the VM
type VMConfig struct {
	// Maximum gas limit for contract execution
	MaxGasLimit uint64

	// Enable or disable security checks
	EnableSecurityChecks bool

	// Enable or disable gas metering
	EnableGasMetering bool

	// Timeout for contract execution
	ExecutionTimeout time.Duration

	// Directory to store compiled contracts
	ContractStorageDir string
}

// ABIGenerator ABI生成模块接口
type ABIGenerator interface {
	// Generate 从源代码生成ABI
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
	// Create contract storage directory if it doesn't exist
	if config.ContractStorageDir == "" {
		config.ContractStorageDir = "./contracts"
	}

	if err := os.MkdirAll(config.ContractStorageDir, 0755); err != nil {
		fmt.Printf("Warning: failed to create contract storage directory: %v\n", err)
	}

	// Create gas metering instance
	gasMetering := NewGasMetering()
	if config.EnableGasMetering && config.MaxGasLimit > 0 {
		gasMetering.SetGasLimit(config.MaxGasLimit)
	}

	// Create security reviewer and ABI generator
	securityReviewer := NewSecurityReviewer()
	abiGenerator := NewABIGenerator()

	// Create contract manager
	contractManager := NewContractManager(config.ContractStorageDir, securityReviewer, abiGenerator)

	return &VMEngine{
		config:           config,
		version:          "1.0.0",
		createdAt:        time.Now(),
		securityReviewer: securityReviewer,
		abiGenerator:     abiGenerator,
		gasMetering:      gasMetering,
		contractManager:  contractManager,
	}
}

// Compile compiles the given source code into an executable file
func (vm *VMEngine) Compile(sourceCode string) (*CompiledContract, error) {
	// Perform security checks if enabled
	if vm.config.EnableSecurityChecks {
		if err := vm.securityReviewer.Review(sourceCode); err != nil {
			return nil, fmt.Errorf("security review failed: %w", err)
		}
	}

	// TODO: Implement source code compilation
	// This would involve:
	// 1. Parsing the source code
	// 2. Performing security checks
	// 3. Compiling to executable using TinyGo
	// 4. Storing the executable file locally

	if sourceCode == "" {
		return nil, fmt.Errorf("source code cannot be empty")
	}

	// Generate a hash of the source code for filename
	hash := sha256.Sum256([]byte(sourceCode))
	hashStr := hex.EncodeToString(hash[:])[:16]

	// Create a temporary file for the source code
	tmpDir := os.TempDir()
	sourceFile := filepath.Join(tmpDir, fmt.Sprintf("contract_%s.go", hashStr))
	execFile := filepath.Join(vm.config.ContractStorageDir, fmt.Sprintf("contract_%s", hashStr))

	// Write source code to temporary file
	if err := os.WriteFile(sourceFile, []byte(sourceCode), 0644); err != nil {
		return nil, fmt.Errorf("failed to write source file: %v", err)
	}

	// TODO: Use TinyGo to compile the source code
	// For now, we'll create a placeholder executable
	placeholderContent := fmt.Sprintf("#!/bin/sh\necho 'Executing contract: %s'\necho 'Source hash: %s'", sourceFile, hashStr)
	if err := os.WriteFile(execFile, []byte(placeholderContent), 0755); err != nil {
		return nil, fmt.Errorf("failed to create executable file: %v", err)
	}

	// Generate ABI
	contractABI, err := vm.abiGenerator.Generate(sourceCode)
	if err != nil {
		return nil, fmt.Errorf("failed to generate ABI: %w", err)
	}

	// Create compiled contract
	compiledContract := &CompiledContract{
		ExecutablePath: execFile,
		ABI:            contractABI,
		CompileTime:    time.Now(),
		SourceHash:     hashStr,
		Address:        "",
	}

	return compiledContract, nil
}

// GenerateABI generates the ABI for the given source code
func (vm *VMEngine) GenerateABI(sourceCode string) (*abi.ABI, error) {
	// Perform security checks if enabled
	if vm.config.EnableSecurityChecks {
		if err := vm.securityReviewer.Review(sourceCode); err != nil {
			return nil, fmt.Errorf("security review failed: %w", err)
		}
	}

	return vm.abiGenerator.Generate(sourceCode)
}

// Deploy deploys the compiled contract
func (vm *VMEngine) Deploy(contract *CompiledContract) (string, error) {
	// TODO: Implement contract deployment
	// This would involve:
	// 1. Verifying the executable file exists
	// 2. Generating a contract address
	// 3. Storing the executable path
	// 4. Initializing contract state

	if contract == nil {
		return "", fmt.Errorf("contract cannot be nil")
	}

	if contract.ExecutablePath == "" {
		return "", fmt.Errorf("executable path cannot be empty")
	}

	// Check if the executable file exists
	if _, err := os.Stat(contract.ExecutablePath); os.IsNotExist(err) {
		return "", fmt.Errorf("executable file does not exist: %s", contract.ExecutablePath)
	}

	// Use contract manager to deploy
	address, err := vm.contractManager.Deploy(contract)
	if err != nil {
		return "", fmt.Errorf("failed to deploy contract: %w", err)
	}

	// Update contract address
	contract.Address = address

	return address, nil
}

// Execute executes a function on the deployed contract
func (vm *VMEngine) Execute(contractAddress, function string, args ...interface{}) ([]byte, error) {
	// Reset gas metering for this execution
	if vm.config.EnableGasMetering {
		vm.gasMetering.Reset()
		if vm.config.MaxGasLimit > 0 {
			vm.gasMetering.SetGasLimit(vm.config.MaxGasLimit)
		}
	}

	// TODO: Implement contract execution
	// This would involve:
	// 1. Looking up the contract executable path
	// 2. Setting up the execution environment
	// 3. Running the executable with the given arguments
	// 4. Applying gas metering
	// 5. Returning the result

	if contractAddress == "" {
		return nil, fmt.Errorf("contract address cannot be empty")
	}

	if function == "" {
		return nil, fmt.Errorf("function name cannot be empty")
	}

	// TODO: Look up the executable path for this contract address
	// For now, we'll use a placeholder

	// Placeholder implementation
	result := []byte(fmt.Sprintf("executed %s on %s with args %v", function, contractAddress, args))

	// Consume some gas for the execution
	if vm.config.EnableGasMetering {
		// 消耗一些Gas用于执行
		vm.gasMetering.ConsumeGas(10)
	}

	return result, nil
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
	// TODO: Implement VM shutdown logic
	// This would involve:
	// 1. Stopping any running contracts
	// 2. Cleaning up resources
	// 3. Saving state if necessary

	return nil
}

// String returns a string representation of the VM engine
func (vm *VMEngine) String() string {
	return fmt.Sprintf("VMEngine{version: %s, createdAt: %s}", vm.version, vm.createdAt.Format(time.RFC3339))
}
