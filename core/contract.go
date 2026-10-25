package core

import (
	"context"
	"time"
)

// ContractReader retrieves complete artifacts without deployment semantics.
type ContractReader interface {
	Get(context.Context, string) (*CompiledContract, error)
}

// ContractWriter persists an artifact at a caller-selected address.
// Implementations must not mutate the caller's artifact.
type ContractWriter interface {
	Put(context.Context, string, *CompiledContract) error
}
type ContractStore interface {
	ContractReader
	ContractWriter
}

// ContractDeployer owns address assignment and deployment acceptance rules.
type ContractDeployer interface {
	Deploy(context.Context, *CompiledContract) (string, error)
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
	Address         string           `json:"address"`
	SourcePolicy    string           `json:"source_policy,omitempty"`
	Guarantees      PolicyGuarantees `json:"guarantees,omitempty"`
	ProtocolVersion int              `json:"protocol_version"`
	SourceHash      string           `json:"source_hash"`
	DeployTime      time.Time        `json:"deploy_time"`
	Status          ContractStatus   `json:"status"`
	StoragePath     string           `json:"storage_path"`
}
