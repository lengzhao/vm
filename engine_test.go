package vm

import (
	"os"
	"testing"
	"time"

	"github.com/lengzhao/vm/abi"
)

func TestNewVMEngine(t *testing.T) {
	config := VMConfig{
		MaxGasLimit:          1000000,
		EnableSecurityChecks: true,
		EnableGasMetering:    true,
		ExecutionTimeout:     time.Second * 30,
	}

	vm := NewVMEngine(config)

	if vm == nil {
		t.Error("Expected VM engine to be created, got nil")
	}

	if vm.GetVersion() != "1.0.0" {
		t.Errorf("Expected version 1.0.0, got %s", vm.GetVersion())
	}

	cfg := vm.GetConfig()
	if cfg.MaxGasLimit != 1000000 {
		t.Errorf("Expected MaxGasLimit 1000000, got %d", cfg.MaxGasLimit)
	}
}

func TestCompile(t *testing.T) {
	config := VMConfig{
		MaxGasLimit:          1000000,
		EnableSecurityChecks: true,
		EnableGasMetering:    true,
		ExecutionTimeout:     time.Second * 30,
		ContractStorageDir:   "./test_contracts",
	}

	vm := NewVMEngine(config)

	// Test with valid source code
	sourceCode := "package main\n\nfunc main() {\n\tprintln(\"Hello, World!\")\n}"
	compiledContract, err := vm.Compile(sourceCode)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if compiledContract == nil {
		t.Error("Expected CompiledContract to be created")
	}

	if compiledContract.ExecutablePath == "" {
		t.Error("Expected executable path to be generated")
	}

	// Test with empty source code
	_, err = vm.Compile("")
	if err == nil {
		t.Error("Expected error for empty source code, got nil")
	}

	// Test with invalid source code (unsafe import)
	unsafeCode := "package main\n\nimport \"unsafe\"\n\nfunc main() {\n\tprintln(\"Hello, World!\")\n}"
	_, err = vm.Compile(unsafeCode)
	if err == nil {
		t.Error("Expected error for unsafe import, got nil")
	}

	// Clean up
	os.RemoveAll("./test_contracts")
}

func TestGenerateABI(t *testing.T) {
	config := VMConfig{
		MaxGasLimit:          1000000,
		EnableSecurityChecks: true,
		EnableGasMetering:    true,
		ExecutionTimeout:     time.Second * 30,
	}

	vm := NewVMEngine(config)

	// Test with valid source code
	sourceCode := `
package main

func main() {
	println("Hello, World!")
}

func Add(a, b int) int {
	return a + b
}

func GetBalance() int {
	return 1000
}
`
	abi, err := vm.GenerateABI(sourceCode)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if abi == nil {
		t.Error("Expected ABI to be generated")
	}

	if len(abi.Functions) != 2 {
		t.Errorf("Expected 2 functions, got %d", len(abi.Functions))
	}

	// Test with empty source code
	_, err = vm.GenerateABI("")
	if err == nil {
		t.Error("Expected error for empty source code, got nil")
	}

	// Test with invalid source code (unsafe import)
	unsafeCode := "package main\n\nimport \"unsafe\"\n\nfunc main() {\n\tprintln(\"Hello, World!\")\n}"
	_, err = vm.GenerateABI(unsafeCode)
	if err == nil {
		t.Error("Expected error for unsafe import, got nil")
	}
}

func TestDeploy(t *testing.T) {
	config := VMConfig{
		MaxGasLimit:          1000000,
		EnableSecurityChecks: true,
		EnableGasMetering:    true,
		ExecutionTimeout:     time.Second * 30,
		ContractStorageDir:   "./test_contracts",
	}

	vm := NewVMEngine(config)

	// Create a test compiled contract
	testABI := &abi.ABI{
		PackageName: "test",
		Functions: []abi.Function{
			{Name: "TestFunc", Inputs: nil, Outputs: nil},
		},
	}

	contract := &CompiledContract{
		ExecutablePath: "./test_contracts/test_exec",
		ABI:            testABI,
		SourceHash:     "test_hash",
	}

	// Create a temporary executable file for testing
	os.MkdirAll("./test_contracts", 0755)
	os.WriteFile(contract.ExecutablePath, []byte("#!/bin/sh\necho test"), 0755)

	// Test with valid contract
	address, err := vm.Deploy(contract)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if address == "" {
		t.Error("Expected contract address to be generated")
	}

	// Test with nil contract
	_, err = vm.Deploy(nil)
	if err == nil {
		t.Error("Expected error for nil contract, got nil")
	}

	// Test with contract with empty executable path
	emptyContract := &CompiledContract{
		ExecutablePath: "",
		ABI:            testABI,
	}
	_, err = vm.Deploy(emptyContract)
	if err == nil {
		t.Error("Expected error for empty executable path, got nil")
	}

	// Clean up
	os.RemoveAll("./test_contracts")
}

