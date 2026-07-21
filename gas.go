package vm

import (
	"sync"
)

// GasMetering Gas计费模块接口
type GasMetering interface {
	ConsumeGas(amount uint64) error
	GetConsumedGas() uint64
	SetGasLimit(limit uint64)
	GetGasLimit() uint64
	Reset()
}

// GasMeteringImpl Gas计费模块实现（并发安全）
type GasMeteringImpl struct {
	mu          sync.Mutex
	consumedGas uint64
	gasLimit    uint64
}

// NewGasMetering 创建新的Gas计费模块实例
func NewGasMetering() GasMetering {
	return &GasMeteringImpl{}
}

// ConsumeGas 消耗Gas
func (g *GasMeteringImpl) ConsumeGas(amount uint64) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.gasLimit > 0 && g.consumedGas+amount > g.gasLimit {
		return &GasLimitExceededError{
			Consumed: g.consumedGas,
			Limit:    g.gasLimit,
			Required: amount,
		}
	}
	g.consumedGas += amount
	return nil
}

// GetConsumedGas 获取已消耗的Gas
func (g *GasMeteringImpl) GetConsumedGas() uint64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.consumedGas
}

// SetGasLimit 设置Gas限制
func (g *GasMeteringImpl) SetGasLimit(limit uint64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.gasLimit = limit
}

// GetGasLimit 获取Gas限制
func (g *GasMeteringImpl) GetGasLimit() uint64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.gasLimit
}

// Reset 重置Gas计数器
func (g *GasMeteringImpl) Reset() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.consumedGas = 0
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
