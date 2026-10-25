package gas

import (
	"github.com/lengzhao/vm/core"
	"sync"
)

// Meter Gas计费模块实现（并发安全）
type Meter struct {
	mu          sync.Mutex
	consumedGas uint64
	gasLimit    uint64
}

// NewGasMetering 创建新的Gas计费模块实例
func NewMeter() *Meter {
	return &Meter{}
}

// ConsumeGas 消耗Gas
func (g *Meter) ConsumeGas(amount uint64) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if amount > ^uint64(0)-g.consumedGas || (g.gasLimit > 0 && (g.consumedGas > g.gasLimit || amount > g.gasLimit-g.consumedGas)) {
		return &core.GasLimitExceededError{
			Consumed: g.consumedGas,
			Limit:    g.gasLimit,
			Required: amount,
		}
	}
	g.consumedGas += amount
	return nil
}

// GetConsumedGas 获取已消耗的Gas
func (g *Meter) GetConsumedGas() uint64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.consumedGas
}

// SetGasLimit 设置Gas限制
func (g *Meter) SetGasLimit(limit uint64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.gasLimit = limit
}

// GetGasLimit 获取Gas限制
func (g *Meter) GetGasLimit() uint64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.gasLimit
}

// Reset 重置Gas计数器
func (g *Meter) Reset() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.consumedGas = 0
}
