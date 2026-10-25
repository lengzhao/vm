package core

import (
	"context"
	"github.com/lengzhao/vm/abi"
	"time"
)

// ContractCompiler produces one executable artifact from source.
type ContractCompiler interface {
	Compile(context.Context, string) (*CompiledContract, error)
}

// CompiledContract 编译后的合约
type CompiledContract struct {
	ExecutablePath  string
	SourcePolicy    string
	Guarantees      PolicyGuarantees
	ProtocolVersion int
	ABI             *abi.ABI
	GasProfile      *GasProfile
	CompileTime     time.Time
	SourceHash      string
	Address         string
}
