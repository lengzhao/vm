package vm

import (
	"context"
	"encoding/json"
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

func TestDeployPersistsGasProfile(t *testing.T) {
	buildDir := t.TempDir()
	storeDir := t.TempDir()

	source := `
package main

import "github.com/lengzhao/vm/contractapi"

func Add(a, b int) int {
	return a + b
}

func Info() string {
	contractapi.Log("info", "k", "v")
	return "ok"
}
`
	compiler := NewContractCompilerWithOptions(buildDir, true)
	compiled, err := compiler.Compile(source)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	if compiled.GasProfile == nil {
		t.Fatal("expected GasProfile on CompiledContract")
	}
	if _, ok := compiled.GasProfile.Functions["Add"]; !ok {
		t.Fatal("expected Add in compiled GasProfile")
	}
	if _, ok := compiled.GasProfile.Functions["Info"]; !ok {
		t.Fatal("expected Info in compiled GasProfile")
	}

	manager := NewContractManager(storeDir, NewSecurityReviewer(), NewABIGenerator()).(*ContractManagerImpl)
	address, err := manager.Deploy(compiled)
	if err != nil {
		t.Fatalf("deploy failed: %v", err)
	}

	profilePath := filepath.Join(storeDir, address, "gas_profile.json")
	data, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatalf("gas_profile.json missing: %v", err)
	}

	var onDisk GasProfile
	if err := json.Unmarshal(data, &onDisk); err != nil {
		t.Fatalf("unmarshal gas_profile.json: %v", err)
	}
	for _, name := range []string{"Add", "Info"} {
		if _, ok := onDisk.Functions[name]; !ok {
			t.Fatalf("gas_profile.json missing function %q", name)
		}
	}

	loaded, err := manager.GetGasProfile(address)
	if err != nil {
		t.Fatalf("GetGasProfile: %v", err)
	}
	if _, ok := loaded.Functions["Add"]; !ok {
		t.Fatal("GetGasProfile missing Add")
	}
	if _, ok := loaded.Functions["Info"]; !ok {
		t.Fatal("GetGasProfile missing Info")
	}
}

func TestCompileUsesCacheStillBuildsGasProfile(t *testing.T) {
	buildDir := t.TempDir()
	compiler := NewContractCompilerWithOptions(buildDir, true)

	source := `
package main

func Add(a, b int) int {
	return a + b
}
`
	first, err := compiler.Compile(source)
	if err != nil {
		t.Fatalf("first compile: %v", err)
	}
	if first.GasProfile == nil {
		t.Fatal("first compile: expected GasProfile")
	}

	second, err := compiler.Compile(source)
	if err != nil {
		t.Fatalf("cache-hit compile: %v", err)
	}
	if second.GasProfile == nil {
		t.Fatal("cache-hit compile: expected GasProfile rebuilt from source")
	}
	if _, ok := second.GasProfile.Functions["Add"]; !ok {
		t.Fatal("cache-hit compile: expected Add in GasProfile")
	}
	if second.ExecutablePath != first.ExecutablePath {
		t.Fatalf("expected cache hit path %s, got %s", first.ExecutablePath, second.ExecutablePath)
	}
}
