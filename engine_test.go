package vm

import (
	"os"
	"path/filepath"
	"strings"
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

	engine := NewVMEngine(config)

	if engine == nil {
		t.Error("Expected VM engine to be created, got nil")
	}

	if engine.GetVersion() != "1.0.0" {
		t.Errorf("Expected version 1.0.0, got %s", engine.GetVersion())
	}

	cfg := engine.GetConfig()
	if cfg.MaxGasLimit != 1000000 {
		t.Errorf("Expected MaxGasLimit 1000000, got %d", cfg.MaxGasLimit)
	}
}

func TestCompile(t *testing.T) {
	dir := t.TempDir()
	config := VMConfig{
		MaxGasLimit:          1000000,
		EnableSecurityChecks: true,
		EnableGasMetering:    true,
		ExecutionTimeout:     time.Second * 30,
		ContractStorageDir:   dir,
	}

	engine := NewVMEngine(config)

	sourceCode := `
package main

func Hello() string {
	return "Hello, World!"
}
`
	compiledContract, err := engine.Compile(sourceCode)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if compiledContract == nil {
		t.Fatal("Expected CompiledContract to be created")
	}
	if compiledContract.ExecutablePath == "" {
		t.Fatal("Expected executable path to be generated")
	}
	if _, err := os.Stat(compiledContract.ExecutablePath); err != nil {
		t.Fatalf("Expected executable file to exist: %v", err)
	}

	_, err = engine.Compile("")
	if err == nil {
		t.Error("Expected error for empty source code, got nil")
	}

	unsafeCode := `
package main

import "unsafe"

func Hello() string {
	return "Hello, World!"
}
`
	_, err = engine.Compile(unsafeCode)
	if err == nil {
		t.Error("Expected error for unsafe import, got nil")
	}
}

func TestGenerateABI(t *testing.T) {
	config := VMConfig{
		MaxGasLimit:          1000000,
		EnableSecurityChecks: true,
		EnableGasMetering:    true,
		ExecutionTimeout:     time.Second * 30,
	}

	engine := NewVMEngine(config)

	sourceCode := `
package main

func Add(a, b int) int {
	return a + b
}

func GetBalance() int {
	return 1000
}
`
	contractABI, err := engine.GenerateABI(sourceCode)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if contractABI == nil {
		t.Fatal("Expected ABI to be generated")
	}
	if len(contractABI.Functions) != 2 {
		t.Errorf("Expected 2 functions, got %d", len(contractABI.Functions))
	}

	_, err = engine.GenerateABI("")
	if err == nil {
		t.Error("Expected error for empty source code, got nil")
	}

	unsafeCode := `
package main

import "unsafe"

func Hello() {}
`
	_, err = engine.GenerateABI(unsafeCode)
	if err == nil {
		t.Error("Expected error for unsafe import, got nil")
	}
}

func TestDeploy(t *testing.T) {
	dir := t.TempDir()
	config := VMConfig{
		MaxGasLimit:          1000000,
		EnableSecurityChecks: true,
		EnableGasMetering:    true,
		ExecutionTimeout:     time.Second * 30,
		ContractStorageDir:   dir,
	}

	engine := NewVMEngine(config)

	execPath := filepath.Join(dir, "test_exec")
	if err := os.WriteFile(execPath, []byte("#!/bin/sh\necho test"), 0755); err != nil {
		t.Fatalf("failed to write exec: %v", err)
	}

	placeholderABI := abiPlaceholder()
	contract := &CompiledContract{
		ExecutablePath: execPath,
		ABI:            placeholderABI,
		SourceHash:     "test_hash",
	}

	address, err := engine.Deploy(contract)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if address == "" {
		t.Error("Expected contract address to be generated")
	}

	_, err = engine.Deploy(nil)
	if err == nil {
		t.Error("Expected error for nil contract, got nil")
	}

	_, err = engine.Deploy(&CompiledContract{ExecutablePath: ""})
	if err == nil {
		t.Error("Expected error for empty executable path, got nil")
	}
}

