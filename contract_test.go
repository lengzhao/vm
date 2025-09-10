package vm

import (
	"os"
	"testing"
	"time"
	
	"github.com/lengzhao/vm/abi"
)

func TestNewContractManager(t *testing.T) {
	securityReviewer := NewSecurityReviewer()
	abiGenerator := NewABIGenerator()
	
	contractManager := NewContractManager("./test_contracts", securityReviewer, abiGenerator)
	
	if contractManager == nil {
		t.Error("Expected ContractManager to be created, got nil")
	}
}

func TestDeployAndLoadContract(t *testing.T) {
	// 创建测试目录
	testDir := "./test_contracts_deploy"
	defer os.RemoveAll(testDir)
	
	securityReviewer := NewSecurityReviewer()
	abiGenerator := NewABIGenerator()
	
	contractManager := NewContractManager(testDir, securityReviewer, abiGenerator)
	
	// 创建测试合约
	testABI := &abi.ABI{
		PackageName: "test",
		Functions: []abi.Function{
			{Name: "TestFunc", Inputs: nil, Outputs: nil},
		},
	}
	
	contract := &CompiledContract{
		ExecutablePath: "",
		ABI:            testABI,
		CompileTime:    time.Now(),
		SourceHash:     "test_hash",
		Address:        "",
	}
	
	// 部署合约
	address, err := contractManager.Deploy(contract)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	
	if address == "" {
		t.Error("Expected contract address to be generated")
	}
	
	// 加载合约
	loadedContract, err := contractManager.GetContract(address)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	
	if loadedContract == nil {
		t.Error("Expected contract to be loaded")
	}
	
	if loadedContract.Address != address {
		t.Errorf("Expected contract address %s, got %s", address, loadedContract.Address)
	}
	
	if loadedContract.SourceHash != "test_hash" {
		t.Errorf("Expected source hash 'test_hash', got %s", loadedContract.SourceHash)
	}
}

func TestGetContractABI(t *testing.T) {
	// 创建测试目录
	testDir := "./test_contracts_abi"
	defer os.RemoveAll(testDir)
	
	securityReviewer := NewSecurityReviewer()
	abiGenerator := NewABIGenerator()
	
	contractManager := NewContractManager(testDir, securityReviewer, abiGenerator)
	
	// 创建测试合约
	testABI := &abi.ABI{
		PackageName: "test",
		Functions: []abi.Function{
			{Name: "TestFunc", Inputs: nil, Outputs: nil},
		},
	}
	
	contract := &CompiledContract{
		ExecutablePath: "",
		ABI:            testABI,
		CompileTime:    time.Now(),
		SourceHash:     "test_hash",
		Address:        "",
	}
	
	// 部署合约
	address, err := contractManager.Deploy(contract)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	
	// 获取合约ABI
	loadedABI, err := contractManager.GetContractABI(address)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	
	if loadedABI == nil {
		t.Error("Expected ABI to be loaded")
	}
	
	if loadedABI.PackageName != "test" {
		t.Errorf("Expected package name 'test', got %s", loadedABI.PackageName)
	}
	
	if len(loadedABI.Functions) != 1 {
		t.Errorf("Expected 1 function, got %d", len(loadedABI.Functions))
	}
}

func TestStoreAndLoadContract(t *testing.T) {
	// 创建测试目录
	testDir := "./test_contracts_store"
	defer os.RemoveAll(testDir)
	
	securityReviewer := NewSecurityReviewer()
	abiGenerator := NewABIGenerator()
	
	contractManager := NewContractManager(testDir, securityReviewer, abiGenerator)
	
	// 创建测试合约
	testABI := &abi.ABI{
		PackageName: "test",
		Functions: []abi.Function{
			{Name: "TestFunc", Inputs: nil, Outputs: nil},
		},
	}
	
	contract := &CompiledContract{
		ExecutablePath: "",
		ABI:            testABI,
		CompileTime:    time.Now(),
		SourceHash:     "test_hash",
		Address:        "",
	}
	
	// 存储合约
	address := "test_contract_address"
	err := contractManager.StoreContract(contract, address)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	
	// 加载合约
	loadedContract, err := contractManager.LoadContract(address)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	
	if loadedContract == nil {
		t.Error("Expected contract to be loaded")
	}
	
	if loadedContract.Address != address {
		t.Errorf("Expected contract address %s, got %s", address, loadedContract.Address)
	}
	
	if loadedContract.SourceHash != "test_hash" {
		t.Errorf("Expected source hash 'test_hash', got %s", loadedContract.SourceHash)
	}
}

func TestLoadNonExistentContract(t *testing.T) {
	// 创建测试目录
	testDir := "./test_contracts_nonexistent"
	defer os.RemoveAll(testDir)
	
	securityReviewer := NewSecurityReviewer()
	abiGenerator := NewABIGenerator()
	
	contractManager := NewContractManager(testDir, securityReviewer, abiGenerator)
	
	// 尝试加载不存在的合约
	_, err := contractManager.LoadContract("nonexistent_contract")
	if err == nil {
		t.Error("Expected error for non-existent contract, got nil")
	}
}