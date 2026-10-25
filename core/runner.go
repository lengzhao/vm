package core

import (
	"context"
)

// CallRequest 合约调用请求
type CallRequest struct {
	Function string        `json:"function"`
	Args     []interface{} `json:"args"`
}

// CallResult 合约调用结果
type CallResult struct {
	Data        []byte
	RawResult   interface{}
	GasConsumed uint64
	Events      []Event
	Stdout      string
	Stderr      string
}

// Runner 执行已编译合约产物
type Runner interface {
	Run(ctx context.Context, contract *CompiledContract, req CallRequest) (*CallResult, error)
}
