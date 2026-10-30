package core

// GasMetering Gas计费模块接口
type GasMetering interface {
	ConsumeGas(amount uint64) error
	GetConsumedGas() uint64
	SetGasLimit(limit uint64)
	GetGasLimit() uint64
	Reset()
}

// GasLimitExceededError Gas超出限制错误
type GasLimitExceededError struct {
	Consumed uint64
	Limit    uint64
	Required uint64
}

func (e *GasLimitExceededError) Error() string {
	return "gas limit exceeded"
}
