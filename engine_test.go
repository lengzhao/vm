package vm

import (
	"os"
	"testing"
	"time"
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
	execPath, err := vm.Compile(sourceCode)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if execPath == "" {
		t.Error("Expected executable path to be generated")
	}

	// Test with empty source code
	_, err = vm.Compile("")
	if err == nil {
		t.Error("Expected error for empty source code, got nil")
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

	// Create a temporary executable file for testing
	tmpExecPath := "./test_contracts/test_exec"
	os.MkdirAll("./test_contracts", 0755)
	os.WriteFile(tmpExecPath, []byte("#!/bin/sh\necho test"), 0755)

	// Test with valid executable path
	address, err := vm.Deploy(tmpExecPath)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if address == "" {
		t.Error("Expected contract address to be generated")
	}

	// Test with non-existent executable path
	_, err = vm.Deploy("./nonexistent")
	if err == nil {
		t.Error("Expected error for non-existent executable, got nil")
	}

	// Clean up
	os.Remove(tmpExecPath)
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
