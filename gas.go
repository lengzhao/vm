package vm

// GasMetering Gas计费模块接口
// 根据简化设计原则，接口已精简为核心功能
type GasMetering interface {
	// ConsumeGas 消耗Gas
	ConsumeGas(amount uint64) error
	
	// GetConsumedGas 获取已消耗的Gas
	GetConsumedGas() uint64
	
	// SetGasLimit 设置Gas限制
	SetGasLimit(limit uint64)
	
	// GetGasLimit 获取Gas限制
	GetGasLimit() uint64
	
	// Reset 重置Gas计数器
	Reset()
}

// GasMeteringImpl Gas计费模块实现
type GasMeteringImpl struct {
	// 已消耗的Gas
	consumedGas uint64
	
	// Gas限制
	gasLimit uint64
}

// NewGasMetering 创建新的Gas计费模块实例
func NewGasMetering() GasMetering {
	return &GasMeteringImpl{
		consumedGas: 0,
		gasLimit:    0, // 默认无限制
	}
}

// ConsumeGas 消耗Gas
func (g *GasMeteringImpl) ConsumeGas(amount uint64) error {
	// 检查是否会超出Gas限制
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
	return g.consumedGas
}

// SetGasLimit 设置Gas限制
func (g *GasMeteringImpl) SetGasLimit(limit uint64) {
	g.gasLimit = limit
}

// GetGasLimit 获取Gas限制
func (g *GasMeteringImpl) GetGasLimit() uint64 {
	return g.gasLimit
}

// Reset 重置Gas计数器
func (g *GasMeteringImpl) Reset() {
	g.consumedGas = 0
}

// GasLimitExceededError Gas超出限制错误
type GasLimitExceededError struct {
	Consumed uint64 // 已消耗的Gas
	Limit    uint64 // Gas限制
	Required uint64 // 需要的Gas
}

func (e *GasLimitExceededError) Error() string {
	return "gas limit exceeded"
}