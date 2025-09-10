package vm

import (
	"testing"
)

func TestNewGasMetering(t *testing.T) {
	gasMeter := NewGasMetering()

	if gasMeter == nil {
		t.Error("Expected GasMetering to be created, got nil")
	}

	if gasMeter.GetConsumedGas() != 0 {
		t.Errorf("Expected initial consumed gas to be 0, got %d", gasMeter.GetConsumedGas())
	}

	if gasMeter.GetGasLimit() != 0 {
		t.Errorf("Expected initial gas limit to be 0, got %d", gasMeter.GetGasLimit())
	}
}

func TestConsumeGas(t *testing.T) {
	gasMeter := NewGasMetering()

	// 测试正常消耗Gas
	err := gasMeter.ConsumeGas(100)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if gasMeter.GetConsumedGas() != 100 {
		t.Errorf("Expected consumed gas to be 100, got %d", gasMeter.GetConsumedGas())
	}

	// 测试累加消耗Gas
	err = gasMeter.ConsumeGas(50)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if gasMeter.GetConsumedGas() != 150 {
		t.Errorf("Expected consumed gas to be 150, got %d", gasMeter.GetConsumedGas())
	}
}

func TestSetAndGetGasLimit(t *testing.T) {
	gasMeter := NewGasMetering()

	// 测试设置Gas限制
	gasMeter.SetGasLimit(1000)

	if gasMeter.GetGasLimit() != 1000 {
		t.Errorf("Expected gas limit to be 1000, got %d", gasMeter.GetGasLimit())
	}
}

func TestGasLimitExceeded(t *testing.T) {
	gasMeter := NewGasMetering()

	// 设置Gas限制
	gasMeter.SetGasLimit(100)

	// 消耗一些Gas
	err := gasMeter.ConsumeGas(50)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// 尝试消耗超过限制的Gas
	err = gasMeter.ConsumeGas(60)
	if err == nil {
		t.Error("Expected GasLimitExceededError, got nil")
	}

	// 验证错误类型
	if _, ok := err.(*GasLimitExceededError); !ok {
		t.Errorf("Expected GasLimitExceededError, got %T", err)
	}
}

func TestReset(t *testing.T) {
	gasMeter := NewGasMetering()

	// 消耗一些Gas
	gasMeter.ConsumeGas(100)
	gasMeter.SetGasLimit(1000)

	// 重置
	gasMeter.Reset()

	if gasMeter.GetConsumedGas() != 0 {
		t.Errorf("Expected consumed gas to be 0 after reset, got %d", gasMeter.GetConsumedGas())
	}

	// 验证限制没有被重置
	if gasMeter.GetGasLimit() != 1000 {
		t.Errorf("Expected gas limit to remain 1000 after reset, got %d", gasMeter.GetGasLimit())
	}
}

func TestGasLimitExceededError(t *testing.T) {
	err := &GasLimitExceededError{
		Consumed: 50,
		Limit:    100,
		Required: 60,
	}

	if err.Error() != "gas limit exceeded" {
		t.Errorf("Expected error message 'gas limit exceeded', got '%s'", err.Error())
	}
}