func TestExecuteEndToEnd(t *testing.T) {
	dir := t.TempDir()
	config := VMConfig{
		MaxGasLimit:          1000000,
		EnableSecurityChecks: true,
		EnableGasMetering:    true,
		ExecutionTimeout:     time.Second * 30,
		ContractStorageDir:   dir,
	}

	engine := NewVMEngine(config)

	sourceCode := `
package main

func Add(a, b int) int {
	return a + b
}
`
	compiled, err := engine.Compile(sourceCode)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}

	address, err := engine.Deploy(compiled)
	if err != nil {
		t.Fatalf("deploy failed: %v", err)
	}

	result, err := engine.Execute(address, "Add", 10, 20)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if string(result) != "30" {
		t.Fatalf("expected result 30, got %s", string(result))
	}

	if engine.GetGasConsumed() < 10 {
		t.Fatalf("expected gas consumed >= 10, got %d", engine.GetGasConsumed())
	}

	_, err = engine.Execute("", "Add", 1, 2)
	if err == nil {
		t.Error("Expected error for empty contract address, got nil")
	}

	_, err = engine.Execute(address, "", 1, 2)
	if err == nil {
		t.Error("Expected error for empty function name, got nil")
	}

	_, err = engine.Execute("missing", "Add", 1, 2)
	if err == nil {
		t.Error("Expected error for missing contract, got nil")
	}
}

func TestContractManagement(t *testing.T) {
	dir := t.TempDir()
	config := VMConfig{
		MaxGasLimit:          1000000,
		EnableSecurityChecks: true,
		EnableGasMetering:    true,
		ExecutionTimeout:     time.Second * 30,
		ContractStorageDir:   dir,
	}

	engine := NewVMEngine(config)

	sourceCode := `
package main

func Add(a, b int) int {
	return a + b
}
`

	compiledContract, err := engine.Compile(sourceCode)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	address, err := engine.Deploy(compiledContract)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	retrievedContract, err := engine.GetContract(address)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if retrievedContract.Address != address {
		t.Errorf("Expected contract address %s, got %s", address, retrievedContract.Address)
	}

	retrievedABI, err := engine.GetContractABI(address)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if len(retrievedABI.Functions) != 1 {
		t.Errorf("Expected 1 function, got %d", len(retrievedABI.Functions))
	}
}

func TestGasMeteringOnExecute(t *testing.T) {
	dir := t.TempDir()
	config := VMConfig{
		MaxGasLimit:          100000,
		EnableSecurityChecks: true,
		EnableGasMetering:    true,
		ExecutionTimeout:     time.Second * 30,
		ContractStorageDir:   dir,
	}

	engine := NewVMEngine(config)
	if engine.GetGasConsumed() != 0 {
		t.Errorf("Expected initial gas consumed to be 0, got %d", engine.GetGasConsumed())
	}

	sourceCode := `
package main

func Add(a, b int) int {
	return a + b
}
`
	compiled, err := engine.Compile(sourceCode)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	address, err := engine.Deploy(compiled)
	if err != nil {
		t.Fatalf("deploy failed: %v", err)
	}

	_, err = engine.Execute(address, "Add", 1, 2)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if engine.GetGasConsumed() < 10 {
		t.Errorf("Expected gas consumed >= 10, got %d", engine.GetGasConsumed())
	}

	config.EnableGasMetering = false
	engineNoGas := NewVMEngine(config)
	if engineNoGas.GetGasConsumed() != 0 {
		t.Errorf("Expected gas consumed to be 0 when gas metering is disabled, got %d", engineNoGas.GetGasConsumed())
	}
}

