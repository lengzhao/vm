package contractapi_test

import (
	"testing"

	"github.com/lengzhao/vm/contractapi"
)

func TestConsumeGasAndLimit(t *testing.T) {
	contractapi.ResetGas()
	contractapi.InitGas(5)
	contractapi.ConsumeGas(3)
	if contractapi.GasUsed() != 3 {
		t.Fatalf("got %d", contractapi.GasUsed())
	}
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}
	}()
	contractapi.ConsumeGas(3) // 超限
}

func TestAPIChargesGas(t *testing.T) {
	contractapi.ResetGas()
	contractapi.InitGas(100)
	_ = contractapi.BlockHeight()
	_ = contractapi.Sender()
	contractapi.Log("x", "k", "v")
	// 1 + 1 + 2 = 4
	if contractapi.GasUsed() != 4 {
		t.Fatalf("got %d want 4", contractapi.GasUsed())
	}
}
