package core

import (
	"context"
	"github.com/lengzhao/vm/contractapi"
)

const (
	DefaultMaxCallDepth           = 8
	MaxCallDepth                  = 32
	MaxCrossCalls                 = 1024
	MaxCrossResultBytes           = contractapi.MaxCrossResultBytes
	MaxCallEventBytes             = 1 << 20
	CodeCallFailure     ErrorCode = "CALL_FAILURE"
	CodeCallDepth       ErrorCode = "CALL_DEPTH_EXCEEDED"
	CodeReentrancy      ErrorCode = "REENTRANCY"
)

// CrossCallRequest carries the call point's inclusive gas and preceding events.
// It never grants transaction completion or sender/context selection.
type CrossCallRequest struct {
	Address     string
	Function    string
	Args        []any
	GasConsumed uint64
	Events      []Event
}
type ContractCaller interface {
	Call(context.Context, CrossCallRequest) (*CallResult, error)
}
type contractCallerKey struct{}

func WithContractCaller(ctx context.Context, caller ContractCaller) context.Context {
	return context.WithValue(ctx, contractCallerKey{}, caller)
}
func ContractCallerFromContext(ctx context.Context) (ContractCaller, bool) {
	c, ok := ctx.Value(contractCallerKey{}).(ContractCaller)
	return c, ok && c != nil
}
