package vm

import (
	"testing"
	"time"
)

func TestNewContractCompiler(t *testing.T) {
	compiler := NewContractCompiler()

	if compiler == nil {
		t.Error("Expected ContractCompiler to be created, got nil")
	}
}

func TestCompilerCompile(t *testing.T) {
	compiler := NewContractCompiler()

	// 测试有效的源代码
	validCode := `
package main

func main() {
	println("Hello, World!")
}

func Add(a, b int) int {
	return a + b
}
`

	compiledContract, err := compiler.Compile(validCode)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if compiledContract == nil {
		t.Error("Expected CompiledContract to be created, got nil")
	}

	if compiledContract.ABI == nil {
		t.Error("Expected ABI to be generated")
	}

	if compiledContract.CompileTime.IsZero() {
		t.Error("Expected CompileTime to be set")
	}

	// 测试无效的源代码
	invalidCode := `
package main

import "unsafe"

func main() {
	println("Hello, World!")
}
`

	_, err = compiler.Compile(invalidCode)
	if err == nil {
		t.Error("Expected error for invalid code, got nil")
	}
}

func TestCompilerValidate(t *testing.T) {
	compiler := NewContractCompiler()

	// 测试有效的源代码
	validCode := `
package main

func main() {
	println("Hello, World!")
}
`

	err := compiler.Validate(validCode)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// 测试无效的源代码
	invalidCode := `
package main

import "os"

func main() {
	println("Hello, World!")
}
`

	err = compiler.Validate(invalidCode)
	if err == nil {
		t.Error("Expected error for invalid code, got nil")
	}
}

func TestCompiledContract(t *testing.T) {
	contract := &CompiledContract{
		ExecutablePath: "/path/to/executable",
		ABI:            nil,
		CompileTime:    time.Now(),
		SourceHash:     "hash123",
		Address:        "contract_address",
	}

	if contract.ExecutablePath != "/path/to/executable" {
		t.Errorf("Expected ExecutablePath '/path/to/executable', got '%s'", contract.ExecutablePath)
	}

	if contract.SourceHash != "hash123" {
		t.Errorf("Expected SourceHash 'hash123', got '%s'", contract.SourceHash)
	}

	if contract.Address != "contract_address" {
		t.Errorf("Expected Address 'contract_address', got '%s'", contract.Address)
	}
}
