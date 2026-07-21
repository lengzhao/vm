package vm

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/lengzhao/vm/abi"
)

func TestNewContractCompiler(t *testing.T) {
	compiler := NewContractCompiler()
	if compiler == nil {
		t.Error("Expected ContractCompiler to be created, got nil")
	}
}

func TestCompilerCompile(t *testing.T) {
	dir := t.TempDir()
	compiler := NewContractCompilerWithOptions(dir, true)

	validCode := `
package main

func Add(a, b int) int {
	return a + b
}
`

	compiledContract, err := compiler.Compile(validCode)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if compiledContract == nil {
		t.Fatal("Expected CompiledContract to be created, got nil")
	}
	if compiledContract.ABI == nil {
		t.Error("Expected ABI to be generated")
	}
	if compiledContract.CompileTime.IsZero() {
		t.Error("Expected CompileTime to be set")
	}
	if compiledContract.ExecutablePath == "" {
		t.Fatal("Expected ExecutablePath to be set")
	}
	if _, err := os.Stat(compiledContract.ExecutablePath); err != nil {
		t.Fatalf("Expected executable to exist: %v", err)
	}

	invalidCode := `
package main

import "unsafe"

func Add(a, b int) int {
	return a + b
}
`
	_, err = compiler.Compile(invalidCode)
	if err == nil {
		t.Error("Expected error for invalid code, got nil")
	}
}

func TestCompilerCompileExportedFunctions(t *testing.T) {
	dir := t.TempDir()
	compiler := NewContractCompilerWithOptions(dir, true)

	validCode := `
package main

import "fmt"

func Add(a, b int) int {
	return a + b
}

func GetBalance() int {
	return 1000
}

func Hello() {
	fmt.Println("hi")
}
`

	compiledContract, err := compiler.Compile(validCode)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if len(compiledContract.ABI.Functions) != 3 {
		t.Errorf("Expected 3 functions in ABI, got %d", len(compiledContract.ABI.Functions))
	}

	names := make(map[string]bool)
	for _, fn := range compiledContract.ABI.Functions {
		names[fn.Name] = true
	}
	for _, want := range []string{"Add", "GetBalance", "Hello"} {
		if !names[want] {
			t.Errorf("Expected %s function in ABI", want)
		}
	}
}

func TestCompilerCompileWithMain(t *testing.T) {
	compiler := NewContractCompilerWithOptions(t.TempDir(), true)

	invalidContractCode := `
package main

import "fmt"

func main() {
	fmt.Println("This should not be here")
}

func Add(a, b int) int {
	return a + b
}
`

	_, err := compiler.Compile(invalidContractCode)
	if err == nil {
		t.Fatal("Expected error for contract with main function, got nil")
	}
	if !strings.Contains(err.Error(), "main function") {
		t.Errorf("Expected error message about main function, got %v", err)
	}
}

func TestCompilerValidate(t *testing.T) {
	compiler := NewContractCompiler()

	validCode := `
package main

func Hello() {
	println("Hello, World!")
}
`
	if err := compiler.Validate(validCode); err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	invalidCode := `
package main

import "os"

func Hello() {
	println("Hello, World!")
}
`
	if err := compiler.Validate(invalidCode); err == nil {
		t.Error("Expected error for invalid code, got nil")
	}
}

func TestCompilerInjectGas(t *testing.T) {
	compiler := NewContractCompiler()

	sourceCode := `
package main

func Hello() {
	println("Hello, World!")
}

func Loop() {
	for i := 0; i < 3; i++ {
		println(i)
	}
}
`

	injectedCode, err := compiler.InjectGas(sourceCode)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if !strings.Contains(injectedCode, "__vmConsumeGas") {
		t.Fatal("Expected Gas consume injection")
	}
	if strings.Count(injectedCode, "__vmConsumeGas") < 3 {
		t.Fatalf("Expected consume points in functions and loop, got code:\n%s", injectedCode)
	}
}

func TestGenerateEntryUsesContractAPIGas(t *testing.T) {
	compiler := NewContractCompiler().(*ContractCompilerImpl)
	entry, err := compiler.generateEntryFile(&abi.ABI{})
	if err != nil {
		t.Fatalf("generateEntryFile: %v", err)
	}
	for _, want := range []string{
		"contractapi.InitGas",
		"contractapi.ConsumeGas",
		"contractapi.GasUsed()",
		"contractapi.ResetGas()",
		"contractapi.GasEntryBase",
	} {
		if !strings.Contains(entry, want) {
			t.Fatalf("entry missing %q, got:\n%s", want, entry)
		}
	}
	if strings.Contains(entry, "__vmGasConsumed +=") {
		t.Fatal("entry should not maintain local __vmGasConsumed counter")
	}
	if strings.Contains(entry, "var __vmGasConsumed") || strings.Contains(entry, "var __vmGasLimit") {
		t.Fatal("entry should not declare local __vmGasConsumed/__vmGasLimit")
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