func TestExecute(t *testing.T) {
	config := VMConfig{
		MaxGasLimit:          1000000,
		EnableSecurityChecks: true,
		EnableGasMetering:    true,
		ExecutionTimeout:     time.Second * 30,
	}

	vm := NewVMEngine(config)

	// Test with valid parameters
	result, err := vm.Execute("contract_123", "testFunction", "arg1", 42)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if len(result) == 0 {
		t.Error("Expected result to be returned")
	}

	// Test with empty contract address
	_, err = vm.Execute("", "testFunction", "arg1")
	if err == nil {
		t.Error("Expected error for empty contract address, got nil")
	}

	// Test with empty function name
	_, err = vm.Execute("contract_123", "", "arg1")
	if err == nil {
		t.Error("Expected error for empty function name, got nil")
	}
}

func TestContractManagement(t *testing.T) {
	config := VMConfig{
		MaxGasLimit:          1000000,
		EnableSecurityChecks: true,
		EnableGasMetering:    true,
		ExecutionTimeout:     time.Second * 30,
		ContractStorageDir:   "./test_contracts",
	}

	vm := NewVMEngine(config)

	// Compile a contract
	sourceCode := `
package main

func main() {
	println("Hello, World!")
}

func Add(a, b int) int {
	return a + b
}
`

	compiledContract, err := vm.Compile(sourceCode)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Deploy the contract
	address, err := vm.Deploy(compiledContract)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if address == "" {
		t.Error("Expected contract address to be generated")
	}

	// Get the contract
	retrievedContract, err := vm.GetContract(address)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if retrievedContract == nil {
		t.Error("Expected contract to be retrieved")
	}

	if retrievedContract.Address != address {
		t.Errorf("Expected contract address %s, got %s", address, retrievedContract.Address)
	}

	// Get the contract ABI
	retrievedABI, err := vm.GetContractABI(address)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if retrievedABI == nil {
		t.Error("Expected ABI to be retrieved")
	}

	if len(retrievedABI.Functions) != 1 {
		t.Errorf("Expected 1 function, got %d", len(retrievedABI.Functions))
	}

	// Clean up
	os.RemoveAll("./test_contracts")
}

func TestGasMetering(t *testing.T) {
	config := VMConfig{
		MaxGasLimit:          100,
		EnableSecurityChecks: true,
		EnableGasMetering:    true,
		ExecutionTimeout:     time.Second * 30,
	}

	vm := NewVMEngine(config)

	// 检查初始Gas消耗
	if vm.GetGasConsumed() != 0 {
		t.Errorf("Expected initial gas consumed to be 0, got %d", vm.GetGasConsumed())
	}

	// 执行一个函数
	_, err := vm.Execute("contract_123", "testFunction", "arg1", 42)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// 检查Gas消耗
	if vm.GetGasConsumed() != 10 {
		t.Errorf("Expected gas consumed to be 10, got %d", vm.GetGasConsumed())
	}

	// 测试禁用Gas计费的情况
	config.EnableGasMetering = false
	vmNoGas := NewVMEngine(config)

	if vmNoGas.GetGasConsumed() != 0 {
		t.Errorf("Expected gas consumed to be 0 when gas metering is disabled, got %d", vmNoGas.GetGasConsumed())
	}
}

func TestStop(t *testing.T) {
	config := VMConfig{
		MaxGasLimit:          1000000,
		EnableSecurityChecks: true,
		EnableGasMetering:    true,
		ExecutionTimeout:     time.Second * 30,
	}

	vm := NewVMEngine(config)

	// Test stopping the VM
	err := vm.Stop()

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

func TestSecurityReview(t *testing.T) {
	config := VMConfig{
		MaxGasLimit:          1000000,
		EnableSecurityChecks: true,
		EnableGasMetering:    true,
		ExecutionTimeout:     time.Second * 30,
		ContractStorageDir:   "./test_contracts",
	}

	vm := NewVMEngine(config)

	// Test valid code
	validCode := `
package main

import "fmt"

func main() {
	fmt.Println("Hello, World!")
}

func Add(a, b int) int {
	return a + b
}
`
	_, err := vm.Compile(validCode)
	if err != nil {
		t.Errorf("Expected no error for valid code, got %v", err)
	}

	// Test invalid code with unsafe import
	unsafeCode := `
package main

import "unsafe"

func main() {
	fmt.Println("Hello, World!")
}
`
	_, err = vm.Compile(unsafeCode)
	if err == nil {
		t.Error("Expected error for unsafe import, got nil")
	}

	// Test invalid code with disallowed import
	disallowedCode := `
package main

import "os"

func main() {
	fmt.Println("Hello, World!")
}
`
	_, err = vm.Compile(disallowedCode)
	if err == nil {
		t.Error("Expected error for disallowed import, got nil")
	}

	// Clean up
	os.RemoveAll("./test_contracts")
}
