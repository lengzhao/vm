package vm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/lengzhao/vm/abi"
)

// ContractManager 合约管理模块接口
// 根据简化设计原则，接口已精简为核心功能
// 存储管理功能已整合到合约管理模块中
type ContractManager interface {
	// Deploy 部署合约
	Deploy(contract *CompiledContract) (string, error)

	// GetContract 获取合约
	GetContract(address string) (*CompiledContract, error)

	// GetContractABI 获取合约ABI
	GetContractABI(address string) (*abi.ABI, error)

	// StoreContract 存储合约
	StoreContract(contract *CompiledContract, address string) error

	// LoadContract 加载合约
	LoadContract(address string) (*CompiledContract, error)
}

// ContractManagerImpl 合约管理模块实现
type ContractManagerImpl struct {
	// 合约存储目录
	storageDir string

	// 安全审查模块
	securityReviewer SecurityReviewer

	// ABI生成模块
	abiGenerator ABIGenerator
}

// ContractStatus 合约状态
type ContractStatus int

const (
	ContractStatusUnknown ContractStatus = iota
	ContractStatusDeployed
	ContractStatusSuspended
	ContractStatusDestroyed
)

// ContractMetadata 合约元数据
type ContractMetadata struct {
	Address     string         `json:"address"`
	SourceHash  string         `json:"source_hash"`
	DeployTime  time.Time      `json:"deploy_time"`
	Status      ContractStatus `json:"status"`
	StoragePath string         `json:"storage_path"`
}

// NewContractManager 创建新的合约管理模块实例
func NewContractManager(storageDir string, securityReviewer SecurityReviewer, abiGenerator ABIGenerator) ContractManager {
	// 确保存储目录存在
	os.MkdirAll(storageDir, 0755)

	return &ContractManagerImpl{
		storageDir:       storageDir,
		securityReviewer: securityReviewer,
		abiGenerator:     abiGenerator,
	}
}

// Deploy 部署合约
func (c *ContractManagerImpl) Deploy(contract *CompiledContract) (string, error) {
	// 生成合约地址
	address := c.generateContractAddress(contract.SourceHash)

	// 存储合约
	if err := c.StoreContract(contract, address); err != nil {
		return "", fmt.Errorf("failed to store contract: %w", err)
	}

	// 更新合约地址
	contract.Address = address

	return address, nil
}

// GetContract 获取合约
func (c *ContractManagerImpl) GetContract(address string) (*CompiledContract, error) {
	return c.LoadContract(address)
}

// GetContractABI 获取合约ABI
func (c *ContractManagerImpl) GetContractABI(address string) (*abi.ABI, error) {
	contract, err := c.LoadContract(address)
	if err != nil {
		return nil, err
	}

	return contract.ABI, nil
}

// StoreContract 存储合约
func (c *ContractManagerImpl) StoreContract(contract *CompiledContract, address string) error {
	// 创建合约存储目录
	contractDir := filepath.Join(c.storageDir, address)
	if err := os.MkdirAll(contractDir, 0755); err != nil {
		return fmt.Errorf("failed to create contract directory: %w", err)
	}

	// 复制可执行文件到合约目录
	if contract.ExecutablePath != "" {
		execFileName := filepath.Base(contract.ExecutablePath)
		destExecPath := filepath.Join(contractDir, execFileName)

		// 读取源文件
		data, err := os.ReadFile(contract.ExecutablePath)
		if err != nil {
			return fmt.Errorf("failed to read executable file: %w", err)
		}

		// 写入目标文件
		if err := os.WriteFile(destExecPath, data, 0755); err != nil {
			return fmt.Errorf("failed to write executable file: %w", err)
		}

		// 更新合约的可执行文件路径
		contract.ExecutablePath = destExecPath
	}

	// 创建元数据文件
	metadata := &ContractMetadata{
		Address:     address,
		SourceHash:  contract.SourceHash,
		DeployTime:  time.Now(),
		Status:      ContractStatusDeployed,
		StoragePath: contractDir,
	}

	metadataPath := filepath.Join(contractDir, "metadata.json")
	metadataBytes, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	if err := os.WriteFile(metadataPath, metadataBytes, 0644); err != nil {
		return fmt.Errorf("failed to write metadata file: %w", err)
	}

	// 创建ABI文件
	if contract.ABI != nil {
		abiPath := filepath.Join(contractDir, "abi.json")
		abiBytes, err := json.MarshalIndent(contract.ABI, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal ABI: %w", err)
		}

		if err := os.WriteFile(abiPath, abiBytes, 0644); err != nil {
			return fmt.Errorf("failed to write ABI file: %w", err)
		}
	}

	return nil
}

// LoadContract 加载合约
func (c *ContractManagerImpl) LoadContract(address string) (*CompiledContract, error) {
	// 检查合约目录是否存在
	contractDir := filepath.Join(c.storageDir, address)
	if _, err := os.Stat(contractDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("contract not found: %s", address)
	}

	// 读取元数据
	metadataPath := filepath.Join(contractDir, "metadata.json")
	metadataBytes, err := os.ReadFile(metadataPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read metadata file: %w", err)
	}

	var metadata ContractMetadata
	if err := json.Unmarshal(metadataBytes, &metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	// 读取ABI
	abiPath := filepath.Join(contractDir, "abi.json")
	abiBytes, err := os.ReadFile(abiPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read ABI file: %w", err)
	}

	var contractABI abi.ABI
	if err := json.Unmarshal(abiBytes, &contractABI); err != nil {
		return nil, fmt.Errorf("failed to unmarshal ABI: %w", err)
	}

	// 查找可执行文件
	var execPath string
	entries, err := os.ReadDir(contractDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read contract directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() && entry.Name() != "metadata.json" && entry.Name() != "abi.json" {
			execPath = filepath.Join(contractDir, entry.Name())
			break
		}
	}

	// 创建合约对象
	contract := &CompiledContract{
		ExecutablePath: execPath,
		ABI:            &contractABI,
		CompileTime:    metadata.DeployTime, // 使用部署时间作为编译时间
		SourceHash:     metadata.SourceHash,
		Address:        address,
	}

	return contract, nil
}

// generateContractAddress 生成合约地址
func (c *ContractManagerImpl) generateContractAddress(sourceHash string) string {
	// 使用时间戳和源码哈希生成合约地址
	data := fmt.Sprintf("%s_%d", sourceHash, time.Now().UnixNano())
	hash := sha256.Sum256([]byte(data))
	return "contract_" + hex.EncodeToString(hash[:])[:16]
}
