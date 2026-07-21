package vm

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lengzhao/vm/abi"
)

func TestNewContractManager(t *testing.T) {
	dir := t.TempDir()
	contractManager := NewContractManager(dir, NewSecurityReviewer(), NewABIGenerator())
	if contractManager == nil {
		t.Error("Expected ContractManager to be created, got nil")
	}
}

func TestDeployAndLoadContract(t *testing.T) {
	dir := t.TempDir()
	contractManager := NewContractManager(dir, NewSecurityReviewer(), NewABIGenerator())

	testABI := &abi.ABI{
		PackageName: "test",
		Functions: []abi.Function{
			{Name: "TestFunc"},
		},
	}

	contract := &CompiledContract{
		ABI:        testABI,
		CompileTime: time.Now(),
		SourceHash: "test_hash",
	}

	address, err := contractManager.Deploy(contract)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if address == "" {
		t.Fatal("Expected contract address to be generated")
	}

	loadedContract, err := contractManager.GetContract(address)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if loadedContract.Address != address {
		t.Errorf("Expected contract address %s, got %s", address, loadedContract.Address)
	}
	if loadedContract.SourceHash != "test_hash" {
		t.Errorf("Expected source hash 'test_hash', got %s", loadedContract.SourceHash)
	}
}

func TestGetContractABI(t *testing.T) {
	dir := t.TempDir()
	contractManager := NewContractManager(dir, NewSecurityReviewer(), NewABIGenerator())

	testABI := &abi.ABI{
		PackageName: "test",
		Functions: []abi.Function{
			{Name: "TestFunc"},
		},
	}

	contract := &CompiledContract{
		ABI:        testABI,
		CompileTime: time.Now(),
		SourceHash: "test_hash",
	}

	address, err := contractManager.Deploy(contract)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	loadedABI, err := contractManager.GetContractABI(address)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if loadedABI.PackageName != "test" {
		t.Errorf("Expected package name 'test', got %s", loadedABI.PackageName)
	}
	if len(loadedABI.Functions) != 1 {
		t.Errorf("Expected 1 function, got %d", len(loadedABI.Functions))
	}
}

func TestStoreAndLoadContract(t *testing.T) {
	dir := t.TempDir()
	contractManager := NewContractManager(dir, NewSecurityReviewer(), NewABIGenerator())

	testABI := &abi.ABI{
		PackageName: "test",
		Functions: []abi.Function{
			{Name: "TestFunc"},
		},
	}

	contract := &CompiledContract{
		ABI:        testABI,
		CompileTime: time.Now(),
		SourceHash: "test_hash",
	}

	address := "test_contract_address"
	if err := contractManager.StoreContract(contract, address); err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	loadedContract, err := contractManager.LoadContract(address)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if loadedContract.Address != address {
		t.Errorf("Expected contract address %s, got %s", address, loadedContract.Address)
	}
}

func TestLoadNonExistentContract(t *testing.T) {
	dir := t.TempDir()
	contractManager := NewContractManager(dir, NewSecurityReviewer(), NewABIGenerator())

	_, err := contractManager.LoadContract("nonexistent_contract")
	if err == nil {
		t.Error("Expected error for non-existent contract, got nil")
	}
}

func TestDeployRealArtifactAndExecute(t *testing.T) {
	buildDir := t.TempDir()
	storeDir := t.TempDir()

	compiler := NewContractCompilerWithOptions(buildDir, true)
	compiled, err := compiler.Compile(`
package main

func Add(a, b int) int {
	return a + b
}
`)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	if _, err := os.Stat(compiled.ExecutablePath); err != nil {
		t.Fatalf("executable missing: %v", err)
	}

	manager := NewContractManager(storeDir, NewSecurityReviewer(), NewABIGenerator())
	address, err := manager.Deploy(compiled)
	if err != nil {
		t.Fatalf("deploy failed: %v", err)
	}

	loaded, err := manager.GetContract(address)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if loaded.ExecutablePath == "" {
		t.Fatal("expected executable path after deploy")
	}
	if _, err := os.Stat(loaded.ExecutablePath); err != nil {
		t.Fatalf("deployed executable missing: %v", err)
	}
	if filepath.Dir(loaded.ExecutablePath) != filepath.Join(storeDir, address) {
		t.Fatalf("expected executable under contract dir, got %s", loaded.ExecutablePath)
	}

	runner := NewProcessRunner(5 * time.Second)
	result, err := runner.Run(context.Background(), loaded, CallRequest{
		Function: "Add",
		Args:     []interface{}{2, 5},
	})
	if err != nil {
		t.Fatalf("execute after deploy failed: %v", err)
	}
	if string(result.Data) != "7" {
		t.Fatalf("expected 7, got %s", string(result.Data))
	}
}
