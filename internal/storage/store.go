package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/lengzhao/vm/abi"
	"github.com/lengzhao/vm/core"
	"github.com/lengzhao/vm/internal/artifact"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Store persists complete contract artifacts under explicit addresses.
type Store struct {
	storageDir string
	mu         sync.RWMutex
}

func New(dir string) (*Store, error) {
	if dir == "" {
		dir = "./contracts"
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	return &Store{storageDir: dir}, nil
}
func (c *Store) addressDir(address string) (string, error) {
	if address == "" || address == "." || address == ".." || filepath.Base(address) != address {
		return "", fmt.Errorf("invalid contract address")
	}
	return filepath.Join(c.storageDir, address), nil
}

// StoreContract 存储合约
func (c *Store) Put(ctx context.Context, address string, input *core.CompiledContract) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.writeRecord(ctx, address, input, false)
}

func (c *Store) Create(ctx context.Context, address string, input *core.CompiledContract) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.writeRecord(ctx, address, input, true)
}

func (c *Store) writeRecord(ctx context.Context, address string, input *core.CompiledContract, createOnly bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input == nil {
		return executionError(core.CodeInvalidRequest, "contract cannot be nil", nil)
	}
	snapshot := *input
	contract := &snapshot
	contract.Address = address

	if contract == nil {
		return executionError(core.CodeInvalidRequest, "contract cannot be nil", nil)
	}
	if contract.ProtocolVersion != core.ProtocolVersion {
		return executionError(core.CodeUnsupportedVersion, "unsupported artifact protocol; recompile", nil)
	}
	if err := abi.ValidateCallable(contract.ABI); err != nil {
		return executionError(core.CodeUnsupportedABI, err.Error(), err)
	}
	if err := artifact.Validate(ctx, contract); err != nil {
		return err
	}
	// 创建合约存储目录
	finalDir, err := c.addressDir(address)
	if err != nil {
		return err
	}
	if createOnly {
		if exists, err := c.identicalRecord(ctx, address, contract); exists || err != nil {
			return err
		}
	}
	contractDir, err := os.MkdirTemp(c.storageDir, ".record-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(contractDir)

	// 复制可执行文件到合约目录
	if contract.ExecutablePath != "" {
		execFileName := "artifact"
		destExecPath := filepath.Join(contractDir, execFileName)

		// 读取源文件
		data, err := readFile(ctx, contract.ExecutablePath)
		if err != nil {
			return fmt.Errorf("failed to read executable file: %w", err)
		}

		// 写入目标文件
		if err := writeFile(ctx, destExecPath, data, 0755); err != nil {
			return fmt.Errorf("failed to write executable file: %w", err)
		}

		// 更新合约的可执行文件路径
		contract.ExecutablePath = destExecPath
	}

	// 创建元数据文件
	metadata := &core.ContractMetadata{
		Deployment:      contract.Deployment,
		ManifestSHA256:  contract.ManifestSHA256,
		Guarantees:      contract.Guarantees,
		Address:         address,
		SourceHash:      contract.SourceHash,
		SourcePolicy:    contract.SourcePolicy,
		ProtocolVersion: contract.ProtocolVersion,
		DeployTime:      time.Now(),
		Status:          core.ContractStatusDeployed,
		StoragePath:     finalDir,
	}

	metadataPath := filepath.Join(contractDir, "metadata.json")
	metadataBytes, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	if err := writeFile(ctx, metadataPath, metadataBytes, 0644); err != nil {
		return fmt.Errorf("failed to write metadata file: %w", err)
	}

	// 创建ABI文件
	if contract.ABI != nil {
		abiPath := filepath.Join(contractDir, "abi.json")
		abiBytes, err := json.MarshalIndent(contract.ABI, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal ABI: %w", err)
		}

		if err := writeFile(ctx, abiPath, abiBytes, 0644); err != nil {
			return fmt.Errorf("failed to write ABI file: %w", err)
		}
	}

	// 创建 core.GasProfile 文件
	if contract.GasProfile != nil {
		profilePath := filepath.Join(contractDir, "gas_profile.json")
		profileBytes, err := json.MarshalIndent(contract.GasProfile, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal gas profile: %w", err)
		}

		if err := writeFile(ctx, profilePath, profileBytes, 0644); err != nil {
			return fmt.Errorf("failed to write gas profile file: %w", err)
		}
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	manifest, err := json.MarshalIndent(contract.Manifest, "", "  ")
	if err != nil {
		return err
	}
	if err := writeFile(ctx, filepath.Join(contractDir, "manifest.json"), manifest, 0644); err != nil {
		return err
	}
	// Verify the actual copied bytes before publishing the replacement.
	if err := artifact.Validate(ctx, contract); err != nil {
		return err
	}
	if createOnly {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := os.Rename(contractDir, finalDir); err != nil {
			// A complete nonempty address directory cannot be overwritten by rename.
			if exists, checkErr := c.identicalRecord(ctx, address, contract); exists || checkErr != nil {
				return checkErr
			}
			return fmt.Errorf("publish deployment: %w", err)
		}
		return nil
	}
	return c.commit(contractDir, finalDir)
}

// GetGasProfile 读取部署目录中的 gas_profile.json
func (c *Store) gasProfile(ctx context.Context, address string) (*core.GasProfile, error) {
	profilePath := filepath.Join(c.storageDir, address, "gas_profile.json")
	data, err := readFile(ctx, profilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read gas profile: %w", err)
	}

	var profile core.GasProfile
	if err := json.Unmarshal(data, &profile); err != nil {
		return nil, fmt.Errorf("failed to unmarshal gas profile: %w", err)
	}
	return &profile, nil
}

// LoadContract 加载合约
func (c *Store) Get(ctx context.Context, address string) (*core.CompiledContract, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.get(ctx, address)
}

func (c *Store) get(ctx context.Context, address string) (*core.CompiledContract, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// 检查合约目录是否存在
	contractDir, err := c.addressDir(address)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(contractDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("contract not found: %s", address)
	}

	// 读取元数据
	metadataPath := filepath.Join(contractDir, "metadata.json")
	metadataBytes, err := readFile(ctx, metadataPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read metadata file: %w", err)
	}

	var metadata core.ContractMetadata
	if err := json.Unmarshal(metadataBytes, &metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}
	if metadata.ProtocolVersion != core.ProtocolVersion {
		return nil, executionError(core.CodeUnsupportedVersion, "unsupported stored protocol; recompile and deploy", nil)
	}

	// 读取ABI
	abiPath := filepath.Join(contractDir, "abi.json")
	abiBytes, err := readFile(ctx, abiPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read ABI file: %w", err)
	}

	var contractABI abi.ABI
	if err := json.Unmarshal(abiBytes, &contractABI); err != nil {
		return nil, fmt.Errorf("failed to unmarshal ABI: %w", err)
	}
	if err := abi.ValidateCallable(&contractABI); err != nil {
		return nil, executionError(core.CodeUnsupportedABI, err.Error(), err)
	}

	manifestBytes, err := readFile(ctx, filepath.Join(contractDir, "manifest.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, executionError(core.CodeUnsupportedVersion, "artifact manifest missing; recompile and deploy", err)
		}
		return nil, err
	}
	var manifest core.ArtifactManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return nil, executionError(core.CodeArtifactIntegrity, "invalid artifact manifest", err)
	}
	execPath := filepath.Join(contractDir, "artifact")
	if metadata.Address != address {
		return nil, executionError(core.CodeArtifactIntegrity, "stored address mismatch", nil)
	}

	// 创建合约对象
	contract := &core.CompiledContract{
		Deployment:      metadata.Deployment,
		Manifest:        &manifest,
		ManifestSHA256:  metadata.ManifestSHA256,
		ExecutablePath:  execPath,
		ABI:             &contractABI,
		CompileTime:     metadata.DeployTime, // 使用部署时间作为编译时间
		SourceHash:      metadata.SourceHash,
		SourcePolicy:    metadata.SourcePolicy,
		Guarantees:      metadata.Guarantees,
		ProtocolVersion: metadata.ProtocolVersion,
		Address:         address,
	}

	profile, err := c.gasProfile(ctx, address)
	if err != nil {
		return nil, executionError(core.CodeArtifactIntegrity, "invalid gas profile", err)
	}
	contract.GasProfile = profile
	if err := artifact.Validate(ctx, contract); err != nil {
		return nil, err
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return contract, nil
}

// identicalRecord is called while the instance lock is held. Other Create
// instances publish complete directories atomically; Put's replacement contract
// still requires callers to coordinate cross-instance writers.
func (c *Store) identicalRecord(ctx context.Context, address string, input *core.CompiledContract) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	path, err := c.addressDir(address)
	if err != nil {
		return false, err
	}
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	existing, err := c.get(ctx, address)
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	if err != nil {
		return false, executionError(core.CodeDeploymentConflict, "address already contains an invalid record", err)
	}
	sameDeployment := (existing.Deployment == nil && input.Deployment == nil) || (existing.Deployment != nil && input.Deployment != nil && *existing.Deployment == *input.Deployment)
	if existing.ManifestSHA256 != input.ManifestSHA256 || !sameDeployment {
		return false, executionError(core.CodeDeploymentConflict, "address already contains a different deployment", nil)
	}
	return true, nil
}

func executionError(code core.ErrorCode, message string, cause error) error {
	return &core.ExecutionError{Code: code, Message: message, Cause: cause}
}
func readFile(ctx context.Context, path string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("record file must be regular: %s", path)
	}
	data, err := os.ReadFile(path)
	if err == nil {
		err = ctx.Err()
	}
	return data, err
}
func writeFile(ctx context.Context, path string, data []byte, mode os.FileMode) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return os.WriteFile(path, data, mode)
}

// commit publishes a complete snapshot under the per-store lock. On failure,
// restore the previous snapshot; preserve its backup if rollback itself fails.
func (c *Store) commit(staged, target string) error {
	var backup string
	if _, err := os.Stat(target); err == nil {
		dir, err := os.MkdirTemp(c.storageDir, ".previous-")
		if err != nil {
			return err
		}
		backup = filepath.Join(dir, "record")
		if err := os.Rename(target, backup); err != nil {
			os.RemoveAll(dir)
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(staged, target); err != nil {
		if backup != "" {
			if restoreErr := os.Rename(backup, target); restoreErr != nil {
				return errors.Join(err, fmt.Errorf("restore previous artifact from %s: %w", backup, restoreErr))
			}
			os.RemoveAll(filepath.Dir(backup))
		}
		return err
	}
	if backup != "" {
		if err := os.RemoveAll(filepath.Dir(backup)); err != nil {
			return fmt.Errorf("artifact committed but old snapshot cleanup failed: %w", err)
		}
	}
	return nil
}