func TestStop(t *testing.T) {
	engine := NewVMEngine(VMConfig{})
	if err := engine.Stop(); err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

func TestSecurityReview(t *testing.T) {
	dir := t.TempDir()
	config := VMConfig{
		MaxGasLimit:          1000000,
		EnableSecurityChecks: true,
		EnableGasMetering:    true,
		ExecutionTimeout:     time.Second * 30,
		ContractStorageDir:   dir,
	}

	engine := NewVMEngine(config)

	validCode := `
package main

import "fmt"

func Hello() {
	fmt.Println("Hello, World!")
}

func Add(a, b int) int {
	return a + b
}
`
	_, err := engine.Compile(validCode)
	if err != nil {
		t.Fatalf("Expected no error for valid code, got %v", err)
	}

	unsafeCode := `
package main

import "unsafe"

func Hello() {}
`
	_, err = engine.Compile(unsafeCode)
	if err == nil {
		t.Error("Expected error for unsafe import, got nil")
	}

	disallowedCode := `
package main

import "os"

func Hello() {}
`
	_, err = engine.Compile(disallowedCode)
	if err == nil {
		t.Error("Expected error for disallowed import, got nil")
	}

	withMain := `
package main

func main() {}

func Add(a, b int) int { return a + b }
`
	_, err = engine.Compile(withMain)
	if err == nil || !strings.Contains(err.Error(), "main function") {
		t.Fatalf("Expected main function error, got %v", err)
	}
}

func TestExecuteGasLimitExceeded(t *testing.T) {
	dir := t.TempDir()
	engine := NewVMEngine(VMConfig{
		MaxGasLimit:          25,
		EnableSecurityChecks: true,
		EnableGasMetering:    true,
		ExecutionTimeout:     time.Second * 10,
		ContractStorageDir:   dir,
	})

	source := `
package main

func Loop() int {
	sum := 0
	for i := 0; i < 1000; i++ {
		sum = sum + i
	}
	return sum
}
`
	compiled, err := engine.Compile(source)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	address, err := engine.Deploy(compiled)
	if err != nil {
		t.Fatalf("deploy failed: %v", err)
	}

	_, err = engine.Execute(address, "Loop")
	if err == nil {
		t.Fatal("expected gas limit exceeded")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "gas") {
		t.Fatalf("expected gas error, got %v", err)
	}
}

func TestExecuteTimeout(t *testing.T) {
	dir := t.TempDir()
	engine := NewVMEngine(VMConfig{
		MaxGasLimit:          1000000,
		EnableSecurityChecks: true,
		EnableGasMetering:    true,
		ExecutionTimeout:     200 * time.Millisecond,
		ContractStorageDir:   dir,
	})

	source := `
package main

import "time"

func Hang() {
	time.Sleep(5 * time.Second)
}
`
	compiled, err := engine.Compile(source)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	address, err := engine.Deploy(compiled)
	if err != nil {
		t.Fatalf("deploy failed: %v", err)
	}

	_, err = engine.Execute(address, "Hang")
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestExecuteUnknownFunctionAndBadArgs(t *testing.T) {
	dir := t.TempDir()
	engine := NewVMEngine(VMConfig{
		MaxGasLimit:          1000000,
		EnableSecurityChecks: true,
		EnableGasMetering:    true,
		ExecutionTimeout:     time.Second * 10,
		ContractStorageDir:   dir,
	})

	source := `
package main

func Add(a, b int) int {
	return a + b
}
`
	compiled, err := engine.Compile(source)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	address, err := engine.Deploy(compiled)
	if err != nil {
		t.Fatalf("deploy failed: %v", err)
	}

	_, err = engine.Execute(address, "NoSuch")
	if err == nil || !strings.Contains(err.Error(), "unknown function") {
		t.Fatalf("expected unknown function error, got %v", err)
	}

	_, err = engine.Execute(address, "Add", "not-int", 2)
	if err == nil {
		t.Fatal("expected invalid arg error")
	}
}

func TestExecuteMultipleReturnValues(t *testing.T) {
	dir := t.TempDir()
	engine := NewVMEngine(VMConfig{
		MaxGasLimit:          1000000,
		EnableSecurityChecks: true,
		EnableGasMetering:    true,
		ExecutionTimeout:     time.Second * 10,
		ContractStorageDir:   dir,
	})

	source := `
package main

func GetUserDetails(id int) (string, int, bool) {
	return "User", id * 100, true
}
`
	compiled, err := engine.Compile(source)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	address, err := engine.Deploy(compiled)
	if err != nil {
		t.Fatalf("deploy failed: %v", err)
	}

	result, err := engine.Execute(address, "GetUserDetails", 7)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if !strings.Contains(string(result), "User") || !strings.Contains(string(result), "700") {
		t.Fatalf("unexpected result: %s", string(result))
	}
}

func TestExecuteWithHostContext(t *testing.T) {
	dir := t.TempDir()
	engine := NewVMEngine(VMConfig{
		MaxGasLimit:          1000000,
		EnableSecurityChecks: true,
		EnableGasMetering:    true,
		ExecutionTimeout:     time.Second * 30,
		ContractStorageDir:   dir,
	})

	source := `
package main

import "github.com/lengzhao/vm/contractapi"

func Info() (uint64, string, string) {
	contractapi.Log("InfoCalled", "sender", contractapi.Sender())
	return contractapi.BlockHeight(), contractapi.Sender(), contractapi.ContractAddress()
}
`
	compiled, err := engine.Compile(source)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	address, err := engine.Deploy(compiled)
	if err != nil {
		t.Fatalf("deploy failed: %v", err)
	}

	host := &MemoryHost{
		Height:   0, // 零值也应注入
		Time:     1700000001,
		From:     "alice",
		Contract: Address(address),
	}
	result, err := engine.ExecuteWithContext(address, "Info", &CallContext{Host: host})
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if !strings.Contains(string(result.Data), "0") || !strings.Contains(string(result.Data), "alice") {
		t.Fatalf("unexpected data: %s", string(result.Data))
	}
	if len(result.Events) != 1 || result.Events[0].Name != "InfoCalled" {
		t.Fatalf("unexpected events: %+v", result.Events)
	}
	// 事件以 ExecuteResult 为准，不再回放到 Host.Log，避免重复
	if len(host.Events) != 0 {
		t.Fatalf("expected no host.Log replay, got %d", len(host.Events))
	}
	if len(engine.GetLastEvents()) != 1 {
		t.Fatalf("expected last events on engine")
	}
}

func TestEstimateGasStaticAndDryRun(t *testing.T) {
	dir := t.TempDir()
	engine := NewVMEngine(VMConfig{
		MaxGasLimit:          1000000,
		EnableSecurityChecks: true,
		EnableGasMetering:    true,
		ExecutionTimeout:     time.Second * 30,
		ContractStorageDir:   dir,
	})

	source := `
package main

import "github.com/lengzhao/vm/contractapi"

func Info() (uint64, string, string) {
	contractapi.Log("InfoCalled", "sender", contractapi.Sender())
	return contractapi.BlockHeight(), contractapi.Sender(), contractapi.ContractAddress()
}
`
	compiled, err := engine.Compile(source)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	address, err := engine.Deploy(compiled)
	if err != nil {
		t.Fatalf("deploy failed: %v", err)
	}

	static, err := engine.EstimateGas(address, "Info", nil)
	if err != nil {
		t.Fatalf("static EstimateGas: %v", err)
	}
	if static.Mode != "static" {
		t.Fatalf("static Mode=%q want static", static.Mode)
	}
	if !static.ProfileUsed {
		t.Fatal("static ProfileUsed want true")
	}
	if static.Estimated == 0 {
		t.Fatal("static Estimated want > 0")
	}

	host := &MemoryHost{
		Height:   42,
		Time:     1700000001,
		From:     "alice",
		Contract: Address(address),
	}
	dry, err := engine.EstimateGas(address, "Info", &EstimateOptions{
		DryRun:  true,
		CallCtx: &CallContext{Host: host},
	})
	if err != nil {
		t.Fatalf("dry-run EstimateGas: %v", err)
	}
	if dry.Mode != "dry_run" {
		t.Fatalf("dry Mode=%q want dry_run", dry.Mode)
	}
	if dry.Estimated == 0 {
		t.Fatal("dry Estimated want > 0")
	}
	if static.Estimated < dry.Estimated {
		t.Fatalf("static %d < dry %d", static.Estimated, dry.Estimated)
	}

	_, err = engine.EstimateGas(address, "NoSuch", nil)
	if err == nil {
		t.Fatal("expected error for unknown function")
	}
}

func TestEstimateGasMissingProfile(t *testing.T) {
	dir := t.TempDir()
	engine := NewVMEngine(VMConfig{
		MaxGasLimit:          1000000,
		EnableSecurityChecks: true,
		EnableGasMetering:    true,
		ExecutionTimeout:     time.Second * 10,
		ContractStorageDir:   dir,
	})

	_, err := engine.EstimateGas("deadbeef", "Info", nil)
	if err == nil {
		t.Fatal("expected error for missing gas profile")
	}
}

func TestCompileUsesCache(t *testing.T) {
	dir := t.TempDir()
	compiler := NewContractCompilerWithOptions(dir, true)
	source := `
package main
func Add(a, b int) int { return a + b }
`
	first, err := compiler.Compile(source)
	if err != nil {
		t.Fatalf("first compile: %v", err)
	}
	info1, err := os.Stat(first.ExecutablePath)
	if err != nil {
		t.Fatal(err)
	}

	second, err := compiler.Compile(source)
	if err != nil {
		t.Fatalf("second compile: %v", err)
	}
	if first.ExecutablePath != second.ExecutablePath {
		t.Fatalf("expected cached path, got %s vs %s", first.ExecutablePath, second.ExecutablePath)
	}
	info2, err := os.Stat(second.ExecutablePath)
	if err != nil {
		t.Fatal(err)
	}
	if !info1.ModTime().Equal(info2.ModTime()) {
		t.Fatal("expected executable mtime unchanged on cache hit")
	}
}

func abiPlaceholder() *abi.ABI {
	return &abi.ABI{
		PackageName: "test",
		Functions: []abi.Function{
			{Name: "TestFunc"},
		},
	}
}
