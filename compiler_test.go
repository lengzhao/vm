package vm

import (
	"strings"
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

	// 测试有效的源代码（不包含main函数）
	validCode := `
package main

// Add 两个数相加
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

// Add 两个数相加
func Add(a, b int) int {
	return a + b
}
`

	_, err = compiler.Compile(invalidCode)
	if err == nil {
		t.Error("Expected error for invalid code, got nil")
	}
}

func TestCompilerCompileWithTinyGo(t *testing.T) {
	compiler := NewContractCompiler()

	// 测试有效的源代码（不包含main函数）
	validCode := `
package main

import "fmt"

// Add 两个数相加
func Add(a, b int) int {
	return a + b
}

// GetBalance 获取余额
func GetBalance() int {
	return 1000
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

	// 验证ABI包含正确的函数
	if len(compiledContract.ABI.Functions) != 2 { // Add and GetBalance
		t.Errorf("Expected 2 functions in ABI, got %d", len(compiledContract.ABI.Functions))
	}

	// 验证函数名称
	functionNames := make(map[string]bool)
	for _, fn := range compiledContract.ABI.Functions {
		functionNames[fn.Name] = true
	}

	if !functionNames["Add"] {
		t.Error("Expected Add function in ABI")
	}

	if !functionNames["GetBalance"] {
		t.Error("Expected GetBalance function in ABI")
	}
}

func TestCompilerCompileWithoutMain(t *testing.T) {
	compiler := NewContractCompiler()

	// 测试不包含main函数的合约代码（这是正确的智能合约格式）
	contractCode := `
package main

import "fmt"

// Add 两个数相加
func Add(a, b int) int {
	return a + b
}

// GetBalance 获取余额
func GetBalance() int {
	return 1000
}
`

	compiledContract, err := compiler.Compile(contractCode)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if compiledContract == nil {
		t.Error("Expected CompiledContract to be created, got nil")
	}

	if compiledContract.ABI == nil {
		t.Error("Expected ABI to be generated")
	}

	// 验证ABI包含正确的函数
	if len(compiledContract.ABI.Functions) != 2 { // Add and GetBalance
		t.Errorf("Expected 2 functions in ABI, got %d", len(compiledContract.ABI.Functions))
	}

	// 验证函数名称
	functionNames := make(map[string]bool)
	for _, fn := range compiledContract.ABI.Functions {
		functionNames[fn.Name] = true
	}

	if !functionNames["Add"] {
		t.Error("Expected Add function in ABI")
	}

	if !functionNames["GetBalance"] {
		t.Error("Expected GetBalance function in ABI")
	}
}

func TestCompilerCompileWithMain(t *testing.T) {
	compiler := NewContractCompiler()

	// 测试包含main函数的合约代码（这应该是错误的）
	invalidContractCode := `
package main

import "fmt"

func main() {
	fmt.Println("This should not be here")
}

// Add 两个数相加
func Add(a, b int) int {
	return a + b
}
`

	_, err := compiler.Compile(invalidContractCode)
	if err == nil {
		t.Error("Expected error for contract with main function, got nil")
	}

	// 验证错误消息
	if !strings.Contains(err.Error(), "main function") {
		t.Errorf("Expected error message about main function, got %v", err)
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

func TestCompilerInjectGas(t *testing.T) {
	compiler := NewContractCompiler()

	// 测试Gas注入
	sourceCode := `
package main

func main() {
	println("Hello, World!")
}
`

	injectedCode, err := compiler.InjectGas(sourceCode)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if injectedCode == "" {
		t.Error("Expected injected code, got empty string")
	}

	// 检查是否包含Gas注入标记
	if !strings.Contains(injectedCode, "Gas-injected code") {
		t.Error("Expected Gas-injected code comment")
	}
}

// generateMainFunction 已被移除，相关测试也移除

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
