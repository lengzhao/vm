package callcontext

import (
	"context"
	"github.com/lengzhao/vm/core"
)

type Resolved struct {
	BlockHeight    uint64
	BlockTime      uint64
	Sender         core.Address
	Contract       core.Address
	GasLimit       uint64
	BlockHeightSet bool
	BlockTimeSet   bool
	SenderSet      bool
	ContractSet    bool
	GasLimitSet    bool
}

func Resolve(cc *core.CallContext) Resolved {
	var out Resolved
	if cc == nil {
		return out
	}
	if cc.BlockHeight != nil {
		out.BlockHeight = *cc.BlockHeight
		out.BlockHeightSet = true
	} else if cc.Host != nil {
		out.BlockHeight = cc.Host.BlockHeight()
		out.BlockHeightSet = true
	}
	if cc.BlockTime != nil {
		out.BlockTime = *cc.BlockTime
		out.BlockTimeSet = true
	} else if cc.Host != nil {
		out.BlockTime = cc.Host.BlockTime()
		out.BlockTimeSet = true
	}
	if cc.Sender != nil {
		out.Sender = *cc.Sender
		out.SenderSet = true
	} else if cc.Host != nil {
		out.Sender = cc.Host.Sender()
		out.SenderSet = true
	}
	if cc.ContractAddress != nil {
		out.Contract = *cc.ContractAddress
		out.ContractSet = true
	} else if cc.Host != nil {
		out.Contract = cc.Host.ContractAddress()
		out.ContractSet = true
	}
	if cc.GasLimit != nil {
		out.GasLimit = *cc.GasLimit
		out.GasLimitSet = true
	}
	return out
}

type gasLimitKey struct{}
type callContextKey struct{}

// WithGasLimit 将 Gas 限制放入 context，供 Runner 传给子进程
func WithGasLimit(ctx context.Context, limit uint64) context.Context {
	return context.WithValue(ctx, gasLimitKey{}, limit)
}

func GasLimitFromContext(ctx context.Context) (uint64, bool) {
	v := ctx.Value(gasLimitKey{})
	if v == nil {
		return 0, false
	}
	limit, ok := v.(uint64)
	return limit, ok
}

// WithCallContext 将调用上下文放入 context
func WithCallContext(ctx context.Context, callCtx *core.CallContext) context.Context {
	return context.WithValue(ctx, callContextKey{}, callCtx)
}

// CallContextFrom 从 context 取出 core.CallContext
func CallContextFrom(ctx context.Context) (*core.CallContext, bool) {
	v := ctx.Value(callContextKey{})
	if v == nil {
		return nil, false
	}
	cc, ok := v.(*core.CallContext)
	return cc, ok
}
